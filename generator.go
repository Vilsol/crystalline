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

	// camel renames members for JavaScript rather than keeping the Go
	// spelling.
	camel bool

	// optionErr holds what an option could not accept. An option cannot
	// return, and a generator that renders JavaScript with a quote character
	// it does not have would produce a module nobody can import, so the
	// refusal waits for Build rather than being dropped.
	optionErr error

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

// QuoteStyle is the quote character the generated JavaScript uses for its
// string literals.
type QuoteStyle int

const (
	// SingleQuote is the default, and what most formatters produce.
	SingleQuote QuoteStyle = iota

	// DoubleQuote suits a project whose formatter prefers them.
	DoubleQuote
)

// characters renders the style, and reports whether it is one of the two.
func (q QuoteStyle) characters() (string, bool) {
	switch q {
	case SingleQuote:
		return "'", true
	case DoubleQuote:
		return `"`, true
	}

	return "", false
}

// WithQuoteStyle sets the quote character used in the generated JavaScript, so
// the output can match the project's formatter. Defaults to a single quote.
//
// It was a string, which meant any string: a typo produced a module that does
// not parse, found by whoever imported it.
func WithQuoteStyle(style QuoteStyle) GeneratorOption {
	return func(g *Generator) {
		if _, ok := style.characters(); !ok {
			g.optionErr = fmt.Errorf("quote style %d is neither SingleQuote nor DoubleQuote", style)

			return
		}

		g.style.quote = style
	}
}

// WithCamelCase renames exported members for JavaScript: Timeout becomes
// timeout, ID becomes id, HTTPServer becomes httpServer.
//
// Off by default, and worth leaving off. A name identical on both sides means
// one grep finds every use of it across two languages, which is a live
// debugging aid that costs nothing. Turn it on for a surface whose consumers
// will never read the Go.
//
// Type names, namespaces, enum constants and the names given to r.Value are
// left alone: the first three are not members, and the last was written out by
// hand and is already whatever it was meant to be. A single field can be named
// outright with `crystalline:"name=timeout"`, with or without this option.
func WithCamelCase() GeneratorOption {
	return func(g *Generator) {
		g.camel = true
	}
}

// jsMemberName is the one answer to what a Go member is called on the other
// side.
//
// Everything that writes a name into the module, the declarations or the
// bindings asks here, because the module and the declarations describing
// different names is the failure this project keeps having: a name computed
// twice is a name that can differ.
func (g *Generator) jsMemberName(goName string, tag string) string {
	if named, ok := tagValue(tag, tagRename); ok {
		return named
	}

	if !g.camel {
		return goName
	}

	return camelise(goName)
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

// NewGenerator returns a generator publishing under appName, which is the name
// the bindings appear under in the JavaScript global object graph.
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
		// NeedDeps so that a type declared in a dependency carries its whole
		// package with it: an enum's constants live in the declaring package's
		// scope, and without this that scope holds only what was referenced.
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedTypesInfo | packages.NeedFiles | packages.NeedDeps,
		Dir: dir,

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
// collectPackages records the package of every named type reachable from t.
func collectPackages(t types.Type, seen map[types.Type]bool, out *[]string) {
	if t == nil || seen[t] {
		return
	}

	seen[t] = true

	if obj := namedObject(t); obj != nil {
		if obj.Pkg() != nil {
			*out = append(*out, obj.Pkg().Path())
		}

		collectPackages(t.Underlying(), seen, out)

		return
	}

	switch typed := t.(type) {
	case *types.Pointer:
		collectPackages(typed.Elem(), seen, out)
	case *types.Slice:
		collectPackages(typed.Elem(), seen, out)
	case *types.Array:
		collectPackages(typed.Elem(), seen, out)
	case *types.Chan:
		collectPackages(typed.Elem(), seen, out)
	case *types.Map:
		collectPackages(typed.Key(), seen, out)
		collectPackages(typed.Elem(), seen, out)
	case *types.Struct:
		for i := range typed.NumFields() {
			collectPackages(typed.Field(i).Type(), seen, out)
		}
	case *types.Signature:
		for i := range typed.Params().Len() {
			collectPackages(typed.Params().At(i).Type(), seen, out)
		}

		for i := range typed.Results().Len() {
			collectPackages(typed.Results().At(i).Type(), seen, out)
		}
	}
}

func (g *Generator) loadReferenced(entries []entry) error {
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

func referencedPackages(entry entry) []string {
	var out []string

	if entry.Object != nil && entry.Object.Pkg() != nil {
		out = append(out, entry.Object.Pkg().Path())
	}

	// Every named type, whatever its shape. The walk that decides what to
	// declare asks questions this cannot answer yet — whether a type has
	// constants needs the package holding them to be loaded, which is what this
	// is deciding. Using that walk here meant a package contributing only an
	// enum was never loaded, so its constants were never found and it silently
	// became a number.
	collectPackages(entry.Type, make(map[types.Type]bool), &out)

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
