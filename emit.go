//go:build !js

package crystalline

import (
	"fmt"
	"go/format"
	"go/token"
	"go/types"
	"strconv"
	"strings"
)

// Shape of the generated binding file: the registry the manifests are handed,
// and the wrapper around each exposed function.
// Skipped records something that could not be bound, so that a gap in the
// generated surface is reported rather than silently omitted.
type Skipped struct {
	Name   string
	Reason string

	// Pos is where in the source the subject is declared, empty when there is
	// no position to report.
	Pos string
}

func (s Skipped) String() string {
	return withPosition(s.Pos, s.Name, s.Reason)
}

// withPosition renders a report the way a compiler does, so that an editor or
// CI can turn it into an annotation without being taught the format.
func withPosition(pos string, name string, reason string) string {
	if pos == "" {
		return name + ": " + reason
	}

	return pos + ": " + name + ": " + reason
}

// Warning records something that was bound, but with a cost worth knowing
// about. It is distinct from Skipped, which is something that was not bound at
// all.
type Warning struct {
	Name   string
	Reason string

	// Pos is where in the source the subject is declared, empty when there is
	// no position to report.
	Pos string
}

func (w Warning) String() string {
	return withPosition(w.Pos, w.Name, w.Reason)
}

// wideIntegers reports the 64-bit integer types reachable from t.
//
// A JavaScript number is a double, so an int64 beyond 2^53 is rounded rather
// than carried. Refusing the type would break ordinary Go, where identifiers
// and timestamps are routinely int64, so it is bound and the cost is reported.
//
// Plain int is 64 bits on wasm too, but warning about every int would drown the
// signal: someone who wrote int64 chose the range deliberately.
func wideIntegers(m marks, t types.Type, seen map[types.Type]bool) []string {
	if t == nil || seen[t] {
		return nil
	}

	seen[t] = true

	// A mapped type crosses as its counterpart, so its underlying width is not
	// what anybody receives.
	if _, mapped := m.marshallerFor(t); mapped {
		return nil
	}

	switch typed := t.(type) {
	case *types.Basic:
		if typed.Kind() == types.Int64 || typed.Kind() == types.Uint64 {
			return []string{typed.Name()}
		}
	case *types.Named, *types.Alias:
		return wideIntegers(m, t.Underlying(), seen)
	case *types.Pointer:
		return wideIntegers(m, typed.Elem(), seen)
	case *types.Slice:
		return wideIntegers(m, typed.Elem(), seen)
	case *types.Array:
		return wideIntegers(m, typed.Elem(), seen)
	case *types.Chan:
		return wideIntegers(m, typed.Elem(), seen)
	case *types.Map:
		return append(wideIntegers(m, typed.Key(), seen), wideIntegers(m, typed.Elem(), seen)...)
	case *types.Struct:
		var found []string

		for i := range typed.NumFields() {
			found = append(found, wideIntegers(m, typed.Field(i).Type(), seen)...)
		}

		return found
	case *types.Signature:
		var found []string

		for i := range typed.Params().Len() {
			found = append(found, wideIntegers(m, typed.Params().At(i).Type(), seen)...)
		}

		for i := range typed.Results().Len() {
			found = append(found, wideIntegers(m, typed.Results().At(i).Type(), seen)...)
		}

		return found
	}

	return nil
}

// GoBindings is the generated binding source for one package.
type GoBindings struct {
	// Package is the name of the package the source belongs to.
	Package string

	// Source is formatted Go, to be written next to the package it binds.
	Source string

	// Skipped lists everything that could not be bound.
	Skipped []Skipped

	// Warnings lists what was bound at a cost worth knowing about.
	Warnings []Warning
}

// BuildGo emits reflect-free binding source for everything the loaded manifests
// and directives declare.
//
// packageName is the package the generated file belongs to, and selfPath its
// import path; both refer to a package the consumer owns, since generated code
// cannot be written into a dependency.
//
// The generated code registers through syscall/js directly, so a binary built
// from it does not link reflect at all.
func (g *Generator) BuildGo(declarations Declarations, packageName string, selfPath string) (GoBindings, error) {
	e := newEmitter(g, packageName, selfPath, declarations)

	source, err := e.emitBindings(declarations)
	if err != nil {
		return GoBindings{}, err
	}

	formatted, err := format.Source([]byte(source))
	if err != nil {
		return GoBindings{}, fmt.Errorf("generated source is not valid Go: %w\n%s", err, source)
	}

	return GoBindings{
		Package:  packageName,
		Source:   string(formatted),
		Skipped:  e.skipped,
		Warnings: wideIntegerWarnings(e.marks, declarations, droppedNames(e.skipped), g.position),
	}, nil
}

func newEmitter(g *Generator, packageName string, selfPath string, declarations Declarations) *emitter {
	return &emitter{
		gen:     g,
		pkgName: packageName,
		imports: newImports(selfPath),
		marks:   newMarks(declarations),
	}
}

// analysis is what rendering the declarations needs to know about the Go side:
// what could not be bound, and which fields can be read but not written.
//
// It comes from the same pass that emits the bindings, so the declarations
// cannot promise something the bindings do not do. Keeping the two apart is how
// they drifted before.
type analysis struct {
	skipped  []Skipped
	readonly map[string]bool

	// dropped names everything the bindings left out, so the declarations can
	// leave out the same things. Describing a member the bindings never
	// published gave a wrap(undefined) and a TypeError at the call.
	dropped map[string]bool

	// warnings are the costs of what was bound, as opposed to what was not.
	warnings []Warning
}

func (g *Generator) analyse(declarations Declarations) (analysis, error) {
	e := newEmitter(g, "crystalline", "", declarations)

	if _, err := e.emitBindings(declarations); err != nil {
		return analysis{}, err
	}

	dropped := droppedNames(e.skipped)

	return analysis{
		skipped:  e.skipped,
		readonly: e.readonly,
		dropped:  dropped,
		warnings: wideIntegerWarnings(e.marks, declarations, dropped, g.position),
	}, nil
}

// entryPos is where a declared entry was written, when it names a symbol.
func entryPos(entry Entry) token.Pos {
	if entry.Object == nil {
		return token.NoPos
	}

	return entry.Object.Pos()
}

// droppedNames indexes a skip report by the member it names.
func droppedNames(skipped []Skipped) map[string]bool {
	dropped := make(map[string]bool, len(skipped))

	for _, one := range skipped {
		dropped[one.Name] = true
	}

	return dropped
}

// wideIntegerWarnings names each bound member that traffics in 64-bit integers.
func wideIntegerWarnings(m marks, declarations Declarations, dropped map[string]bool, position func(token.Pos) string) []Warning {
	var warnings []Warning

	said := make(map[string]bool)

	for _, entry := range declarations.Entries {
		name := entry.Namespace + "." + entry.Name
		if dropped[name] || said[name] {
			continue
		}

		found := wideIntegers(m, entry.Type, make(map[types.Type]bool))
		if len(found) == 0 {
			continue
		}

		said[name] = true

		warnings = append(warnings, Warning{
			Name:   name,
			Reason: found[0] + " is bound as a JavaScript number, which cannot represent values beyond 2^53 exactly",
			Pos:    position(entryPos(entry)),
		})
	}

	return warnings
}

type emitter struct {
	gen     *Generator
	pkgName string
	imports *imports
	skipped []Skipped

	// marks carries the method decisions a manifest declared.
	marks marks

	// streamStop names the teardown a returned channel takes over, empty when
	// the call has nothing that outlives it.
	streamStop string

	// readonly records the fields that can be read but not written, so the
	// declarations can say so rather than promising a write that throws.
	readonly map[string]bool

	// marshallers holds the generated struct marshal helpers, keyed by type
	// name so each is emitted once.
	marshallers map[string]string
	pending     []*types.Named

	converters map[string]string
}

// skipAt records something that could not be bound, along with where it is
// declared.
func (e *emitter) skipAt(pos token.Pos, name string, reason string) {
	e.skipped = append(e.skipped, Skipped{Name: name, Reason: reason, Pos: e.gen.position(pos)})
}

// emitBindings renders the whole generated file.
func (e *emitter) emitBindings(declarations Declarations) (string, error) {
	e.marshallers = make(map[string]string)
	e.converters = make(map[string]string)
	e.readonly = make(map[string]bool)

	var registrations, wrappers, values strings.Builder

	for _, entry := range declarations.Entries {
		switch entry.Kind {
		case EntryFunc:
			wrapper, err := e.emitDeclaredFunc(entry)
			if err != nil {
				e.skipAt(entryPos(entry), entry.Namespace+"."+entry.Name, err.Error())

				continue
			}

			registrations.WriteString("\tcrystallineNamespace(" + strconv.Quote(e.gen.appName) + ", " + strconv.Quote(entry.Namespace) + ").Set(" +
				strconv.Quote(entry.Name) + ", crystallineWrap(js.FuncOf(" + wrapperName(entry) + ")))\n")
			wrappers.WriteString(wrapper)
		case EntryValue:
			branch, err := e.emitDeclaredValue(entry)
			if err != nil {
				e.skipAt(entryPos(entry), entry.Namespace+"."+entry.Name, err.Error())

				continue
			}

			values.WriteString(branch)
		case EntryType, EntryPlain:
			if named, ok := entry.Type.(*types.Named); ok {
				e.queue(named)
			}
		}
	}

	// An enum's constants are published as one object, so a caller can name a
	// value instead of writing the number the type happens to use.
	seenEnums := make(map[*types.Named]bool)

	for _, entry := range declarations.Entries {
		reached := make([]*types.Named, 0)
		collectNamed(e.marks, entry.Type, seenEnums, &reached)

		for _, named := range reached {
			constants := enumConstants(named)
			if len(constants) == 0 {
				continue
			}

			registrations.WriteString(e.emitEnum(named, constants))
		}
	}

	// Marshallers can queue further marshallers, so drain until stable.
	for len(e.pending) > 0 {
		named := e.pending[0]
		e.pending = e.pending[1:]

		if _, done := e.marshallers[instantiatedName(named)]; done {
			continue
		}

		e.marshallers[instantiatedName(named)] = ""

		marshal := e.emitMarshaller
		if e.marks.isPlain(named) {
			marshal = e.emitPlainMarshaller
		}

		body, err := marshal(named)
		if err != nil {
			e.skipAt(named.Obj().Pos(), named.Obj().Name(), err.Error())

			continue
		}

		e.marshallers[instantiatedName(named)] = body
	}

	var helpers strings.Builder

	for _, name := range sortedKeys(e.marshallers) {
		helpers.WriteString(e.marshallers[name])
	}

	for _, name := range sortedKeys(e.converters) {
		helpers.WriteString(e.converters[name])
	}

	// The manifests run last, so that every function is already bound by the
	// time a manifest hands over its values.
	var manifests strings.Builder

	for _, manifest := range declarations.Manifests {
		// Generating next to the manifest is the natural layout, and importing
		// your own package is a cycle, so call it unqualified there.
		call := manifest.Name
		if manifest.Package != e.imports.self {
			call = e.imports.add(manifest.Package, lastSegment(manifest.Package)) + "." + call
		}

		manifests.WriteString("\t" + call + "(crystallineRegistry{})\n")
	}

	var out strings.Builder

	out.WriteString("//go:build js\n\n")
	out.WriteString("// Code generated by crystalline. DO NOT EDIT.\n\n")
	out.WriteString("package " + e.pkgName + "\n\n")

	var body strings.Builder

	body.WriteString("func init() {\n")
	body.WriteString(registrations.String())

	if manifests.Len() > 0 {
		body.WriteString("\n")
		body.WriteString(manifests.String())
	}

	body.WriteString("}\n\n")
	body.WriteString(e.emitRegistry(values.String()))
	body.WriteString(prelude)
	body.WriteString(wrappers.String())
	body.WriteString(helpers.String())

	// Imports are collected while rendering, so the block is written last.
	out.WriteString(e.imports.block([]string{contextPackage, "errors", "strconv", "sync", "syscall/js"}))
	out.WriteString(body.String())

	return out.String(), nil
}

// emitRegistry renders the bind.Registry implementation the manifests are
// handed. Functions and marks are resolved statically, so only Value carries
// anything through at run time.
func (e *emitter) emitRegistry(values string) string {
	bindAlias := e.imports.add("github.com/Vilsol/crystalline/bind", "bind")

	var out strings.Builder

	out.WriteString("// crystallineRegistry receives what the manifests declare. Everything but a\n")
	out.WriteString("// value is already bound above, so the rest are no-ops.\n")
	out.WriteString("type crystallineRegistry struct{}\n\n")
	out.WriteString("func (crystallineRegistry) Func(fn any, opts ..." + bindAlias + ".Option) {}\n\n")
	out.WriteString("func (crystallineRegistry) Plain(zero any) {}\n\n")
	out.WriteString("func (crystallineRegistry) Marshal(to any, from any) {}\n\n")
	out.WriteString("func (crystallineRegistry) Type(zero any) {}\n\n")
	out.WriteString("func (crystallineRegistry) Ignore(zero any, method string) {}\n\n")
	out.WriteString("func (crystallineRegistry) Promise(zero any, method string) {}\n\n")

	out.WriteString("func (crystallineRegistry) Value(name string, value any, opts ..." + bindAlias + ".Option) {\n")

	if values == "" {
		out.WriteString("}\n\n")

		return out.String()
	}

	out.WriteString("\tresolved := " + bindAlias + ".Resolve(opts)\n\n")
	out.WriteString("\tswitch resolved.Namespace + \"|\" + name {\n")
	out.WriteString(values)
	out.WriteString("\t}\n}\n\n")

	return out.String()
}

// emitEnum publishes an enum's constants as a single frozen object.
func (e *emitter) emitEnum(named *types.Named, constants []*types.Const) string {
	var out strings.Builder

	out.WriteString("\tcrystallineNamespace(" + strconv.Quote(e.gen.appName) + ", " +
		strconv.Quote(named.Obj().Pkg().Name()) + ").Set(" + strconv.Quote(named.Obj().Name()) +
		", map[string]any{\n")

	for _, declared := range constants {
		out.WriteString("\t\t" + strconv.Quote(declared.Name()) + ": " + enumJSValue(declared) + ",\n")
	}

	out.WriteString("\t})\n")

	return out.String()
}

// emitDeclaredValue renders the branch that publishes one declared value. The
// key pairs the namespace override with the name, which is exactly what the
// registry can reconstruct at run time.
func (e *emitter) emitDeclaredValue(entry Entry) (string, error) {
	converted, err := e.toJS("typed", entry.Type, false)
	if err != nil {
		return "", err
	}

	goType := types.TypeString(entry.Type, e.qualifier)

	var out strings.Builder

	out.WriteString("\tcase " + strconv.Quote(entry.NamespaceOverride+"|"+entry.Name) + ":\n")
	out.WriteString("\t\ttyped, ok := value.(" + goType + ")\n")
	out.WriteString("\t\tif !ok {\n\t\t\tcrystallineFail(" + strconv.Quote(entry.Name+": expected "+goType) + ")\n\n\t\t\treturn\n\t\t}\n\n")
	out.WriteString("\t\tcrystallineNamespace(" + strconv.Quote(e.gen.appName) + ", " + strconv.Quote(entry.Namespace) + ").Set(" +
		strconv.Quote(entry.Name) + ", " + converted + ")\n")

	return out.String(), nil
}

func lastSegment(path string) string {
	if index := strings.LastIndex(path, "/"); index >= 0 {
		return path[index+1:]
	}

	return path
}

func wrapperName(entry Entry) string {
	namespace := sanitiseAlias(entry.Namespace)
	if namespace != "" {
		namespace = strings.ToUpper(namespace[:1]) + namespace[1:]
	}

	return "crystallineFn" + namespace + entry.Name
}

func marshalName(name string) string {
	return "crystallineMarshal" + name
}

// declaredName renders a type the way generated code must spell it.
func (e *emitter) declaredName(t types.Type) string {
	return types.TypeString(t, e.qualifier)
}

func (e *emitter) emitDeclaredFunc(entry Entry) (string, error) {
	fn, ok := entry.Object.(*types.Func)
	if !ok {
		return "", fmt.Errorf("%s is not a function", entry.Name)
	}

	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return "", fmt.Errorf("%s is not a function", entry.Name)
	}

	var body strings.Builder

	body.WriteString("func " + wrapperName(entry) + "(this js.Value, args []js.Value) (result any) {\n")
	body.WriteString("\tdefer crystallineRecover(&result)\n\n")
	body.WriteString("\tif len(args) != " + strconv.Itoa(sig.Params().Len()) + " {\n")
	body.WriteString("\t\treturn crystallineFail(" + strconv.Quote(fn.Name()+": expected "+strconv.Itoa(sig.Params().Len())+" arguments, got ") + " + strconv.Itoa(len(args)))\n")
	body.WriteString("\t}\n\n")

	returns, err := e.emitCall(sig, func(call []string) string {
		return qualified(e.qualifier(fn.Pkg()), fn.Name()) + "(" + spread(sig, call) + ")"
	})
	if err != nil {
		return "", err
	}

	body.WriteString(wrapPromise(returns, entry.Promise || e.gen.isPromise(fn) || asyncSignature(sig), 1))
	body.WriteString("}\n\n")

	return body.String(), nil
}

// wrapPromise defers a rendered body onto a goroutine when the member is
// asynchronous, matching what the declarations promise.
// hasCallback reports whether the signature takes a function, which cannot be
// serviced without yielding to the JS event loop.
// spread renders a call's arguments, expanding the final slice for a variadic
// function.
//
// A variadic parameter is an ordinary slice on this side of the boundary, and
// JavaScript passes it as an array. Go still needs it spread at the call site;
// passing it whole does not compile, which is what the generated file used to
// do for every variadic function.
func spread(sig *types.Signature, call []string) string {
	joined := strings.Join(call, ", ")

	if sig.Variadic() && len(call) > 0 {
		joined += "..."
	}

	return joined
}

// qualified joins a package qualifier to a name.
//
// The qualifier is empty for the package the generated file itself belongs to,
// which is the layout the export directive implies: bindings written next to
// the code they bind. Joining with a dot regardless produced ".Owned()", so the
// generated file did not compile.
func qualified(qualifier string, name string) string {
	if qualifier == "" {
		return name
	}

	return qualifier + "." + name
}

// asyncSignature reports whether a call has to be a promise whatever the
// manifest asked for.
//
// A callback and a channel parameter both need the event loop to turn before
// the call can finish, and a context says the call is long enough to be worth
// interrupting. A call that returns a channel is the exception: it hands the
// stream over and returns, and the stream is already asynchronous, so wrapping
// it only makes the caller await something with nothing to wait for.
//
// The declarations and the bindings both read this one answer, so they cannot
// disagree about which calls are promises.
func asyncSignature(sig *types.Signature) bool {
	if hasCallback(sig) || takesChannel(sig) {
		return true
	}

	return takesContext(sig) && !returnsChannel(sig)
}

// takesContext reports whether the signature starts with a context, which
// forces the call to be asynchronous so the signal can interrupt it.
func takesContext(sig *types.Signature) bool {
	return sig.Params().Len() > 0 && isContextType(sig.Params().At(0).Type())
}

// takesChannel reports whether the signature takes a channel the caller fills,
// which is drained on a goroutine and so cannot be serviced synchronously.
func takesChannel(sig *types.Signature) bool {
	for i := 0; i < sig.Params().Len(); i++ {
		if channel, ok := sig.Params().At(i).Type().Underlying().(*types.Chan); ok && channel.Dir() == types.RecvOnly {
			return true
		}
	}

	return false
}

func hasCallback(sig *types.Signature) bool {
	for i := 0; i < sig.Params().Len(); i++ {
		if _, ok := sig.Params().At(i).Type().Underlying().(*types.Signature); ok {
			return true
		}
	}

	return false
}

func wrapPromise(returns string, promise bool, depth int) string {
	if !promise {
		return returns
	}

	indent := strings.Repeat("\t", depth)

	inner := strings.ReplaceAll(strings.TrimRight(returns, "\n"), "\n", "\n\t")

	return indent + "return crystallinePromise(func() any {\n" + inner + "\n" + indent + "})\n"
}

// emitCall renders the argument conversions, the call itself and the conversion
// of its results, with the preamble folded in.
//
// The preamble sets up feeds and cancellation, so it belongs inside the promise
// body: left in the wrapper, its deferred teardown would run the moment the
// promise was handed back.
//
// A context normally lives exactly as long as the call. When the result is a
// stream, the caller reads it after the call has returned, so cancelling on the
// way out would end the stream before anything had been read from it. The
// cancellation is handed to the stream instead, which runs it once the stream
// ends however it ends.
func (e *emitter) emitCall(sig *types.Signature, invoke func(call []string) string) (string, error) {
	streaming := takesContext(sig) && returnsChannel(sig)

	call, preamble, checks, err := e.emitArguments(sig, !streaming)
	if err != nil {
		return "", err
	}

	if streaming {
		e.streamStop = "crystallineStop"
		defer func() { e.streamStop = "" }()
	}

	// Nothing takes the cancellation over if the call fails before handing back
	// a stream, so that path undoes it here.
	onFailure := ""
	if streaming {
		onFailure = "crystallineStop()\n\t\t\t"
	}

	returns, err := e.emitReturn(invoke(call), sig.Results(), checks, onFailure)
	if err != nil {
		return "", err
	}

	return preamble + returns, nil
}

// returnsChannel reports whether the call hands back a stream, which the caller
// reads after the call itself has returned.
func returnsChannel(sig *types.Signature) bool {
	results := sig.Results()

	for i := range results.Len() {
		if _, ok := results.At(i).Type().Underlying().(*types.Chan); ok {
			return true
		}
	}

	return false
}

// emitArguments renders the conversion of every parameter, plus any preamble
// the conversions need.
func (e *emitter) emitArguments(sig *types.Signature, deferStop bool) ([]string, string, string, error) {
	call := make([]string, 0, sig.Params().Len())

	var preamble, checks strings.Builder

	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)

		// A leading context is driven by an AbortSignal rather than converted.
		if i == 0 && isContextType(param.Type()) {
			preamble.WriteString("\tcrystallineCtx, crystallineStop := crystallineContext(args[0])\n")

			if deferStop {
				preamble.WriteString("\tdefer crystallineStop()\n\n")
			} else {
				preamble.WriteString("\n")
			}

			call = append(call, "crystallineCtx")

			continue
		}

		if channel, ok := param.Type().Underlying().(*types.Chan); ok {
			if err := checkChannelParameter(channel, param.Name()); err != nil {
				return nil, "", "", err
			}

			converter, err := e.ensureValueConverter(channel.Elem())
			if err != nil {
				return nil, "", "", fmt.Errorf("channel element: %w", err)
			}

			name := "crystallineFeed" + strconv.Itoa(i)

			preamble.WriteString("\t" + name + ", " + name + "Stop := crystallineFeed(args[" + strconv.Itoa(i) + "], " + converter + ")\n")
			preamble.WriteString("\tdefer " + name + "Stop()\n\n")

			// Checked after the call rather than deferred, so a value the
			// channel could not carry fails the call instead of arriving as an
			// early end of input. Stopping twice is harmless.
			// Panics rather than reporting through the error slot: the slot is
			// read synchronously by the JS wrapper, and a channel parameter
			// always makes the call a promise, so the failure would surface on
			// an unrelated later call. A panic is recovered into a thrown error
			// or a rejection, whichever the call is.
			checks.WriteString("\tif err := " + name + "Stop(); err != nil {\n\t\tpanic(" +
				strconv.Quote(param.Name()+": ") + " + err.Error())\n\t}\n\n")

			call = append(call, name)

			continue
		}

		expr, err := e.fromJS("args["+strconv.Itoa(i)+"]", param.Type())
		if err != nil {
			return nil, "", "", err
		}

		call = append(call, expr)
	}

	return call, preamble.String(), checks.String(), nil
}

// emitReturn renders the call and the conversion of its results.
//
// A trailing error becomes a rejection rather than a returned value, matching
// what the declarations promise.
// emitReturn renders the call and the conversion of its results. checks are
// emitted between the two, for anything that can only be judged once the call
// has finished.
func (e *emitter) emitReturn(invocation string, results *types.Tuple, checks string, onFailure string) (string, error) {
	values, fallible := splitError(results)

	total := len(values)
	if fallible {
		total++
	}

	if total == 0 {
		return "\t" + invocation + "\n\n" + checks + "\treturn nil\n", nil
	}

	names := make([]string, total)
	for i := range names {
		names[i] = "r" + strconv.Itoa(i)
	}

	var body strings.Builder

	body.WriteString("\t" + strings.Join(names, ", ") + " := " + invocation + "\n\n")
	body.WriteString(checks)

	if fallible {
		body.WriteString("\tif " + names[total-1] + " != nil {\n\t\t" + onFailure + "return crystallineErr(" + names[total-1] + ")\n\t}\n\n")
	}

	if len(values) == 0 {
		if fallible {
			body.WriteString("\treturn crystallineOk(nil)\n")
		} else {
			body.WriteString("\treturn nil\n")
		}

		return body.String(), nil
	}

	converted := make([]string, len(values))

	for i, value := range values {
		expr, err := e.toJS(names[i], value.Type(), false)
		if err != nil {
			return "", err
		}

		converted[i] = expr
	}

	produced := converted[0]
	if len(values) > 1 {
		produced = "[]any{" + strings.Join(converted, ", ") + "}"
	}

	if fallible {
		body.WriteString("\treturn crystallineOk(" + produced + ")\n")

		return body.String(), nil
	}

	body.WriteString("\treturn " + produced + "\n")

	return body.String(), nil
}

// toJS renders a Go expression producing a value js.ValueOf accepts. nonNil
// makes an empty collection stand in for a nil one, honouring the not_nil tag.
