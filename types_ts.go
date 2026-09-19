//go:build !js

package crystalline

import (
	"errors"
	"fmt"
	"go/types"
)

// Mapping of Go types onto their TypeScript counterparts.
func (g *Generator) tsType(t types.Type) (string, bool, error) {
	t = unaliased(t)

	switch typed := t.(type) {
	case *types.Basic:
		return basicToJS(typed)
	case *types.Named, *types.Alias:
		return g.namedToJS(t)
	case *types.Pointer:
		jsName, _, err := g.tsType(typed.Elem())

		return jsName, true, err
	case *types.Slice:
		return g.sequenceToJS(typed.Elem())
	case *types.Array:
		return g.sequenceToJS(typed.Elem())
	case *types.Map:
		return g.mapToJS(typed)
	case *types.Signature:
		jsName, err := g.renderSignature("", typed, false, false)

		return jsName, false, err
	case *types.Interface:
		return "unknown", true, nil
	case *types.Chan:
		if typed.Dir() == types.SendOnly {
			return "", false, errors.New("a send-only channel has no JS counterpart")
		}

		// A receive-only channel and an async iterator are the same idea.
		element, _, err := g.tsType(typed.Elem())
		if err != nil {
			return "", false, err
		}

		return "AsyncIterable<" + element + ">", false, nil
	}

	return "", false, fmt.Errorf("un-convertable type: %s", t)
}

// tsNumber is what every Go numeric type crosses as.
const tsNumber = "number"

func basicToJS(basic *types.Basic) (string, bool, error) {
	switch basic.Kind() {
	case types.Bool:
		return "boolean", false, nil
	case types.String:
		return "string", false, nil
	case types.Complex64:
		return "", false, errors.New("complex64 cannot be converted to wasm")
	case types.Complex128:
		return "", false, errors.New("complex128 cannot be converted to wasm")
	case types.UnsafePointer:
		return tsNumber, false, nil
	}

	if basic.Info()&types.IsNumeric != 0 {
		return tsNumber, false, nil
	}

	return "", false, fmt.Errorf("un-convertable basic type: %s", basic)
}

func (g *Generator) namedToJS(t types.Type) (string, bool, error) {
	t = unaliased(t)

	obj := namedObject(t)
	if obj == nil {
		return g.tsType(t.Underlying())
	}

	// error is the one universe-scoped interface with a JS counterpart.
	if obj.Pkg() == nil && obj.Name() == "error" {
		return "Error", false, nil
	}

	// A mapping wins over everything else the type might be, which is the order
	// collectNamed decides what to declare in. Asking second meant time.Duration
	// -- an enum with a mapping -- was named as an enum and declared as nothing.
	// A mapped type crosses as its counterpart rather than as its structure. A
	// declared mapping crosses as whatever its own functions carry.
	if mapped, ok := g.marks.marshallerFor(t); ok {
		if mapped.declaredByManifest() {
			return g.tsType(mapped.intermediate)
		}

		return mapped.declared, false, nil
	}

	// An interface Go declares is supplied from JavaScript, so it is named
	// rather than reduced to unknown.
	if _, ok := t.Underlying().(*types.Interface); ok && !isErrorType(t) && !isContextType(t) {
		if named, ok := t.(*types.Named); ok {
			return qualifiedName(named), false, nil
		}
	}

	// An enum keeps its own name, so a signature says which values are meant.
	if named, ok := t.(*types.Named); ok && len(enumConstants(named)) > 0 {
		return qualifiedName(named), false, nil
	}

	if _, ok := t.Underlying().(*types.Struct); ok {
		name := obj.Name()
		if named, ok := t.(*types.Named); ok {
			name = instantiatedName(named)
		}

		if obj.Pkg() == nil {
			return name, false, nil
		}

		return obj.Pkg().Name() + "." + name, false, nil
	}

	// Defined types over a supported underlying type map to that type.
	return g.tsType(t.Underlying())
}

func (g *Generator) sequenceToJS(elem types.Type) (string, bool, error) {
	if basic, ok := elem.Underlying().(*types.Basic); ok && basic.Kind() == types.Uint8 {
		return "Uint8Array", true, nil
	}

	jsName, optional, err := g.tsType(elem)
	if err != nil {
		return "", false, err
	}

	if optional {
		jsName += orUndefined
	}

	return "Array<" + jsName + ">", true, nil
}

func (g *Generator) mapToJS(typed *types.Map) (string, bool, error) {
	keyName, keyOptional, err := g.tsType(typed.Key())
	if err != nil {
		return "", false, err
	}

	if keyOptional {
		keyName += orUndefined
	}

	valueName, valueOptional, err := g.tsType(typed.Elem())
	if err != nil {
		return "", false, err
	}

	if valueOptional {
		valueName += orUndefined
	}

	return "Record<" + keyName + ", " + valueName + ">", true, nil
}

// unaliased resolves a Go type alias to the type it denotes.
//
// An alias is transparent: `type Alias = Shape` makes Alias and Shape the same
// type, not two types. Every type switch here normalises first, because several
// of them list *types.Alias beside *types.Named and then assert .(*types.Named)
// inside the branch, which panics the generator on a type a manifest is
// perfectly entitled to name.
func unaliased(t types.Type) types.Type {
	return types.Unalias(t)
}

func namedObject(t types.Type) *types.TypeName {
	switch typed := t.(type) {
	case *types.Named:
		return typed.Obj()
	case *types.Alias:
		return typed.Obj()
	}

	return nil
}

// collectNamed walks a type for the named struct types reachable from it,
// recording each one once in declaration-independent order.
func collectNamed(m marks, t types.Type, seen map[*types.Named]bool, order *[]*types.Named) {
	t = unaliased(t)

	switch typed := t.(type) {
	case *types.Named:
		if seen[typed] {
			return
		}

		if _, ok := typed.Underlying().(*types.Struct); !ok {
			// A mapping wins over everything else the type might be. A type can
			// be both: time.Duration has constants of its own, and it crosses
			// as a number of milliseconds rather than as one of them.
			if _, mapped := m.marshallerFor(typed); mapped {
				return
			}

			// An enum's value set and an interface's method set are both
			// declared: they say what a caller may pass. error and
			// context.Context are interfaces with a counterpart of their own,
			// so neither is something JavaScript supplies.
			_, isInterface := typed.Underlying().(*types.Interface)
			supplied := isInterface && !isErrorType(typed) && !isContextType(typed)

			if len(enumConstants(typed)) > 0 || supplied {
				seen[typed] = true
				*order = append(*order, typed)
			}

			return
		}

		// A mapped type crosses as a JS counterpart, so declaring its fields
		// and methods would describe something nobody receives.
		if _, mapped := m.marshallerFor(typed); mapped {
			return
		}

		seen[typed] = true
		*order = append(*order, typed)

		// Only exported members are ever emitted, so only exported members
		// get to decide what has to be convertible. A walk wider than the
		// emitters turns a private implementation detail into a refusal.
		structType := typed.Underlying().(*types.Struct)
		for i := 0; i < structType.NumFields(); i++ {
			if field := structType.Field(i); field.Exported() {
				collectNamed(m, field.Type(), seen, order)
			}
		}

		for i := 0; i < typed.NumMethods(); i++ {
			if method := typed.Method(i); method.Exported() {
				collectNamed(m, method.Type(), seen, order)
			}
		}
	case *types.Pointer:
		collectNamed(m, typed.Elem(), seen, order)
	case *types.Slice:
		collectNamed(m, typed.Elem(), seen, order)
	case *types.Array:
		collectNamed(m, typed.Elem(), seen, order)
	case *types.Map:
		collectNamed(m, typed.Key(), seen, order)
		collectNamed(m, typed.Elem(), seen, order)
	case *types.Chan:
		// A receive-only channel renders as AsyncIterable<Elem>, so the element
		// is named and has to be declared.
		collectNamed(m, typed.Elem(), seen, order)
	case *types.Signature:
		for i := 0; i < typed.Params().Len(); i++ {
			collectNamed(m, typed.Params().At(i).Type(), seen, order)
		}

		for i := 0; i < typed.Results().Len(); i++ {
			collectNamed(m, typed.Results().At(i).Type(), seen, order)
		}
	}
}
