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
	needed  map[string]bool

	// open holds the emission attempts in progress, innermost last. See begin.
	open []map[string]bool

	self string
}

func newImports(selfPath string) *imports {
	return &imports{
		aliases: make(map[string]string),
		needed:  make(map[string]bool),
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

// add registers a package the generated code refers to, so the import block
// declares it.
func (i *imports) add(path string, name string) string {
	if depth := len(i.open); depth > 0 {
		i.open[depth-1][path] = true
	} else {
		i.needed[path] = true
	}

	return i.alias(path, name)
}

// begin opens an emission attempt.
//
// An attempt renders the Go spelling of a type before it knows whether the type
// converts, and the qualifier registers an import as it goes. When the attempt
// is then thrown away the text goes with it, so the imports have to go too, or
// the file declares a package nothing refers to.
func (i *imports) begin() {
	i.open = append(i.open, make(map[string]bool))
}

// commit makes an attempt's imports final.
//
// They go to the import set rather than to the enclosing attempt, because the
// text that named them is kept from here on: a converter that succeeds is
// recorded even if the attempt that asked for it goes on to fail, so rolling
// its imports back with the caller's would leave the file referring to a
// package it does not import.
func (i *imports) commit() {
	last := len(i.open) - 1

	for path := range i.open[last] {
		i.needed[path] = true
	}

	i.open = i.open[:last]
}

// rollback discards an attempt's imports. The aliases it reserved are kept: a
// name that has been handed out once must not be handed to another package.
func (i *imports) rollback() {
	i.open = i.open[:len(i.open)-1]
}

// alias hands out the unique name a package is rendered under without claiming
// the generated code refers to it. Naming and importing are separate: an alias
// spells a marshaller's identifier, and a file that only names a type never
// mentions its package.
func (i *imports) alias(path string, name string) string {
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

	paths := make([]string, 0, len(i.needed))
	for path := range i.needed {
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
// refers to -- including the file's own package, which needs a distinct name
// but no import.
func (i *imports) goTypeName(named *types.Named) string {
	pkg := named.Obj().Pkg()
	if pkg == nil {
		return instantiatedName(named)
	}

	return capitalise(i.alias(pkg.Path(), pkg.Name())) + instantiatedName(named)
}
