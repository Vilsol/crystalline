//go:build !js

package crystalline

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/types"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/Vilsol/crystalline/bind"
)

// exportsDirective marks the function that declares the JS surface. It is
// required rather than inferred from the name, so that a manifest is always
// explicit about what it is.
const exportsDirective = "crystalline:exports"

// exportDirective marks a single declaration for export, as shorthand for a
// manifest entry. It only works for code you own; anything from a dependency
// has to be named in a manifest.
const exportDirective = "crystalline:export"

// promiseDirective marks a declaration as returning a JS Promise.
const promiseDirective = "crystalline:promise"

// EntryKind distinguishes the declarations a manifest can make.
type EntryKind string

const (
	EntryFunc    EntryKind = "func"
	EntryValue   EntryKind = "value"
	EntryType    EntryKind = "type"
	EntryIgnore  EntryKind = "ignore"
	EntryPromise EntryKind = "promise"
)

// hasDirective reports whether a doc comment carries the given directive.
//
// It scans the raw comments rather than the rendered text: Go treats a comment
// of the form //tool:name as a directive and CommentGroup.Text strips it, so a
// directive is invisible to anything reading the rendered form. Both the
// directive spelling and the spaced comment spelling are accepted.
func hasDirective(doc *ast.CommentGroup, name string) bool {
	if doc == nil {
		return false
	}

	for _, comment := range doc.List {
		text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
		if text == name {
			return true
		}
	}

	return false
}

// Entry is one declaration read out of a manifest or directive.
type Entry struct {
	Kind EntryKind

	// Namespace is the JS namespace the entry lands in.
	Namespace string

	// Name is the JS name of the entity, or for Ignore and Promise the name of
	// the type whose method is being marked.
	Name string

	// Method is the method being marked, for Ignore and Promise.
	Method string

	// Type is the Go type of the entity.
	Type types.Type

	// Promise records whether the entity was marked asynchronous.
	Promise bool

	// NamespaceOverride is the literal bind.InNamespace value, empty when none
	// was given. Generated code keys values on it because that is what a
	// registry can reconstruct at run time.
	NamespaceOverride string

	// Object is the declared symbol, for entries that name one.
	Object types.Object
}

func (e Entry) String() string {
	switch e.Kind {
	case EntryIgnore, EntryPromise:
		return string(e.Kind) + " " + e.Namespace + "." + e.Name + "." + e.Method
	case EntryFunc:
		out := "func " + e.Namespace + "." + e.Name
		if e.Promise {
			out += " promise"
		}

		return out
	default:
		out := string(e.Kind) + " " + e.Namespace + "." + e.Name
		if e.Type != nil {
			out += " " + types.TypeString(e.Type, nil)
		}

		if e.Promise {
			out += " promise"
		}

		return out
	}
}

// marks are the method-level decisions a manifest made, keyed independently of
// which load produced the type.
type marks struct {
	ignored  map[string]bool
	promised map[string]bool
}

func newMarks(declarations Declarations) marks {
	m := marks{
		ignored:  make(map[string]bool),
		promised: make(map[string]bool),
	}

	for _, entry := range declarations.Entries {
		named, ok := entry.Type.(*types.Named)
		if !ok {
			continue
		}

		switch entry.Kind {
		case EntryIgnore:
			m.ignored[markKey(named, entry.Method)] = true
		case EntryPromise:
			m.promised[markKey(named, entry.Method)] = true
		}
	}

	return m
}

func markKey(named *types.Named, method string) string {
	path := ""
	if named.Obj().Pkg() != nil {
		path = named.Obj().Pkg().Path()
	}

	return path + "." + named.Obj().Name() + "." + method
}

// Manifest is a function marked with the exports directive.
type Manifest struct {
	// Package is the import path of the package declaring it.
	Package string

	// PackageName is that package's name, which the generated file adopts when
	// it is written alongside the manifest.
	PackageName string

	// Dir is the directory the package lives in, so generated code can be
	// written next to it.
	Dir string

	// Name is the function name, which generated code calls at start-up.
	Name string
}

// Declarations is everything the generator learned about the intended JS
// surface, before any code is emitted.
type Declarations struct {
	Manifests []Manifest
	Entries   []Entry
}

// Declarations reads every manifest and export directive in the loaded
// packages.
func (g *Generator) Declarations() (Declarations, error) {
	var out Declarations

	for _, pkg := range g.roots {
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Doc == nil {
					continue
				}

				switch {
				case hasDirective(fn.Doc, exportsDirective):
					entries, err := g.readManifest(pkg, fn)
					if err != nil {
						return Declarations{}, err
					}

					out.Manifests = append(out.Manifests, Manifest{
						Package:     pkg.PkgPath,
						PackageName: pkg.Name,
						Dir:         packageDir(pkg),
						Name:        fn.Name.Name,
					})
					out.Entries = append(out.Entries, entries...)
				case hasDirective(fn.Doc, exportDirective):
					entry, err := directiveEntry(pkg, fn)
					if err != nil {
						return Declarations{}, err
					}

					out.Entries = append(out.Entries, entry)
				}
			}
		}
	}

	if err := checkNamespaces(out.Entries); err != nil {
		return Declarations{}, err
	}

	if err := g.loadReferenced(out.Entries); err != nil {
		return Declarations{}, err
	}

	return out, nil
}

// packageDir locates a package on disk, so generated code can be written
// beside the manifest that declared it.
func packageDir(pkg *packages.Package) string {
	if len(pkg.GoFiles) == 0 {
		return ""
	}

	return filepath.Dir(pkg.GoFiles[0])
}

// directiveEntry turns a directive-marked function into an entry.
func directiveEntry(pkg *packages.Package, fn *ast.FuncDecl) (Entry, error) {
	if fn.Recv != nil {
		return Entry{}, fmt.Errorf("%s: %s cannot be used on a method, mark the type in a manifest instead", pkg.PkgPath, exportDirective)
	}

	obj := pkg.TypesInfo.Defs[fn.Name]
	if obj == nil {
		return Entry{}, fmt.Errorf("%s: could not resolve %s", pkg.PkgPath, fn.Name.Name)
	}

	return Entry{
		Kind:      EntryFunc,
		Namespace: pkg.Name,
		Name:      fn.Name.Name,
		Type:      obj.Type(),
		Promise:   hasDirective(fn.Doc, promiseDirective),
		Object:    obj,
	}, nil
}

// readManifest walks a manifest body for calls on its registry parameter.
//
// Only the calls are interpreted; everything around them is ordinary Go that
// runs at start-up. The static type of each argument is all the generator
// needs, which is why locals, loops and conversions require no support here.
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
		namespace = pkg.Name
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
