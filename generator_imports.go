//go:build !js

package crystalline

import (
	"go/types"
	"sort"
	"strconv"
	"strings"
)

// imports tracks the packages generated code refers to, handing out a unique
// alias for each.
//
// Generated bindings live in a package the consumer owns rather than in the
// packages they bind, because a manifest can name symbols from a dependency and
// nothing can be written into a module cache. That makes qualification
// mandatory, and two dependencies sharing a package name unremarkable.
type imports struct {
	aliases map[string]string
	taken   map[string]bool
	self    string
}

func newImports(selfPath string) *imports {
	return &imports{
		aliases: make(map[string]string),
		taken: map[string]bool{
			// Reserved by the generated prelude's own imports.
			contextPackage: true, "errors": true, "strconv": true, "sync": true, "js": true,
		},
		self: selfPath,
	}
}

// qualifier is the types.Qualifier generated code renders through.
func (i *imports) qualifier(pkg *types.Package) string {
	if pkg == nil || pkg.Path() == i.self {
		return ""
	}

	return i.add(pkg.Path(), pkg.Name())
}

func (i *imports) add(path string, name string) string {
	if alias, ok := i.aliases[path]; ok {
		return alias
	}

	alias := sanitiseAlias(name)

	for candidate, n := alias, 2; ; n++ {
		if !i.taken[candidate] {
			alias = candidate

			break
		}

		candidate = alias + strconv.Itoa(n)
	}

	i.taken[alias] = true
	i.aliases[path] = alias

	return alias
}

// block renders the import declaration, aliasing every entry so the rendered
// names cannot drift from what the qualifier produced.
func (i *imports) block(fixed []string) string {
	var out strings.Builder

	out.WriteString("import (\n")

	for _, path := range fixed {
		out.WriteString("\t" + strconv.Quote(path) + "\n")
	}

	paths := make([]string, 0, len(i.aliases))
	for path := range i.aliases {
		paths = append(paths, path)
	}

	sort.Strings(paths)

	if len(paths) > 0 {
		out.WriteString("\n")
	}

	for _, path := range paths {
		out.WriteString("\t" + i.aliases[path] + " " + strconv.Quote(path) + "\n")
	}

	out.WriteString(")\n\n")

	return out.String()
}

func sanitiseAlias(name string) string {
	var out strings.Builder

	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
			out.WriteRune(r)
		case r >= '0' && r <= '9' && i > 0:
			out.WriteRune(r)
		default:
			out.WriteRune('_')
		}
	}

	if out.Len() == 0 {
		return "pkg"
	}

	return out.String()
}

// goTypeName is the identifier fragment generated Go uses for a named type.
//
// It carries the package, because two packages may each declare a Config and
// one function cannot marshal both. The import alias is used rather than the
// package name, since the alias is already unique across every path this file
// refers to.
func (i *imports) goTypeName(named *types.Named) string {
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return instantiatedName(named)
	}

	return capitalise(i.add(pkg.Path(), pkg.Name())) + instantiatedName(named)
}
