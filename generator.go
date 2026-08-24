//go:build !js

package crystalline

import (
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Generator produces crystalline declarations by reading Go source, without
// running the program being bound.
//
// It is the static counterpart to Exposer. Where Exposer walks reflect over
// live values inside a running wasm binary, Generator walks go/types over the
// source, which means parameter names, doc comment directives and generic type
// arguments are available directly rather than being reconstructed at runtime.
//
// This is an experiment: it currently emits TypeScript declarations only.
type Generator struct {
	appName string
	style   jsStyle
	dir     string

	// roots are the packages Load was asked for, which is where manifests and
	// directives are looked for. pkgs additionally holds the packages those
	// declarations reach into, loaded for their doc comments.
	roots []*packages.Package
	pkgs  []*packages.Package

	// marks carries the method decisions the current build declared.
	marks marks

	// promises records which declarations carry the promise directive, keyed by
	// a stable name rather than by object identity: a package loaded twice
	// yields different objects for the same declaration.
	promises map[string]bool
}

// GeneratorOption configures a Generator at construction time.
type GeneratorOption func(*Generator)

// WithQuoteStyle sets the quote character used in the generated JavaScript, so
// the output can match the project's formatter. Defaults to a single quote.
func WithQuoteStyle(quote string) GeneratorOption {
	return func(g *Generator) {
		g.style.quote = quote
	}
}

// WithTrailingComma emits trailing commas in the generated JavaScript.
func WithTrailingComma() GeneratorOption {
	return func(g *Generator) {
		g.style.trailingComma = true
	}
}

// NewGenerator creates a Generator publishing under the global go.<appName>
// object on the JS side.
func NewGenerator(appName string, opts ...GeneratorOption) *Generator {
	g := &Generator{
		appName:  appName,
		style:    defaultStyle(),
		promises: make(map[string]bool),
	}

	for _, opt := range opts {
		opt(g)
	}

	return g
}

// Load parses the given package patterns, resolved relative to dir.
func (g *Generator) Load(dir string, patterns ...string) error {
	g.dir = dir

	pkgs, err := g.load(dir, patterns...)
	if err != nil {
		return err
	}

	g.roots = append(g.roots, pkgs...)
	g.pkgs = append(g.pkgs, pkgs...)

	return nil
}

func (g *Generator) load(dir string, patterns ...string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo | packages.NeedFiles,
		Dir:  dir,
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("loading packages: %w", err)
	}

	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("loading %s: %w", pkg.PkgPath, pkg.Errors[0])
		}

		g.collectDocs(pkg)
	}

	return pkgs, nil
}

// loadReferenced pulls in the packages the declarations reach into, so that
// directives written on their declarations are seen. Without this a promise
// comment on a bound method would be silently ignored.
func (g *Generator) loadReferenced(entries []Entry) error {
	known := make(map[string]bool, len(g.pkgs))
	for _, pkg := range g.pkgs {
		known[pkg.PkgPath] = true
	}

	wanted := make(map[string]bool)

	for _, entry := range entries {
		for _, pkg := range referencedPackages(entry) {
			if pkg != "" && !known[pkg] {
				wanted[pkg] = true
			}
		}
	}

	if len(wanted) == 0 {
		return nil
	}

	patterns := make([]string, 0, len(wanted))
	for pkg := range wanted {
		patterns = append(patterns, pkg)
	}

	sort.Strings(patterns)

	pkgs, err := g.load(g.dir, patterns...)
	if err != nil {
		return err
	}

	g.pkgs = append(g.pkgs, pkgs...)

	return nil
}

func referencedPackages(entry Entry) []string {
	var out []string

	if entry.Object != nil && entry.Object.Pkg() != nil {
		out = append(out, entry.Object.Pkg().Path())
	}

	seen := make(map[*types.Named]bool)
	order := make([]*types.Named, 0)
	collectNamed(entry.Type, seen, &order)

	for _, named := range order {
		if named.Obj().Pkg() != nil {
			out = append(out, named.Obj().Pkg().Path())
		}
	}

	return out
}

// collectDocs indexes declaration doc comments by the object they declare, so
// that directives can be looked up from a types.Object later.
func (g *Generator) collectDocs(pkg *packages.Package) {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Doc == nil {
				continue
			}

			obj, ok := pkg.TypesInfo.Defs[fn.Name]
			if !ok || obj == nil {
				continue
			}

			if hasDirective(fn.Doc, promiseDirective) {
				g.promises[objectKey(obj)] = true
			}
		}
	}
}

func (g *Generator) isPromise(obj types.Object) bool {
	if obj == nil {
		return false
	}

	return g.promises[objectKey(obj)]
}

// objectKey names a declaration independently of which load produced it.
func objectKey(obj types.Object) string {
	path := ""
	if obj.Pkg() != nil {
		path = obj.Pkg().Path()
	}

	if fn, ok := obj.(*types.Func); ok {
		if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
			receiver := sig.Recv().Type()
			if pointer, ok := receiver.(*types.Pointer); ok {
				receiver = pointer.Elem()
			}

			if named, ok := receiver.(*types.Named); ok {
				return path + "." + named.Obj().Name() + "." + fn.Name()
			}
		}
	}

	return path + "." + obj.Name()
}

// Build renders the JS module and TypeScript declarations for everything the
// loaded manifests and directives declare.
func (g *Generator) Build(declarations Declarations) (Output, error) {
	g.marks = newMarks(declarations)

	entities := make(map[string][]Entry)

	// A type is declared where it is defined, not where it was reached from,
	// so interfaces are grouped by their own package rather than by the entry
	// that pulled them in.
	interfaces := make(map[string][]*types.Named)
	seen := make(map[*types.Named]bool)

	for _, entry := range declarations.Entries {
		switch entry.Kind {
		case EntryFunc, EntryValue:
			entities[entry.Namespace] = append(entities[entry.Namespace], entry)
		case EntryType:
		default:
			continue
		}

		reached := make([]*types.Named, 0)
		collectNamed(entry.Type, seen, &reached)

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

	var tsd, bindings strings.Builder

	tsd.WriteString(resultDeclarations)

	names := make([]string, 0, len(namespaces))

	for _, namespace := range sortedKeys(namespaces) {
		declared := interfaces[namespace]
		sort.Slice(declared, func(i, j int) bool { return instantiatedName(declared[i]) < instantiatedName(declared[j]) })

		exposed := entities[namespace]
		sort.SliceStable(exposed, func(i, j int) bool { return exposed[i].Name < exposed[j].Name })

		rendered, err := g.renderNamespace(namespace, declared, exposed)
		if err != nil {
			return Output{}, err
		}

		tsd.WriteString(rendered)

		if bound := g.renderNamespaceJS(namespace, exposed); bound != "" {
			names = append(names, namespace)
			bindings.WriteString(bound)
		}
	}

	tsd.WriteString("export const initializeCrystalline: () => void;")

	var js strings.Builder

	js.WriteString(jsWrapHelper)
	js.WriteString("\n\n")

	for _, name := range names {
		js.WriteString("export let " + name + ";\n")
	}

	js.WriteString("\nexport const initializeCrystalline = () => {\n")
	js.WriteString(initGuard(g.style, g.appName))
	js.WriteString(bindings.String())
	js.WriteString("};")

	return Output{TypeScript: tsd.String(), JavaScript: js.String()}, nil
}

// renderNamespace renders the interfaces a package declares, then the entities
// exposed under its name.
func (g *Generator) renderNamespace(namespace string, declared []*types.Named, exposed []Entry) (string, error) {
	var body strings.Builder

	for _, named := range declared {
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
		jsName += " | undefined"
	}

	return "  const " + entry.Name + ": " + jsName + ";\n", nil
}

// renderNamespaceJS binds one namespace out of the global object graph the Go
// side publishes into.
func (g *Generator) renderNamespaceJS(namespace string, bound []Entry) string {
	if len(bound) == 0 {
		return ""
	}

	prefix := "globalThis[" + g.style.quoted("go") + "][" + g.style.quoted(g.appName) + "][" + g.style.quoted(namespace) + "]"

	var out strings.Builder

	out.WriteString("  " + namespace + " = {\n")

	for i, entry := range bound {
		comma := ","
		if !g.style.trailingComma && i == len(bound)-1 {
			comma = ""
		}

		access := prefix + "[" + g.style.quoted(entry.Name) + "]"
		if _, isFunc := entry.Type.(*types.Signature); isFunc {
			access = "wrap(" + access + ")"
		}

		out.WriteString("    " + entry.Name + ": " + access + comma + "\n")
	}

	out.WriteString("  };\n")

	return out.String()
}

func (g *Generator) renderInterface(named *types.Named) (string, error) {
	structType, ok := named.Underlying().(*types.Struct)
	if !ok {
		return "", fmt.Errorf("%s is not a struct", named.Obj().Name())
	}

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

		jsName, optional, err := g.tsType(field.Type())
		if err != nil {
			return "", fmt.Errorf("%s.%s: %w", named.Obj().Name(), field.Name(), err)
		}

		marker := ""
		if optional && !tagHasOption(tag, tagNotNil) {
			marker = "?"
		}

		result.WriteString("    " + field.Name() + marker + ": " + jsName + ";\n")
	}

	methods := make([]*types.Func, 0, named.NumMethods())
	for i := 0; i < named.NumMethods(); i++ {
		if method := named.Method(i); method.Exported() {
			methods = append(methods, method)
		}
	}

	sort.Slice(methods, func(i, j int) bool { return methods[i].Name() < methods[j].Name() })

	for _, method := range methods {
		if g.marks.ignored[markKey(named, method.Name())] {
			continue
		}

		promise := g.marks.promised[markKey(named, method.Name())] || g.isPromise(method)

		signature, err := g.renderSignature(method.Name(), method.Type().(*types.Signature), true, promise)
		if err != nil {
			return "", fmt.Errorf("%s.%s: %w", named.Obj().Name(), method.Name(), err)
		}

		result.WriteString("    " + signature + ";\n")
	}

	result.WriteString("  }\n")

	return result.String(), nil
}

// renderSignature renders a function type. Named signatures become
// "Name(a: T): R"; anonymous ones become "(a: T) => R".
func (g *Generator) renderSignature(name string, sig *types.Signature, named bool, promise bool) (string, error) {
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

			// A channel the caller fills is drained on a goroutine, which
			// cannot happen without yielding to the event loop.
			promise = true
		}

		// A Go call blocks the single JS thread, so a context is the only way
		// to cancel one; AbortSignal is how JS spells the same thing.
		if i == 0 && isContextType(param.Type()) {
			name := param.Name()
			if name == "" || name == "ctx" {
				name = "signal"
			}

			result.WriteString(name + ": AbortSignal")

			promise = true

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

		marker := ": "
		if optional {
			marker = "?: "
		}

		result.WriteString(argName + marker + jsName)

		// A callback parameter forces the whole call to be asynchronous.
		if _, isFunc := param.Type().Underlying().(*types.Signature); isFunc {
			promise = true
		}
	}

	result.WriteString(")")

	if named {
		result.WriteString(": ")
	} else {
		result.WriteString(" => ")
	}

	results, fallible := splitError(sig.Results())

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

	return obj.Pkg() != nil && obj.Pkg().Path() == "context" && obj.Name() == "Context"
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
func (g *Generator) tsType(t types.Type) (string, bool, error) {
	switch typed := t.(type) {
	case *types.Basic:
		return basicToJS(typed)
	case *types.Named, *types.Alias:
		return g.namedToJS(t)
	case *types.Pointer:
		jsName, _, err := g.tsType(typed.Elem())

		return jsName, true, err
	case *types.Slice:
		return g.sequenceToJS(typed.Elem())
	case *types.Array:
		return g.sequenceToJS(typed.Elem())
	case *types.Map:
		return g.mapToJS(typed)
	case *types.Signature:
		jsName, err := g.renderSignature("", typed, false, false)

		return jsName, false, err
	case *types.Interface:
		return "unknown", true, nil
	case *types.Chan:
		if typed.Dir() == types.SendOnly {
			return "", false, errors.New("a send-only channel has no JS counterpart")
		}

		// A receive-only channel and an async iterator are the same idea.
		element, _, err := g.tsType(typed.Elem())
		if err != nil {
			return "", false, err
		}

		return "AsyncIterable<" + element + ">", false, nil
	}

	return "", false, fmt.Errorf("un-convertable type: %s", t)
}

func basicToJS(basic *types.Basic) (string, bool, error) {
	switch basic.Kind() {
	case types.Bool:
		return "boolean", false, nil
	case types.String:
		return "string", false, nil
	case types.Complex64:
		return "", false, errors.New("complex64 cannot be converted to wasm")
	case types.Complex128:
		return "", false, errors.New("complex128 cannot be converted to wasm")
	case types.UnsafePointer:
		return "number", false, nil
	}

	if basic.Info()&types.IsNumeric != 0 {
		return "number", false, nil
	}

	return "", false, fmt.Errorf("un-convertable basic type: %s", basic)
}

func (g *Generator) namedToJS(t types.Type) (string, bool, error) {
	obj := namedObject(t)
	if obj == nil {
		return g.tsType(t.Underlying())
	}

	// error is the one universe-scoped interface with a JS counterpart.
	if obj.Pkg() == nil && obj.Name() == "error" {
		return "Error", false, nil
	}

	if _, ok := t.Underlying().(*types.Struct); ok {
		name := obj.Name()
		if named, ok := t.(*types.Named); ok {
			name = instantiatedName(named)
		}

		if obj.Pkg() == nil {
			return name, false, nil
		}

		return obj.Pkg().Name() + "." + name, false, nil
	}

	// Defined types over a supported underlying type map to that type.
	return g.tsType(t.Underlying())
}

func (g *Generator) sequenceToJS(elem types.Type) (string, bool, error) {
	if basic, ok := elem.Underlying().(*types.Basic); ok && basic.Kind() == types.Uint8 {
		return "Uint8Array", true, nil
	}

	jsName, optional, err := g.tsType(elem)
	if err != nil {
		return "", false, err
	}

	if optional {
		jsName += " | undefined"
	}

	return "Array<" + jsName + ">", true, nil
}

func (g *Generator) mapToJS(typed *types.Map) (string, bool, error) {
	keyName, keyOptional, err := g.tsType(typed.Key())
	if err != nil {
		return "", false, err
	}

	if keyOptional {
		keyName += " | undefined"
	}

	valueName, valueOptional, err := g.tsType(typed.Elem())
	if err != nil {
		return "", false, err
	}

	if valueOptional {
		valueName += " | undefined"
	}

	return "Record<" + keyName + ", " + valueName + ">", true, nil
}

func namedObject(t types.Type) *types.TypeName {
	switch typed := t.(type) {
	case *types.Named:
		return typed.Obj()
	case *types.Alias:
		return typed.Obj()
	}

	return nil
}

// collectNamed walks a type for the named struct types reachable from it,
// recording each one once in declaration-independent order.
func collectNamed(t types.Type, seen map[*types.Named]bool, order *[]*types.Named) {
	switch typed := t.(type) {
	case *types.Named:
		if seen[typed] {
			return
		}

		if _, ok := typed.Underlying().(*types.Struct); !ok {
			return
		}

		seen[typed] = true
		*order = append(*order, typed)

		structType := typed.Underlying().(*types.Struct)
		for i := 0; i < structType.NumFields(); i++ {
			collectNamed(structType.Field(i).Type(), seen, order)
		}

		for i := 0; i < typed.NumMethods(); i++ {
			collectNamed(typed.Method(i).Type(), seen, order)
		}
	case *types.Pointer:
		collectNamed(typed.Elem(), seen, order)
	case *types.Slice:
		collectNamed(typed.Elem(), seen, order)
	case *types.Array:
		collectNamed(typed.Elem(), seen, order)
	case *types.Map:
		collectNamed(typed.Key(), seen, order)
		collectNamed(typed.Elem(), seen, order)
	case *types.Signature:
		for i := 0; i < typed.Params().Len(); i++ {
			collectNamed(typed.Params().At(i).Type(), seen, order)
		}

		for i := 0; i < typed.Results().Len(); i++ {
			collectNamed(typed.Results().At(i).Type(), seen, order)
		}
	}
}
