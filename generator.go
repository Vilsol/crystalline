//go:build !js

package crystalline

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Generator reads Go source and emits the bindings for it, without running the
// program being bound.
//
// Walking go/types rather than reflect is what makes parameter names, doc
// comment directives and generic type arguments available at all: none of them
// survive into a value at run time.

type Generator struct {
	appName string
	style   jsStyle
	dir     string

	// roots are the packages Load was asked for, which is where manifests and
	// directives are looked for. pkgs additionally holds the packages those
	// declarations reach into, loaded for their doc comments.
	roots []*packages.Package
	pkgs  []*packages.Package

	// banner is prepended to both generated files, for the pragmas a project's
	// linters and formatters expect at the top of generated sources.
	banner string

	// profile counts and times every call made through the module.
	profile bool

	// marks carries the method decisions the current build declared.
	// fset renders the positions go/packages records, so a report can say
	// where in the source a problem is rather than only which symbol.
	fset *token.FileSet

	marks marks

	// readonly names the fields that can be read but not written, filled in
	// by the analysis Build runs before rendering.
	readonly map[string]bool

	// usesResult records whether anything rendered can fail, so that a surface
	// with nothing fallible does not carry a Result declaration.
	usesResult bool

	// dropped names what the bindings could not bind, so the declarations leave
	// out exactly what the bindings did.
	dropped map[string]bool

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

// WithBanner prepends text to the generated JavaScript and declarations, for
// the pragmas a project's tooling expects at the top of a generated file, such
// as an eslint-disable comment.
func WithBanner(text string) GeneratorOption {
	return func(g *Generator) {
		g.banner = text
	}
}

// WithProfiling counts and times every call made through the generated module.
//
// A crossing costs about the same however the binding was written, so the thing
// worth reducing is how many there are. Nothing measured that, which left the
// advice to count crossings with no way to act on it.
//
// Off by default: a counter and a clock reading on a five microsecond call are
// not free.
func WithProfiling() GeneratorOption {
	return func(g *Generator) {
		g.profile = true
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
// position renders a source position, empty when there is none to render.
func (g *Generator) position(pos token.Pos) string {
	if g.fset == nil || !pos.IsValid() {
		return ""
	}

	at := g.fset.Position(pos)

	// Relative where possible: an absolute path is noise in a terminal, and CI
	// annotations are resolved against the workspace root.
	if cwd, err := os.Getwd(); err == nil {
		if relative, err := filepath.Rel(cwd, at.Filename); err == nil && !strings.HasPrefix(relative, "..") {
			at.Filename = relative
		}
	}

	return at.String()
}

func NewGenerator(appName string, opts ...GeneratorOption) *Generator {
	g := &Generator{
		appName:  appName,
		style:    defaultStyle(),
		promises: make(map[string]bool),
		fset:     token.NewFileSet(),
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

		// Shared across every load, so a position from one package renders
		// against the same file set as any other.
		Fset: g.fset,
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
	// Empty marks: this runs before a manifest has been read, and it only
	// decides which packages to load, so over-collecting costs nothing.
	collectNamed(marks{}, entry.Type, seen, &order)

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
