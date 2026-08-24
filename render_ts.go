//go:build !js

package crystalline

import (
	"fmt"
	"go/types"
	"reflect"
	"sort"
	"strings"
)

// Rendering of the TypeScript declarations and the JavaScript module.
func (g *Generator) Build(declarations Declarations) (Output, error) {
	g.marks = newMarks(declarations)

	analysed, err := g.analyse(declarations)
	if err != nil {
		return Output{}, err
	}

	g.readonly = analysed.readonly
	g.dropped = analysed.dropped
	g.usesResult = false

	entities := make(map[string][]Entry)

	// A type is declared where it is defined, not where it was reached from,
	// so interfaces are grouped by their own package rather than by the entry
	// that pulled them in.
	interfaces := make(map[string][]*types.Named)
	seen := make(map[*types.Named]bool)

	for _, entry := range declarations.Entries {
		// Left out of the bindings, so left out here too. One decision about
		// what can be bound, read by both artifacts.
		if g.dropped[entry.Namespace+"."+entry.Name] {
			continue
		}

		switch entry.Kind {
		case EntryFunc, EntryValue:
			entities[entry.Namespace] = append(entities[entry.Namespace], entry)
		case EntryType, EntryPlain:
		default:
			continue
		}

		reached := make([]*types.Named, 0)
		collectNamed(g.marks, entry.Type, seen, &reached)

		for _, named := range reached {
			owner := named.Obj().Name()
			if named.Obj().Pkg() != nil {
				owner = named.Obj().Pkg().Name()
			}

			interfaces[owner] = append(interfaces[owner], named)
		}
	}

	namespaces := make(map[string]bool, len(entities)+len(interfaces))
	for namespace := range entities {
		namespaces[namespace] = true
	}

	for namespace := range interfaces {
		namespaces[namespace] = true
	}

	var rendered, bindings strings.Builder

	names := make([]string, 0, len(namespaces))

	for _, namespace := range sortedKeys(namespaces) {
		declared := interfaces[namespace]
		sort.Slice(declared, func(i, j int) bool { return instantiatedName(declared[i]) < instantiatedName(declared[j]) })

		exposed := entities[namespace]
		sort.SliceStable(exposed, func(i, j int) bool { return exposed[i].Name < exposed[j].Name })

		namespaced, err := g.renderNamespace(namespace, declared, exposed)
		if err != nil {
			return Output{}, err
		}

		rendered.WriteString(namespaced)

		enums := make([]string, 0)

		for _, named := range declared {
			if len(enumConstants(named)) > 0 {
				enums = append(enums, named.Obj().Name())
			}
		}

		if bound := g.renderNamespaceJS(namespace, exposed, enums); bound != "" {
			names = append(names, namespace)
			bindings.WriteString(bound)
		}
	}

	// Composed last, because whether Result is declared depends on what the
	// namespaces turned out to contain.
	var tsd strings.Builder

	tsd.WriteString(g.bannerText())

	if g.usesResult {
		tsd.WriteString(resultDeclarations)
	}

	tsd.WriteString(rendered.String())

	if len(names) > 0 {
		tsd.WriteString(tsLoader(names))
	}

	if g.profile {
		tsd.WriteString(tsProfiling)
	}

	tsd.WriteString("export const initializeCrystalline: () => void;")

	var js strings.Builder

	js.WriteString(g.bannerText())
	helper := jsWrapHelper
	if g.profile {
		helper = jsProfilingWrapHelper
	}

	js.WriteString(helper)
	js.WriteString("\n\n")
	js.WriteString(jsPendingHelper(g.style))
	js.WriteString("\n\n")

	for _, name := range names {
		js.WriteString("export let " + name + " = pending(" + g.style.quoted(name) + ");\n")
	}

	js.WriteString("\nexport const initializeCrystalline = () => {\n")
	js.WriteString(initGuard(g.style, g.appName))
	js.WriteString(bindings.String())

	// Recorded so that a stale binding captured before this ran can say which
	// of the two mistakes it is.
	js.WriteString("\n  initialized = true;\n")
	js.WriteString("};")

	if len(names) > 0 {
		js.WriteString("\n\n")
		js.WriteString(jsLoader(g.style, names))
	}

	return Output{TypeScript: tsd.String(), JavaScript: js.String(), Skipped: analysed.skipped, Warnings: analysed.warnings}, nil
}

// bannerText renders the configured banner, ending it with a newline so the
// generated content starts on its own line.
func (g *Generator) bannerText() string {
	if g.banner == "" {
		return ""
	}

	return strings.TrimRight(g.banner, "\n") + "\n\n"
}

// renderNamespace renders the interfaces a package declares, then the entities
// exposed under its name.
func (g *Generator) renderNamespace(namespace string, declared []*types.Named, exposed []Entry) (string, error) {
	var body strings.Builder

	for _, named := range declared {
		if constants := enumConstants(named); len(constants) > 0 {
			body.WriteString(renderEnum(named, constants))

			continue
		}

		rendered, err := g.renderInterface(named)
		if err != nil {
			return "", err
		}

		body.WriteString(rendered)
	}

	for _, entry := range exposed {
		rendered, err := g.renderEntity(namespace, entry)
		if err != nil {
			return "", err
		}

		body.WriteString(rendered)
	}

	return "export declare namespace " + namespace + " {\n" + body.String() + "}\n", nil
}

func (g *Generator) renderEntity(namespace string, entry Entry) (string, error) {
	if sig, ok := entry.Type.(*types.Signature); ok {
		promise := entry.Promise
		if entry.Object != nil {
			promise = promise || g.isPromise(entry.Object)
		}

		rendered, err := g.renderSignature(entry.Name, sig, true, promise)
		if err != nil {
			return "", fmt.Errorf("%s.%s: %w", namespace, entry.Name, err)
		}

		return "  function " + rendered + ";\n", nil
	}

	jsName, optional, err := g.tsType(entry.Type)
	if err != nil {
		return "", fmt.Errorf("%s.%s: %w", namespace, entry.Name, err)
	}

	if optional {
		jsName += orUndefined
	}

	return "  const " + entry.Name + ": " + jsName + ";\n", nil
}

// renderNamespaceJS binds one namespace out of the global object graph the Go
// side publishes into.
func (g *Generator) renderNamespaceJS(namespace string, bound []Entry, enums []string) string {
	if len(bound) == 0 && len(enums) == 0 {
		return ""
	}

	prefix := "globalThis[" + g.style.quoted("go") + "][" + g.style.quoted(g.appName) + "][" + g.style.quoted(namespace) + "]"

	var out strings.Builder

	out.WriteString("  " + namespace + " = {\n")

	names := make([]string, 0, len(bound)+len(enums))
	accessors := make(map[string]string, len(bound)+len(enums))

	for _, entry := range bound {
		access := prefix + "[" + g.style.quoted(entry.Name) + "]"
		if _, isFunc := entry.Type.(*types.Signature); isFunc {
			access = "wrap(" + g.style.quoted(namespace+"."+entry.Name) + ", " + access + ")"
		}

		names = append(names, entry.Name)
		accessors[entry.Name] = access
	}

	// An enum's constants are a value like any other, and not callable, so they
	// are bound directly.
	for _, name := range enums {
		names = append(names, name)
		accessors[name] = prefix + "[" + g.style.quoted(name) + "]"
	}

	sort.Strings(names)

	for i, name := range names {
		comma := ","
		if !g.style.trailingComma && i == len(names)-1 {
			comma = ""
		}

		out.WriteString("    " + name + ": " + accessors[name] + comma + "\n")
	}

	out.WriteString("  };\n")

	return out.String()
}

func (g *Generator) renderInterface(named *types.Named) (string, error) {
	structType, ok := named.Underlying().(*types.Struct)
	if !ok {
		return "", fmt.Errorf("%s is not a struct", named.Obj().Name())
	}

	plain := g.marks.isPlain(named)

	var result strings.Builder

	result.WriteString("  interface " + instantiatedName(named) + " {\n")

	for i := 0; i < structType.NumFields(); i++ {
		field := structType.Field(i)
		if !field.Exported() {
			continue
		}

		tag := reflect.StructTag(structType.Tag(i)).Get(tagName)
		if err := validateTag(tag); err != nil {
			return "", fmt.Errorf("%s.%s: %w", named.Obj().Name(), field.Name(), err)
		}

		if g.dropped[instantiatedName(named)+"."+field.Name()] {
			continue
		}

		jsName, optional, err := g.tsType(field.Type())
		if err != nil {
			return "", fmt.Errorf("%s.%s: %w", named.Obj().Name(), field.Name(), err)
		}

		marker := ""
		if optional && !tagHasOption(tag, tagNotNil) {
			marker = "?"
		}

		// A field with no way back from JS throws when written, and plain data
		// is a snapshot, so the declaration says so instead of inviting a write
		// that goes nowhere.
		prefix := ""
		if plain || g.readonly[instantiatedName(named)+"."+field.Name()] {
			prefix = "readonly "
		}

		result.WriteString("    " + prefix + field.Name() + marker + ": " + jsName + ";\n")
	}

	// Plain data carries no methods, so declaring them would promise something
	// that is not there.
	var methods []*types.Func
	if !plain {
		methods = exportedMethods(named)
	}

	for _, method := range methods {
		if g.marks.ignored[markKey(named, method.Name())] || g.dropped[instantiatedName(named)+"."+method.Name()] {
			continue
		}

		promise := g.marks.promised[markKey(named, method.Name())] || g.isPromise(method)

		signature, err := g.renderSignature(method.Name(), method.Type().(*types.Signature), true, promise)
		if err != nil {
			return "", fmt.Errorf("%s.%s: %w", named.Obj().Name(), method.Name(), err)
		}

		result.WriteString("    " + signature + ";\n")
	}

	// A wrapper is a live view backed by Go, and holds a slot in the bridge for
	// every field and method until it is let go. Both of these exist at run
	// time; leaving them out of the declarations meant the deterministic
	// release the documentation recommends did not typecheck.
	if !plain {
		result.WriteString("    /** Releases the Go resources behind this wrapper. */\n")
		result.WriteString("    release(): void;\n")
		result.WriteString("    [Symbol.dispose](): void;\n")
	}

	result.WriteString("  }\n")

	return result.String(), nil
}

// renderSignature renders a function type. Named signatures become
// "Name(a: T): R"; anonymous ones become "(a: T) => R".
func (g *Generator) renderSignature(name string, sig *types.Signature, named bool, promise bool) (string, error) {
	// Whether a call is a promise is decided in one place, shared with the
	// emitter, so the declarations cannot describe a shape the bindings do not
	// produce.
	promise = promise || asyncSignature(sig)

	var result strings.Builder

	if named {
		result.WriteString(name)
	}

	result.WriteString("(")

	params := sig.Params()
	for i := 0; i < params.Len(); i++ {
		if i > 0 {
			result.WriteString(", ")
		}

		param := params.At(i)

		if channel, ok := param.Type().Underlying().(*types.Chan); ok {
			if err := checkChannelParameter(channel, param.Name()); err != nil {
				return "", err
			}
		}

		// A Go call blocks the single JS thread, so a context is the only way
		// to cancel one; AbortSignal is how JS spells the same thing.
		if i == 0 && isContextType(param.Type()) {
			name := param.Name()
			if name == "" || name == "ctx" {
				name = "signal"
			}

			result.WriteString(name + ": AbortSignal")

			continue
		}

		var (
			jsName   string
			optional bool
			err      error
		)

		// Go awaits whatever a JS callback returns, so a callback parameter is
		// typed as returning a promise, and forces the whole call to be async.
		if callback, isFunc := param.Type().Underlying().(*types.Signature); isFunc {
			jsName, err = g.renderSignature("", callback, false, true)
		} else {
			jsName, optional, err = g.tsType(param.Type())
		}

		if err != nil {
			return "", err
		}

		argName := param.Name()
		if argName == "" {
			argName = fmt.Sprintf("arg%d", i+1)
		}

		// A pointer means null is allowed, not that the argument may be left
		// out, so it stays required and widens instead. Marking it optional
		// would also make any required parameter after it a syntax error.
		if optional {
			jsName += orUndefined
		}

		result.WriteString(argName + ": " + jsName)
	}

	result.WriteString(")")

	if named {
		result.WriteString(": ")
	} else {
		result.WriteString(" => ")
	}

	results, fallible := splitError(sig.Results())
	if fallible {
		g.usesResult = true
	}

	if promise {
		result.WriteString("Promise<")
	}

	// Failure is carried by the value rather than by timing: a call that can
	// fail is not necessarily slow, and making it a promise would force every
	// caller to be async for no reason.
	if fallible {
		result.WriteString("Result<")
	}

	returns, err := g.renderResults(results)
	if err != nil {
		return "", err
	}

	result.WriteString(returns)

	if fallible {
		result.WriteString(">")
	}

	if promise {
		result.WriteString(">")
	}

	return result.String(), nil
}

// checkChannelParameter refuses a channel whose type does not say which way the
// function uses it.
//
// Reading an undirected channel as a source would silently discard anything the
// function sends, and a duplex over one channel cannot work either: both sides
// would draw from the same queue, so Go could receive its own values. The
// direction has to come from the author.
func checkChannelParameter(channel *types.Chan, name string) error {
	switch channel.Dir() {
	case types.RecvOnly:
		return nil
	case types.SendOnly:
		return fmt.Errorf("parameter %s: a send-only channel has no JS counterpart, return a <-chan instead", name)
	}

	return fmt.Errorf("parameter %s: a channel parameter must say its direction, use <-chan %s to receive what the caller supplies",
		name, types.TypeString(channel.Elem(), nil))
}

// isContextType reports whether a type is context.Context.
func isContextType(t types.Type) bool {
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}

	obj := named.Obj()

	return obj.Pkg() != nil && obj.Pkg().Path() == contextPackage && obj.Name() == "Context"
}

// splitError separates a trailing error from the values a call produces.
func splitError(results *types.Tuple) ([]*types.Var, bool) {
	values := make([]*types.Var, 0, results.Len())

	for i := 0; i < results.Len(); i++ {
		values = append(values, results.At(i))
	}

	if len(values) == 0 || !isErrorType(values[len(values)-1].Type()) {
		return values, false
	}

	return values[:len(values)-1], true
}

func (g *Generator) renderResults(results []*types.Var) (string, error) {
	if len(results) == 0 {
		return "void", nil
	}

	var result strings.Builder

	if len(results) > 1 {
		result.WriteString("[")
	}

	for i, value := range results {
		if i > 0 {
			result.WriteString(", ")
		}

		jsName, optional, err := g.tsType(value.Type())
		if err != nil {
			return "", err
		}

		if optional {
			result.WriteString("(" + jsName + " | undefined)")
		} else {
			result.WriteString(jsName)
		}
	}

	if len(results) > 1 {
		result.WriteString("]")
	}

	return result.String(), nil
}

// tsType renders a Go type as its TypeScript name, reporting whether the value
// can be absent on the JS side.
