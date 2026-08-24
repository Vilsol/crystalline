//go:build !js

package crystalline

import (
	"go/constant"
	"go/types"
	"sort"
	"strings"
)

// Go spells an enum as a named integer or string type, a block of constants of
// that type, and usually a String method. Bound as its underlying type, all
// three collapse into "number": the values a caller may pass, their names, and
// their meaning are all lost.
//
// The type is declared as the union of its values, and the constants are bound
// as an object beside it. TypeScript keeps types and values in separate
// declaration spaces, so both can carry the Go name.

// enumConstants returns the constants declared for a named type, in source
// order, or nothing if the type is not an enum.
func enumConstants(named *types.Named) []*types.Const {
	basic, ok := named.Underlying().(*types.Basic)
	if !ok {
		return nil
	}

	if basic.Info()&(types.IsInteger|types.IsString) == 0 {
		return nil
	}

	pkg := named.Obj().Pkg()
	if pkg == nil {
		return nil
	}

	scope := pkg.Scope()

	var found []*types.Const

	for _, name := range scope.Names() {
		declared, ok := scope.Lookup(name).(*types.Const)
		if !ok || !declared.Exported() {
			continue
		}

		if types.Identical(declared.Type(), named) {
			found = append(found, declared)
		}
	}

	sort.Slice(found, func(i, j int) bool { return found[i].Pos() < found[j].Pos() })

	return found
}

// enumLiteral renders a constant the way TypeScript spells it: a bare number,
// or a quoted string, which is how go/constant already renders both.
func enumLiteral(declared *types.Const) string {
	return declared.Val().ExactString()
}

// qualifiedName renders a named type as the declarations refer to it.
func qualifiedName(named *types.Named) string {
	if named.Obj().Pkg() == nil {
		return named.Obj().Name()
	}

	return named.Obj().Pkg().Name() + "." + named.Obj().Name()
}

// renderEnum declares the value set and the constants that name them.
func renderEnum(named *types.Named, constants []*types.Const) string {
	literals := make([]string, 0, len(constants))
	for _, declared := range constants {
		literals = append(literals, enumLiteral(declared))
	}

	name := named.Obj().Name()

	out := "  type " + name + " = " + strings.Join(literals, " | ") + ";\n"
	out += "  const " + name + ": {\n"

	for _, declared := range constants {
		out += "    readonly " + declared.Name() + ": " + enumLiteral(declared) + ";\n"
	}

	return out + "  };\n"
}

// enumJSValue renders a constant as the Go literal the bindings hand to JS.
func enumJSValue(declared *types.Const) string {
	if declared.Val().Kind() == constant.String {
		return declared.Val().ExactString()
	}

	return "float64(" + declared.Val().ExactString() + ")"
}
