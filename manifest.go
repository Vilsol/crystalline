//go:build !js

package crystalline

import (
	"fmt"
	"go/ast"
	"go/constant"
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

func (g *Generator) readManifest(pkg *packages.Package, fn *ast.FuncDecl) ([]Entry, error) {
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 || len(fn.Type.Params.List[0].Names) != 1 {
		return nil, fmt.Errorf("%s.%s: a manifest takes exactly one parameter, the bind.Registry", pkg.PkgPath, fn.Name.Name)
	}

	receiver := fn.Type.Params.List[0].Names[0].Name

	var (
		entries []Entry
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

		entry, err := g.readCall(pkg, fn.Name.Name, selector.Sel.Name, call)
		if err != nil {
			failure = err

			return false
		}

		entries = append(entries, entry)

		return true
	})

	return entries, failure
}

func (g *Generator) readCall(pkg *packages.Package, manifest string, method string, call *ast.CallExpr) (Entry, error) {
	where := pkg.PkgPath + "." + manifest + ": r." + method

	switch method {
	case "Func":
		return g.readFunc(pkg, where, call)
	case "Value":
		return g.readValue(pkg, where, call)
	case "Type":
		return g.readType(pkg, where, call, EntryType)
	case "Plain":
		return g.readType(pkg, where, call, EntryPlain)
	case "Ignore":
		return g.readMethodMark(pkg, where, call, EntryIgnore)
	case "Promise":
		return g.readMethodMark(pkg, where, call, EntryPromise)
	}

	return Entry{}, fmt.Errorf("%s is not a Registry method", where)
}

func (g *Generator) readFunc(pkg *packages.Package, where string, call *ast.CallExpr) (Entry, error) {
	if len(call.Args) == 0 {
		return Entry{}, fmt.Errorf("%s needs a function", where)
	}

	obj := referencedObject(pkg, call.Args[0])
	if obj == nil {
		return Entry{}, fmt.Errorf("%s needs a function named directly, so that it can be resolved without running anything", where)
	}

	options := readOptions(pkg, call.Args[1:])

	namespace := obj.Pkg().Name()
	if options.Namespace != "" {
		namespace = options.Namespace
	}

	return Entry{
		Kind:      EntryFunc,
		Namespace: namespace,
		Name:      obj.Name(),
		Type:      obj.Type(),
		Promise:   options.Promise,
		Object:    obj,
	}, nil
}

func (g *Generator) readValue(pkg *packages.Package, where string, call *ast.CallExpr) (Entry, error) {
	if len(call.Args) < 2 {
		return Entry{}, fmt.Errorf("%s needs a name and a value", where)
	}

	name, ok := stringLiteral(pkg, call.Args[0])
	if !ok {
		return Entry{}, fmt.Errorf("%s needs a literal name, so that it can be resolved without running anything", where)
	}

	valueType := pkg.TypesInfo.TypeOf(call.Args[1])
	if valueType == nil {
		return Entry{}, fmt.Errorf("%s: could not determine the type of the value", where)
	}

	options := readOptions(pkg, call.Args[2:])

	namespace := options.Namespace
	if namespace == "" {
		namespace = valueNamespace(valueType, pkg.Name)
	}

	return Entry{
		Kind:              EntryValue,
		NamespaceOverride: options.Namespace,
		Namespace:         namespace,
		Name:              name,
		Type:              valueType,
		Promise:           options.Promise,
	}, nil
}

func (g *Generator) readType(pkg *packages.Package, where string, call *ast.CallExpr, kind EntryKind) (Entry, error) {
	if len(call.Args) == 0 {
		return Entry{}, fmt.Errorf("%s needs a value of the type to declare", where)
	}

	named, err := namedArgument(pkg, where, call.Args[0])
	if err != nil {
		return Entry{}, err
	}

	return Entry{
		Kind:      kind,
		Namespace: named.Obj().Pkg().Name(),
		Name:      named.Obj().Name(),
		Type:      named,
	}, nil
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

func (g *Generator) readMethodMark(pkg *packages.Package, where string, call *ast.CallExpr, kind EntryKind) (Entry, error) {
	if len(call.Args) < 2 {
		return Entry{}, fmt.Errorf("%s needs a value of the type and a method name", where)
	}

	named, err := namedArgument(pkg, where, call.Args[0])
	if err != nil {
		return Entry{}, err
	}

	method, ok := stringLiteral(pkg, call.Args[1])
	if !ok {
		return Entry{}, fmt.Errorf("%s needs a literal method name, so that it can be resolved without running anything", where)
	}

	if !namedHasMethod(named, method) {
		return Entry{}, fmt.Errorf("%s: %s has no exported method %q", where, named, method)
	}

	return Entry{
		Kind:      kind,
		Namespace: named.Obj().Pkg().Name(),
		Name:      named.Obj().Name(),
		Method:    method,
		Type:      named,
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

// readOptions folds the bind.Option arguments of a call. Options are recognised
// by the function being called, so they cannot be hidden behind a variable.
func readOptions(pkg *packages.Package, args []ast.Expr) bind.Options {
	var resolved bind.Options

	for _, arg := range args {
		call, ok := arg.(*ast.CallExpr)
		if !ok {
			continue
		}

		obj := referencedObject(pkg, call.Fun)
		if obj == nil {
			continue
		}

		switch obj.Name() {
		case "AsPromise":
			resolved.Promise = true
		case "InNamespace":
			if len(call.Args) == 1 {
				if name, ok := stringLiteral(pkg, call.Args[0]); ok {
					resolved.Namespace = name
				}
			}
		}
	}

	return resolved
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
func checkNamespaces(entries []Entry) error {
	owners := make(map[string]map[string]bool)

	for _, entry := range entries {
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
