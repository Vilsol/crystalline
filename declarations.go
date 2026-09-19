//go:build !js

package crystalline

import (
	"fmt"
	"go/ast"
	"go/types"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

// exportsDirective marks the function that declares the JS surface. It is
// required rather than inferred from the name, so that a manifest is always
// explicit about what it is.

// What a manifest or directive declares, and the directives themselves.

const exportsDirective = "crystalline:exports"

// exportDirective marks a single declaration for export, as shorthand for a
// manifest entry. It only works for code you own; anything from a dependency
// has to be named in a manifest.
const exportDirective = "crystalline:export"

// promiseDirective marks a declaration as returning a JS Promise.
const promiseDirective = "crystalline:promise"

// entryKind distinguishes the declarations a manifest can make.
type entryKind string

const (
	entryFunc    entryKind = "func"
	entryValue   entryKind = "value"
	entryType    entryKind = "type"
	entryIgnore  entryKind = "ignore"
	entryPromise entryKind = "promise"
	entryPlain   entryKind = "plain"
	entryMarshal entryKind = "marshal"
	entryImport  entryKind = "import"
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

// entry is one declaration read out of a manifest or directive.
type entry struct {
	Kind entryKind

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

	// From is the second symbol an entry names, for a mapping that needs a way
	// back as well as a way out.
	From types.Object

	// Path is where an imported value lives in the JavaScript global object
	// graph.
	Path string

	// Promised names the methods of an imported interface that return a
	// JavaScript promise, and so may be awaited.
	Promised []string

	// Called maps a Go method name onto the name JavaScript knows it by.
	Called map[string]string
}

func (e entry) String() string {
	switch e.Kind {
	case entryImport:
		return "import " + e.Namespace + "." + e.Name + " " + e.Path
	case entryIgnore, entryPromise:
		return string(e.Kind) + " " + e.Namespace + "." + e.Name + "." + e.Method
	case entryFunc:
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
	plain    map[string]bool

	// custom maps a type onto a JS counterpart, keyed like the rest.
	custom map[string]marshaller
}

func newMarks(declarations Declarations) marks {
	m := marks{
		ignored:  make(map[string]bool),
		promised: make(map[string]bool),
		plain:    make(map[string]bool),
		custom:   make(map[string]marshaller),
	}

	for _, entry := range declarations.entries {
		named, ok := entry.Type.(*types.Named)
		if !ok {
			continue
		}

		switch entry.Kind {
		case entryIgnore:
			m.ignored[markKey(named, entry.Method)] = true
		case entryPromise:
			m.promised[markKey(named, entry.Method)] = true
		case entryMarshal:
			m.custom[markKey(named, "")] = marshaller{
				to:           entry.Object,
				from:         entry.From,
				intermediate: entry.Object.Type().(*types.Signature).Results().At(0).Type(),
			}
		case entryPlain:
			// Plainness reaches everything the type contains: a plain value
			// cannot hold a live wrapper, so the whole reachable set converts
			// the same way. The declarations show which types those are.
			for _, reached := range plainReachable(named) {
				m.plain[markKey(reached, "")] = true
			}
		}
	}

	return m
}

// plainReachable collects the structs a plain type contains, following fields
// only. A method's parameter type is not part of the value, so it is not
// dragged in.
func plainReachable(named *types.Named) []*types.Named {
	seen := make(map[*types.Named]bool)
	order := make([]*types.Named, 0, 1)

	var walk func(types.Type)

	walk = func(t types.Type) {
		switch typed := t.(type) {
		case *types.Named:
			structType, ok := typed.Underlying().(*types.Struct)
			if !ok || seen[typed] {
				return
			}

			seen[typed] = true
			order = append(order, typed)

			for i := range structType.NumFields() {
				walk(structType.Field(i).Type())
			}
		case *types.Pointer:
			walk(typed.Elem())
		case *types.Slice:
			walk(typed.Elem())
		case *types.Array:
			walk(typed.Elem())
		case *types.Map:
			walk(typed.Elem())
		}
	}

	walk(named)

	return order
}

// marshallerFor reports how a type crosses, preferring what the manifest
// declared over the standard library defaults.
func (m marks) marshallerFor(t types.Type) (marshaller, bool) {
	t = unaliased(t)

	if named, ok := t.(*types.Named); ok {
		if found, ok := m.custom[markKey(named, "")]; ok {
			return found, true
		}
	}

	return builtinMarshallerFor(t)
}

// isPlain reports whether a type is marshalled as data rather than as a live
// wrapper.
func (m marks) isPlain(named *types.Named) bool {
	return m.plain[markKey(named, "")]
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
//
// What was declared is deliberately not reachable from outside: it is
// go/types objects and generator bookkeeping, and a caller only ever carries
// the whole of it from Declarations to Build and BuildGo.
type Declarations struct {
	Manifests []Manifest

	entries []entry
}

// Empty reports whether anything was declared at all, which is a build worth
// stopping rather than one that writes an empty module.
func (d Declarations) Empty() bool {
	return len(d.Manifests) == 0 && len(d.entries) == 0
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
					out.entries = append(out.entries, entries...)
				case hasDirective(fn.Doc, exportDirective):
					entry, err := directiveEntry(pkg, fn)
					if err != nil {
						return Declarations{}, err
					}

					out.entries = append(out.entries, entry)
				}
			}
		}
	}

	if err := checkNamespaces(out.entries); err != nil {
		return Declarations{}, err
	}

	if err := g.loadReferenced(out.entries); err != nil {
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
func directiveEntry(pkg *packages.Package, fn *ast.FuncDecl) (entry, error) {
	if fn.Recv != nil {
		return entry{}, fmt.Errorf("%s: %s cannot be used on a method, mark the type in a manifest instead", pkg.PkgPath, exportDirective)
	}

	obj := pkg.TypesInfo.Defs[fn.Name]
	if obj == nil {
		return entry{}, fmt.Errorf("%s: could not resolve %s", pkg.PkgPath, fn.Name.Name)
	}

	return entry{
		Kind:      entryFunc,
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
