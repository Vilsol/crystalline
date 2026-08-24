//go:build !js

package crystalline

import (
	"fmt"
	"go/types"
	"slices"
	"strings"
)

// Struct tag options recognised on the `crystalline` key.
const (
	tagName   = "crystalline"
	tagNotNil = "not_nil"
)

var knownTagOptions = []string{tagNotNil}

// tagHasOption reports whether a crystalline struct tag carries an option.
func tagHasOption(tag string, want string) bool {
	for len(tag) > 0 {
		var option string
		option, tag, _ = strings.Cut(tag, ",")

		if strings.TrimSpace(option) == want {
			return true
		}
	}

	return false
}

// validateTag rejects options this version does not understand, so that a
// misspelled option fails the build instead of being quietly dropped.
func validateTag(tag string) error {
	for len(tag) > 0 {
		var option string
		option, tag, _ = strings.Cut(tag, ",")

		option = strings.TrimSpace(option)
		if option == "" {
			continue
		}

		if !slices.Contains(knownTagOptions, option) {
			return fmt.Errorf("unknown %s tag option %q (known options: %s)", tagName, option, strings.Join(knownTagOptions, ", "))
		}
	}

	return nil
}

// sortedKeys returns a map's keys in a stable order.
func sortedKeys[T any](data map[string]T) []string {
	result := make([]string, 0, len(data))
	for key := range data {
		result = append(result, key)
	}

	slices.Sort(result)

	return result
}

// instantiatedName renders a named type including its type arguments.
//
// Two instantiations of one generic type are different types, and naming them
// both after the bare generic collides: the declarations end up with two
// interfaces of one name, and the Go bindings with one marshaller doing duty
// for both.
// typeIdentity is the one answer to "which named type is this", for anything
// that has to find a decision about a type again later.
//
// The bare name is not an answer: two packages may each declare a Config, and
// keying anything on the short name silently merges them.
func typeIdentity(named *types.Named) string {
	pkg := ""
	if named.Obj().Pkg() != nil {
		pkg = named.Obj().Pkg().Name() + "."
	}

	return pkg + instantiatedName(named)
}

// memberIdentity names one field or method of a type.
func memberIdentity(named *types.Named, member string) string {
	return typeIdentity(named) + "." + member
}

func instantiatedName(named *types.Named) string {
	name := named.Obj().Name()

	args := named.TypeArgs()
	if args == nil || args.Len() == 0 {
		return name
	}

	var out strings.Builder

	out.WriteString(name)
	out.WriteString("Of")

	for i := 0; i < args.Len(); i++ {
		if i > 0 {
			out.WriteString("And")
		}

		out.WriteString(typeArgumentName(args.At(i)))
	}

	return out.String()
}

func typeArgumentName(t types.Type) string {
	switch typed := t.(type) {
	case *types.Basic:
		return capitalise(typed.Name())
	case *types.Named:
		return instantiatedName(typed)
	case *types.Alias:
		return capitalise(typed.Obj().Name())
	case *types.Pointer:
		return "Ptr" + typeArgumentName(typed.Elem())
	case *types.Slice:
		return "SliceOf" + typeArgumentName(typed.Elem())
	case *types.Array:
		return "ArrayOf" + typeArgumentName(typed.Elem())
	case *types.Map:
		return "MapOf" + typeArgumentName(typed.Key()) + "To" + typeArgumentName(typed.Elem())
	}

	return capitalise(strings.NewReplacer("[", "", "]", "", ".", "", "*", "Ptr", " ", "").Replace(t.String()))
}

func capitalise(name string) string {
	if name == "" {
		return name
	}

	return strings.ToUpper(name[:1]) + name[1:]
}
