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

// EntryKind distinguishes the declarations a manifest can make.
type EntryKind string

const (
	EntryFunc    EntryKind = "func"
	EntryValue   EntryKind = "value"
	EntryType    EntryKind = "type"
	EntryIgnore  EntryKind = "ignore"
	EntryPromise EntryKind = "promise"
	EntryPlain   EntryKind = "plain"
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
	plain    map[string]bool
}

func newMarks(declarations Declarations) marks {
	m := marks{
		ignored:  make(map[string]bool),
		promised: make(map[string]bool),
		plain:    make(map[string]bool),
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
		case EntryPlain:
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
