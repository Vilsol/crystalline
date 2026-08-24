//go:build !js

package crystalline

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"github.com/Vilsol/crystalline/bind"
	"golang.org/x/tools/go/packages"
)

// exportsDirective marks the function that declares the JS surface. It is
// required rather than inferred from the name, so that a manifest is always
// explicit about what it is.

// Reading a manifest: the calls it makes on its registry, and the checks that
// keep every one of them resolvable without executing anything.

func (g *Generator) readManifest(pkg *packages.Package, fn *ast.FuncDecl) ([]entry, error) {
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 || len(fn.Type.Params.List[0].Names) != 1 {
		return nil, fmt.Errorf("%s.%s: a manifest takes exactly one parameter, the bind.Registry", pkg.PkgPath, fn.Name.Name)
	}

	receiver := fn.Type.Params.List[0].Names[0].Name

	var (
		entries []entry
		failure error
	)

	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}

		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		ident, ok := selector.X.(*ast.Ident)
		if !ok || ident.Name != receiver {
			return true
		}

		read, err := g.readCall(pkg, fn.Name.Name, selector.Sel.Name, call)
		if err != nil {
			failure = err

			return false
		}

		entries = append(entries, read...)

		return true
	})

	return entries, failure
}

func (g *Generator) readCall(pkg *packages.Package, manifest string, method string, call *ast.CallExpr) ([]entry, error) {
	where := pkg.PkgPath + "." + manifest + ": r." + method

	switch method {
	case "Func":
		entry, err := g.readFunc(pkg, where, call)

		return oneEntry(entry, err)
	case "Value":
		entry, err := g.readValue(pkg, where, call)

		return oneEntry(entry, err)
	case "Type":
		return g.readType(pkg, where, call)
	case "Import":
		entry, err := g.readImport(pkg, where, call)

		return oneEntry(entry, err)
	}

	return nil, fmt.Errorf("%s is not a Registry method", where)
}

func oneEntry(read entry, err error) ([]entry, error) {
	if err != nil {
		return nil, err
	}

	return []entry{read}, nil
}

func (g *Generator) readFunc(pkg *packages.Package, where string, call *ast.CallExpr) (entry, error) {
	if len(call.Args) == 0 {
		return entry{}, fmt.Errorf("%s needs a function", where)
	}

	obj := referencedObject(pkg, call.Args[0])
	if obj == nil {
		return entry{}, fmt.Errorf("%s needs a function named directly, so that it can be resolved without running anything", where)
	}

	options, err := readOptions(pkg, where, call.Args[1:])
	if err != nil {
		return entry{}, err
	}

	if err := options.rejectTypeOnly(where); err != nil {
		return entry{}, err
	}

	namespace := obj.Pkg().Name()
	if options.Namespace != "" {
		namespace = options.Namespace
	}

	return entry{
		Kind:      entryFunc,
		Namespace: namespace,
		Name:      obj.Name(),
		Type:      obj.Type(),
		Promise:   options.Promise,
		Object:    obj,
	}, nil
}

func (g *Generator) readValue(pkg *packages.Package, where string, call *ast.CallExpr) (entry, error) {
	if len(call.Args) < 2 {
		return entry{}, fmt.Errorf("%s needs a name and a value", where)
	}

	name, ok := stringLiteral(pkg, call.Args[0])
	if !ok {
		return entry{}, fmt.Errorf("%s needs a literal name, so that it can be resolved without running anything", where)
	}

	valueType := pkg.TypesInfo.TypeOf(call.Args[1])
	if valueType == nil {
		return entry{}, fmt.Errorf("%s: could not determine the type of the value", where)
	}

	options, err := readOptions(pkg, where, call.Args[2:])
	if err != nil {
		return entry{}, err
	}

	if err := options.rejectTypeOnly(where); err != nil {
		return entry{}, err
	}

	// A promise is a way of returning, and a value does not return. Recording
	// the option and reading it only for function types meant asking for one
	// here compiled, generated and did nothing.
	if _, isFunc := valueType.Underlying().(*types.Signature); options.Promise && !isFunc {
		return entry{}, fmt.Errorf("%s: bind.AsPromise() applies to a function, and %s is not one", where, valueType)
	}

	namespace := options.Namespace
	if namespace == "" {
		namespace = valueNamespace(valueType, pkg.Name)
	}

	return entry{
		Kind:              entryValue,
		NamespaceOverride: options.Namespace,
		Namespace:         namespace,
		Name:              name,
		Type:              valueType,
		Promise:           options.Promise,
	}, nil
}

// readImport reads a variable Go fills from an object JavaScript already has.
//
// The interface is the whole contract. Nothing is generated from a description
// of the JavaScript API, so nothing is generated that nobody asked for, and the
// union types and overloads a real API description is full of never arise.
func (g *Generator) readImport(pkg *packages.Package, where string, call *ast.CallExpr) (entry, error) {
	if len(call.Args) == 0 {
		return entry{}, fmt.Errorf("%s needs a pointer to the variable to fill", where)
	}

	unary, ok := call.Args[0].(*ast.UnaryExpr)
	if !ok || unary.Op != token.AND {
		return entry{}, fmt.Errorf("%s must be a pointer to a variable of interface type, written as &Name", where)
	}

	obj := referencedObject(pkg, unary.X)

	variable, isVar := obj.(*types.Var)
	if !isVar {
		return entry{}, fmt.Errorf("%s must be a pointer to a variable of interface type, so that it can be resolved without running anything", where)
	}

	declared, isInterface := variable.Type().Underlying().(*types.Interface)
	if !isInterface {
		return entry{}, fmt.Errorf("%s must be a pointer to a variable of interface type, and %s is a %s",
			where, variable.Name(), variable.Type())
	}

	if !variable.Exported() {
		return entry{}, fmt.Errorf("%s: %s is unexported, and generated code in another package cannot fill it",
			where, variable.Name())
	}

	if declared.NumMethods() == 0 {
		return entry{}, fmt.Errorf("%s: %s declares no methods, so there is nothing for Go to call",
			where, variable.Type())
	}

	options, err := readOptions(pkg, where, call.Args[1:])
	if err != nil {
		return entry{}, err
	}

	if options.Path == "" {
		return entry{}, fmt.Errorf("%s needs bind.At to say where the value lives, as bind.At(%q)", where, "localStorage")
	}

	if err := options.checkForImport(where); err != nil {
		return entry{}, err
	}

	for _, method := range options.PromiseMethods {
		if !interfaceHasMethod(declared, method) {
			return entry{}, fmt.Errorf("%s: %s has no method %q", where, variable.Type(), method)
		}
	}

	return entry{
		Kind:      entryImport,
		Namespace: variable.Pkg().Name(),
		Name:      variable.Name(),
		Type:      variable.Type(),
		Path:      options.Path,
		Promised:  options.PromiseMethods,
		Object:    variable,
	}, nil
}

// readType reads a declared type and the options that say how it crosses.
//
// One call produces every decision made about the type, so a reader sees them
// together and the generator has one place to check them against each other.
func (g *Generator) readType(pkg *packages.Package, where string, call *ast.CallExpr) ([]entry, error) {
	if len(call.Args) == 0 {
		return nil, fmt.Errorf("%s needs a value of the type to declare", where)
	}

	named, err := namedArgument(pkg, where, call.Args[0])
	if err != nil {
		return nil, err
	}

	options, err := readOptions(pkg, where, call.Args[1:])
	if err != nil {
		return nil, err
	}

	if err := options.checkForType(where, named); err != nil {
		return nil, err
	}

	base := entry{
		Kind:      entryType,
		Namespace: named.Obj().Pkg().Name(),
		Name:      named.Obj().Name(),
		Type:      named,
	}

	switch {
	case options.hasMarshal:
		// A mapped type is not declared as a struct as well: what it crosses
		// as is whatever its functions carry, and declaring both would
		// describe a shape the bindings never publish.
		base, err = g.readMarshal(where, named, options)
		if err != nil {
			return nil, err
		}
	case options.Plain:
		base.Kind = entryPlain
	}

	entries := []entry{base}

	for _, method := range options.Without {
		entries = append(entries, methodMark(named, entryIgnore, method))
	}

	for _, method := range options.PromiseMethods {
		entries = append(entries, methodMark(named, entryPromise, method))
	}

	return entries, nil
}

func methodMark(named *types.Named, kind entryKind, method string) entry {
	return entry{
		Kind:      kind,
		Namespace: named.Obj().Pkg().Name(),
		Name:      named.Obj().Name(),
		Method:    method,
		Type:      named,
	}
}

// valueNamespace picks the namespace an exposed value belongs in.
//
// A value carries no package of its own, so the manifest's package was the
// default. That is almost never what was meant: r.Value("Nodes", api.Nodes)
// belongs beside the rest of api, not beside the manifest, and every consumer
// ended up writing bind.InNamespace on every call to say so. The value's type
// usually names a package, so that is the better default, with the manifest's
// own package as the fallback for types that name none.
//
// Every artifact reads this one answer off the entry, so the declarations, the
// module and the bindings cannot disagree about where a value lives.
func valueNamespace(t types.Type, fallback string) string {
	if name := packageOf(t); name != "" {
		return name
	}

	return fallback
}

// packageOf finds the package a type belongs to, looking through the containers
// that hold one.
//
// What a container holds wins over what it is keyed by, so map[uint32]*api.Node
// and map[api.Kind]string both land in api: a lookup table belongs with the
// thing it describes either way.
func packageOf(t types.Type) string {
	if obj := namedObject(t); obj != nil && obj.Pkg() != nil {
		return obj.Pkg().Name()
	}

	switch typed := t.(type) {
	case *types.Pointer:
		return packageOf(typed.Elem())
	case *types.Slice:
		return packageOf(typed.Elem())
	case *types.Array:
		return packageOf(typed.Elem())
	case *types.Chan:
		return packageOf(typed.Elem())
	case *types.Map:
		if name := packageOf(typed.Elem()); name != "" {
			return name
		}

		return packageOf(typed.Key())
	}

	return ""
}

// readMarshal checks the pair of functions that define a mapping, and turns
// them into the entry that carries it.
//
// The signatures say everything: func(T) X gives the Go type and what it
// crosses as, and func(X) (T, error) gives the way back and the admission that
// it can refuse. Both are checked here rather than at run time, because a
// mapping that does not line up would otherwise emit code that does not
// compile, several steps away from the manifest that asked for it. They are
// checked against the type they were given to as well, since that is a third
// statement of the same fact and nothing else compares them.
func (g *Generator) readMarshal(where string, subject *types.Named, options manifestOptions) (entry, error) {
	to, from := options.marshalTo, options.marshalFrom

	out, ok := to.Type().(*types.Signature)
	if !ok || out.Params().Len() != 1 || out.Results().Len() != 1 {
		return entry{}, fmt.Errorf("%s: %s must take the type and return what it crosses as", where, to.Name())
	}

	back, ok := from.Type().(*types.Signature)
	if !ok || back.Params().Len() != 1 || back.Results().Len() != 2 || !isErrorType(back.Results().At(1).Type()) {
		return entry{}, fmt.Errorf("%s: %s must take what it crosses as and return the type and an error", where, from.Name())
	}

	if !types.Identical(out.Params().At(0).Type(), subject) {
		return entry{}, fmt.Errorf("%s: %s takes %s, but the mapping was declared on %s", where,
			to.Name(), out.Params().At(0).Type(), subject)
	}

	if !types.Identical(subject, back.Results().At(0).Type()) {
		return entry{}, fmt.Errorf("%s: %s returns %s, but %s takes %s", where,
			from.Name(), back.Results().At(0).Type(), to.Name(), subject)
	}

	if !types.Identical(out.Results().At(0).Type(), back.Params().At(0).Type()) {
		return entry{}, fmt.Errorf("%s: %s crosses as %s, but %s reads %s", where,
			to.Name(), out.Results().At(0).Type(), from.Name(), back.Params().At(0).Type())
	}

	return entry{
		Kind:      entryMarshal,
		Namespace: subject.Obj().Pkg().Name(),
		Name:      subject.Obj().Name(),
		Type:      subject,
		Object:    to,
		From:      from,
	}, nil
}

// namedHasMethod reports whether the type or its pointer declares the method.
func namedHasMethod(named *types.Named, method string) bool {
	for i := 0; i < named.NumMethods(); i++ {
		if named.Method(i).Name() == method {
			return true
		}
	}

	return false
}

func namedArgument(pkg *packages.Package, where string, arg ast.Expr) (*types.Named, error) {
	argType := pkg.TypesInfo.TypeOf(arg)
	if argType == nil {
		return nil, fmt.Errorf("%s: could not determine the type", where)
	}

	if pointer, ok := argType.(*types.Pointer); ok {
		argType = pointer.Elem()
	}

	named, ok := argType.(*types.Named)
	if !ok {
		return nil, fmt.Errorf("%s: %s is not a named type", where, argType)
	}

	if named.Obj().Pkg() == nil {
		return nil, fmt.Errorf("%s: %s has no package", where, argType)
	}

	return named, nil
}

// manifestOptions is the resolved form of the options on one registry call.
//
// It carries the declared symbols alongside bind.Options, because a mapping is
// read as a pair of go/types objects here and as a pair of values at run time.
type manifestOptions struct {
	bind.Options

	marshalTo   types.Object
	marshalFrom types.Object
	hasMarshal  bool
}

// readOptions folds the bind.Option arguments of a call. Options are recognised
// by the function being called, so they cannot be hidden behind a variable.
func readOptions(pkg *packages.Package, where string, args []ast.Expr) (manifestOptions, error) {
	var resolved manifestOptions

	for _, arg := range args {
		call, ok := arg.(*ast.CallExpr)
		if !ok {
			return manifestOptions{}, fmt.Errorf("%s needs its options written as calls, so that they can be read without running anything", where)
		}

		obj := referencedObject(pkg, call.Fun)
		if obj == nil {
			return manifestOptions{}, fmt.Errorf("%s needs its options named directly, so that they can be read without running anything", where)
		}

		switch obj.Name() {
		case "AsPromise":
			methods, err := literalNames(pkg, where, obj.Name(), call.Args)
			if err != nil {
				return manifestOptions{}, err
			}

			if len(methods) == 0 {
				resolved.Promise = true
			}

			resolved.PromiseMethods = append(resolved.PromiseMethods, methods...)
		case "Without":
			methods, err := literalNames(pkg, where, obj.Name(), call.Args)
			if err != nil {
				return manifestOptions{}, err
			}

			resolved.Without = append(resolved.Without, methods...)
		case "Plain":
			resolved.Plain = true
		case "At":
			if len(call.Args) != 1 {
				return manifestOptions{}, fmt.Errorf("%s: bind.At needs a path", where)
			}

			path, ok := stringLiteral(pkg, call.Args[0])
			if !ok {
				return manifestOptions{}, fmt.Errorf("%s: bind.At needs a literal path, so that it can be read without running anything", where)
			}

			if !isJSPath(path) {
				return manifestOptions{}, fmt.Errorf("%s: %q is not a path of JavaScript identifiers", where, path)
			}

			resolved.Path = path
		case "InNamespace":
			if len(call.Args) == 1 {
				name, ok := stringLiteral(pkg, call.Args[0])
				if !ok {
					return manifestOptions{}, fmt.Errorf("%s: bind.InNamespace needs a literal name, so that it can be read without running anything", where)
				}

				resolved.Namespace = name
			}
		case "MarshalledBy":
			if len(call.Args) != 2 {
				return manifestOptions{}, fmt.Errorf("%s: bind.MarshalledBy needs a function out and a function back", where)
			}

			to := referencedObject(pkg, call.Args[0])
			from := referencedObject(pkg, call.Args[1])

			if to == nil || from == nil {
				return manifestOptions{}, fmt.Errorf("%s: bind.MarshalledBy needs both functions named directly, so that they can be resolved without running anything", where)
			}

			resolved.marshalTo, resolved.marshalFrom, resolved.hasMarshal = to, from, true
		default:
			return manifestOptions{}, fmt.Errorf("%s: bind.%s is not an option", where, obj.Name())
		}
	}

	return resolved, nil
}

// literalNames reads the method names an option was given.
func literalNames(pkg *packages.Package, where string, option string, args []ast.Expr) ([]string, error) {
	names := make([]string, 0, len(args))

	for _, arg := range args {
		name, ok := stringLiteral(pkg, arg)
		if !ok {
			return nil, fmt.Errorf("%s: bind.%s needs literal method names, so that they can be resolved without running anything", where, option)
		}

		names = append(names, name)
	}

	return names, nil
}

// rejectTypeOnly refuses the options that only mean something for a type,
// rather than accepting them on a function or a value and doing nothing.
func (o manifestOptions) rejectTypeOnly(where string) error {
	switch {
	case o.Plain:
		return fmt.Errorf("%s: bind.Plain() says how a type crosses, so it belongs on r.Type", where)
	case len(o.Without) > 0:
		return fmt.Errorf("%s: bind.Without names methods of a type, so it belongs on r.Type", where)
	case len(o.PromiseMethods) > 0:
		return fmt.Errorf("%s: bind.AsPromise names methods of a type, so it belongs on r.Type; on a function it takes no names", where)
	case o.hasMarshal:
		return fmt.Errorf("%s: bind.MarshalledBy maps a type, so it belongs on r.Type", where)
	}

	return nil
}

// checkForImport refuses the options that say nothing about an import.
func (o manifestOptions) checkForImport(where string) error {
	switch {
	case o.Plain:
		return fmt.Errorf("%s: bind.Plain() says how a type crosses out, and an import comes in", where)
	case o.hasMarshal:
		return fmt.Errorf("%s: bind.MarshalledBy maps a type crossing out, and an import comes in", where)
	case o.Namespace != "":
		return fmt.Errorf("%s: bind.InNamespace places something in JavaScript, and an import is already there", where)
	case len(o.Without) > 0:
		return fmt.Errorf("%s: bind.Without hides a method from JavaScript; an imported interface declares only what Go calls", where)
	case o.Promise:
		return fmt.Errorf("%s: bind.AsPromise() applies to a function; name the methods that return a promise", where)
	}

	return nil
}

// interfaceHasMethod reports whether an interface declares the method.
func interfaceHasMethod(declared *types.Interface, method string) bool {
	for i := range declared.NumMethods() {
		if declared.Method(i).Name() == method {
			return true
		}
	}

	return false
}

// isJSPath reports whether a path is a dotted run of JavaScript identifiers.
func isJSPath(path string) bool {
	if path == "" {
		return false
	}

	for _, segment := range strings.Split(path, ".") {
		if !isJSIdentifier(segment) {
			return false
		}
	}

	return true
}

// checkForType refuses the options that cannot apply to a type, and the
// combinations that contradict each other.
func (o manifestOptions) checkForType(where string, named *types.Named) error {
	switch {
	case o.Promise:
		return fmt.Errorf("%s: %s is not a function; name the methods that return a promise, as bind.AsPromise(%q)", where, named, "Method")
	case o.Namespace != "":
		return fmt.Errorf("%s: bind.InNamespace applies to a function or a value; a type follows the package that declares it", where)
	case o.Plain && o.hasMarshal:
		return fmt.Errorf("%s: bind.Plain() and bind.MarshalledBy say different things about how %s crosses", where, named)
	case o.Plain && len(o.Without)+len(o.PromiseMethods) > 0:
		return fmt.Errorf("%s: %s is plain data, which has no methods to name", where, named)
	case o.hasMarshal && len(o.Without)+len(o.PromiseMethods) > 0:
		return fmt.Errorf("%s: %s is mapped by its own functions, which have no methods to name", where, named)
	}

	for _, method := range append(append([]string{}, o.Without...), o.PromiseMethods...) {
		if !namedHasMethod(named, method) {
			return fmt.Errorf("%s: %s has no exported method %q", where, named, method)
		}
	}

	return nil
}

// referencedObject resolves an expression that names a symbol directly.
func referencedObject(pkg *packages.Package, expr ast.Expr) types.Object {
	switch typed := expr.(type) {
	case *ast.Ident:
		return pkg.TypesInfo.Uses[typed]
	case *ast.SelectorExpr:
		return pkg.TypesInfo.Uses[typed.Sel]
	}

	return nil
}

func stringLiteral(pkg *packages.Package, expr ast.Expr) (string, bool) {
	value := pkg.TypesInfo.Types[expr].Value
	if value == nil || value.Kind() != constant.String {
		return "", false
	}

	return constant.StringVal(value), true
}

// checkNamespaces rejects two different packages claiming one JS namespace,
// which would otherwise silently merge their surfaces.
func checkNamespaces(entries []entry) error {
	owners := make(map[string]map[string]bool)

	for _, entry := range entries {
		// An import is filled from JavaScript rather than published to it, so
		// it claims nothing and cannot collide with anything.
		if entry.Kind == entryImport {
			continue
		}

		// A namespace becomes "export let <name>" in the module, so a name
		// JavaScript cannot spell produces a file that does not parse, found by
		// whoever imports it rather than whoever wrote it.
		if !isJSIdentifier(entry.Namespace) {
			return fmt.Errorf("namespace %q is not a JavaScript identifier: give it a name with bind.InNamespace", entry.Namespace)
		}

		if entry.Object == nil || entry.Object.Pkg() == nil {
			continue
		}

		if owners[entry.Namespace] == nil {
			owners[entry.Namespace] = make(map[string]bool)
		}

		owners[entry.Namespace][entry.Object.Pkg().Path()] = true
	}

	for _, namespace := range sortedKeys(owners) {
		if len(owners[namespace]) < 2 {
			continue
		}

		paths := make([]string, 0, len(owners[namespace]))
		for path := range owners[namespace] {
			paths = append(paths, path)
		}

		sort.Strings(paths)

		return fmt.Errorf("namespace %q is claimed by %s: give one of them an explicit bind.InNamespace", namespace, strings.Join(paths, " and "))
	}

	return nil
}

// isJSIdentifier reports whether a name can be a JavaScript binding.
//
// Deliberately narrower than the language allows: an ASCII identifier is what
// a Go package name already is, so anything else came from an explicit
// bind.InNamespace and is worth refusing rather than escaping.
func isJSIdentifier(name string) bool {
	if name == "" {
		return false
	}

	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_', r == '$':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}

	return !jsReserved[name]
}

// jsReserved are the words a binding cannot be named.
var jsReserved = map[string]bool{
	"break": true, "case": true, "catch": true, "class": true, "const": true,
	"continue": true, "debugger": true, "default": true, "delete": true, "do": true,
	"else": true, "enum": true, "export": true, "extends": true, "false": true,
	"finally": true, "for": true, "function": true, "if": true, "import": true,
	"in": true, "instanceof": true, "new": true, "null": true, "return": true,
	"super": true, "switch": true, "this": true, "throw": true, "true": true,
	"try": true, "typeof": true, "var": true, tsVoid: true, "while": true,
	"with": true, "yield": true, "let": true, "static": true, "await": true,
}
