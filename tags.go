//go:build !js

package crystalline

import (
	"fmt"
	"go/types"
	"slices"
	"strings"
	"unicode"
)

// Struct tag options recognised on the `crystalline` key.
const (
	tagName   = "crystalline"
	tagNotNil = "not_nil"
	tagBigInt = "bigint"
	tagRename = "name"
)

var knownTagOptions = []string{tagNotNil, tagBigInt, tagRename + "=<name>"}

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
// validateTag checks a field's tag against the field it is on.
//
// not_nil promises an empty collection where there would be a null, which only
// a slice or a map has. Accepting it anywhere meant a pointer field lost the ?
// from its declaration and kept returning null: the type said the field was
// always there and the value disagreed.
func validateTag(tag string, t types.Type) error {
	for len(tag) > 0 {
		var option string
		option, tag, _ = strings.Cut(tag, ",")

		option = strings.TrimSpace(option)
		if option == "" {
			continue
		}

		if key, value, assigned := strings.Cut(option, "="); assigned {
			if key != tagRename {
				return fmt.Errorf("unknown %s tag option %q (known options: %s)", tagName, key, strings.Join(knownTagOptions, ", "))
			}

			if !isJSIdentifier(value) {
				return fmt.Errorf("%s=%s is not a JavaScript identifier", tagRename, value)
			}

			continue
		}

		if !slices.Contains(knownTagOptions, option) {
			return fmt.Errorf("unknown %s tag option %q (known options: %s)", tagName, option, strings.Join(knownTagOptions, ", "))
		}

		if option == tagNotNil && !hasEmptyForm(t) {
			return fmt.Errorf("%s applies to a slice or a map, and this is a %s", tagNotNil, t)
		}

		// Narrower integers already cross exactly, so a bigint there is a cost
		// with nothing to buy: JSON.stringify throws on one, and mixing it with
		// a number is a TypeError.
		if option == tagBigInt && !isWideInteger(t) {
			return fmt.Errorf("%s applies to an int64 or a uint64, and this is a %s", tagBigInt, t)
		}
	}

	return nil
}

// isWideInteger reports whether a type is one a JavaScript number cannot hold
// exactly. It is the same question the wide-integer warning asks.
func isWideInteger(t types.Type) bool {
	basic, ok := t.Underlying().(*types.Basic)
	if !ok {
		return false
	}

	return basic.Kind() == types.Int64 || basic.Kind() == types.Uint64
}

// isUnsignedWide distinguishes the two, since they parse and format differently.
func isUnsignedWide(t types.Type) bool {
	basic, ok := t.Underlying().(*types.Basic)

	return ok && basic.Kind() == types.Uint64
}

// tagValue reads an option written as key=value.
func tagValue(tag string, key string) (string, bool) {
	for len(tag) > 0 {
		var option string
		option, tag, _ = strings.Cut(tag, ",")

		if name, value, assigned := strings.Cut(strings.TrimSpace(option), "="); assigned && name == key {
			return value, true
		}
	}

	return "", false
}

// camelise lowers the leading run of capitals rather than only the first
// letter, so ID becomes id and HTTPServer becomes httpServer. Lowering one
// letter would give hTTPServer, which is nobody's convention.
func camelise(name string) string {
	if name == "" {
		return name
	}

	runes := []rune(name)

	upper := 0
	for upper < len(runes) && unicode.IsUpper(runes[upper]) {
		upper++
	}

	if upper == 0 {
		return name
	}

	// A run that ends before the name does starts the next word, so it keeps
	// its capital: the S of HTTPServer.
	if upper > 1 && upper < len(runes) {
		upper--
	}

	for i := range upper {
		runes[i] = unicode.ToLower(runes[i])
	}

	return string(runes)
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

// hasEmptyForm reports whether a type has an empty value that can stand in for
// nil on the other side.
func hasEmptyForm(t types.Type) bool {
	switch t.Underlying().(type) {
	case *types.Slice, *types.Map:
		return true
	}

	return false
}
