package crystalline

import (
	"context"
	"fmt"
	"reflect"
	"strings"
)

type FuncMeta struct {
	ArgNames []string
	Promise  bool
}

type Definition struct {
	Name        string
	Entities    map[string]reflect.Type
	Definitions map[string]reflect.Type
	Nested      map[string]*Definition
	FuncMeta    map[string]map[string]*FuncMeta
	Promises    map[string]bool
	NotNil      map[string]map[string]bool
}

var (
	JSQuoteStyle    = "'"
	JSTrailingComma = false
)

func (d *Definition) Serialize(ctx context.Context, appName string, path []string) (string, string, error) {
	var tsdFile strings.Builder
	var jsFile strings.Builder

	subPath := path

	if d.Name != "" {
		jsFile.WriteString(fmt.Sprintf("%s = {\n", d.Name))
		tsdFile.WriteString(fmt.Sprintf("export declare namespace %s {\n", d.Name))
		subPath = append(path, d.Name)
	}

	defTsdFile, err := d.serializeDefinitions(ctx, d.Definitions, subPath)
	if err != nil {
		return "", "", err
	}

	tsdFile.WriteString(defTsdFile)

	defTsdFile, defJsFile, err := d.serializeEntities(ctx, d.Entities, subPath, appName)
	if err != nil {
		return "", "", err
	}

	tsdFile.WriteString(defTsdFile)
	jsFile.WriteString(defJsFile)

	hasEntities := len(defJsFile) > 0
	writtenJs := make(map[string]bool)
	for _, key := range SortedKeys(d.Nested) {
		definition := d.Nested[key]
		defTsdFile, defJsFile, err = definition.Serialize(ctx, appName, subPath)
		if err != nil {
			return "", "", err
		}

		tsdFile.WriteString(defTsdFile)
		jsFile.WriteString(defJsFile)

		if len(defJsFile) > 0 {
			writtenJs[key] = true
		}
	}

	if d.Name != "" {
		jsFile.WriteString("};\n")
		tsdFile.WriteString("}\n")
	}

	if !hasEntities && len(writtenJs) == 0 {
		return tsdFile.String(), "", nil
	}

	if d.Name == "" {
		innerJs := jsFile.String()

		jsFile = strings.Builder{}
		for _, key := range SortedKeys(d.Entities) {
			jsFile.WriteString(fmt.Sprintf("export let %s;\n", key))
		}

		for _, key := range SortedKeys(writtenJs) {
			jsFile.WriteString(fmt.Sprintf("export let %s;\n", key))
		}

		jsFile.WriteString("\n")
		jsFile.WriteString("export const initializeCrystalline = () => {\n")

		splitLines := strings.Split(strings.TrimSpace(innerJs), "\n")
		indented := make([]string, len(splitLines))
		for i, line := range splitLines {
			indented[i] = "  " + line
		}
		jsFile.WriteString(strings.Join(indented, "\n"))

		jsFile.WriteString("\n};")

		tsdFile.WriteString("export const initializeCrystalline: () => void;")
	}

	return tsdFile.String(), jsFile.String(), nil
}

func (d *Definition) typeToInterface(ctx context.Context, name string, typeDef reflect.Type) (string, error) {
	if typeDef.Kind() != reflect.Struct {
		return "", fmt.Errorf("%s cannot be converted to an interface: %s", name, typeDef.Kind())
	}

	var result strings.Builder
	result.WriteString("interface ")
	result.WriteString(name)
	result.WriteString(" {\n")

	interfaceCtx := withContextStep(ctx, name)

	for i := 0; i < typeDef.NumField(); i++ {
		field := typeDef.Field(i)
		if field.PkgPath != "" {
			continue
		}

		jsName, optional, err := d.typeToJSName(withContextStep(interfaceCtx, field.Name), "", field.Type, false, name, false)
		if err != nil {
			return "", err
		}

		result.WriteString("  ")
		result.WriteString(field.Name)
		if optional {
			if !d.NotNil[name][field.Name] {
				result.WriteString("?")
			}
		}
		result.WriteString(": ")
		result.WriteString(jsName)
		result.WriteString(";\n")
	}

	newInstance := reflect.New(typeDef)
	for i := 0; i < newInstance.NumMethod(); i++ {
		typeMethod := newInstance.Type().Method(i)
		if typeMethod.PkgPath != "" {
			continue
		}

		if isIgnored(typeDef.String(), typeMethod.Name) {
			continue
		}

		instanceMethod := newInstance.Method(i)
		jsName, _, err := d.typeToJSName(withContextStep(interfaceCtx, typeMethod.Name), typeMethod.Name, instanceMethod.Type(), true, name, false)
		if err != nil {
			return "", err
		}

		result.WriteString("  ")
		result.WriteString(jsName)
		result.WriteString(";\n")
	}

	result.WriteString("}\n")

	return result.String(), nil
}

func (d *Definition) typeToJSName(ctx context.Context, name string, typeDef reflect.Type, topLevel bool, interfaceName string, returnsPromise bool) (string, bool, error) {
	switch typeDef.Kind() {
	case reflect.Bool:
		return "boolean", false, nil
	case reflect.Int:
		fallthrough
	case reflect.Int8:
		fallthrough
	case reflect.Int16:
		fallthrough
	case reflect.Int32:
		fallthrough
	case reflect.Int64:
		fallthrough
	case reflect.Uint:
		fallthrough
	case reflect.Uint8:
		fallthrough
	case reflect.Uint16:
		fallthrough
	case reflect.Uint32:
		fallthrough
	case reflect.Uint64:
		fallthrough
	case reflect.Uintptr:
		fallthrough
	case reflect.Float32:
		fallthrough
	case reflect.Float64:
		fallthrough
	case reflect.UnsafePointer:
		return "number", false, nil
	case reflect.Slice:
		fallthrough
	case reflect.Array:
		var result strings.Builder

		if typeDef.Elem().Kind() == reflect.Uint8 {
			return "Uint8Array", true, nil
		}

		result.WriteString("Array<")
		jsName, undefined, err := d.typeToJSName(ctx, "", typeDef.Elem(), false, "", false)
		if err != nil {
			return "", false, err
		}

		result.WriteString(jsName)
		if undefined {
			result.WriteString(" | undefined")
		}
		result.WriteString(">")

		return result.String(), true, nil
	case reflect.Func:
		var result strings.Builder

		if name != "" && topLevel {
			if interfaceName == "" {
				result.WriteString("function ")
			}
			result.WriteString(name)
		}

		result.WriteString("(")

		if d.FuncMeta != nil {
			if funcs, ok := d.FuncMeta[interfaceName]; ok {
				if f, ok := funcs[name]; ok {
					if f.Promise {
						returnsPromise = f.Promise
					}
				}
			}
		}

		for i := 0; i < typeDef.NumIn(); i++ {
			if i > 0 {
				result.WriteString(", ")
			}

			in := typeDef.In(i)

			if in.Kind() == reflect.Func {
				returnsPromise = true
			}

			jsName, optional, err := d.typeToJSName(withContextStep(ctx, in.Name()), in.Name(), in, false, "", true)
			if err != nil {
				return "", false, err
			}

			argName := fmt.Sprintf("arg%d", i+1)

			if d.FuncMeta != nil {
				if funcs, ok := d.FuncMeta[interfaceName]; ok {
					if f, ok := funcs[name]; ok {
						if len(f.ArgNames)-1 >= i {
							argName = f.ArgNames[i]
						}
					}
				}
			}

			if optional {
				result.WriteString(fmt.Sprintf("%s?: ", argName))
			} else {
				result.WriteString(fmt.Sprintf("%s: ", argName))
			}

			result.WriteString(jsName)
		}

		result.WriteString(")")

		if name != "" && topLevel {
			result.WriteString(": ")
		} else {
			result.WriteString(" => ")
		}

		isPromise := returnsPromise
		if d.Promises != nil {
			isPromise = isPromise || d.Promises[name]
		}

		if isPromise {
			result.WriteString("Promise<")
		}

		if typeDef.NumOut() > 0 {
			if typeDef.NumOut() > 1 {
				result.WriteString("[")
			}

			for i := 0; i < typeDef.NumOut(); i++ {
				if i > 0 {
					result.WriteString(", ")
				}

				out := typeDef.Out(i)
				jsName, optional, err := d.typeToJSName(withContextStep(ctx, out.Name()), out.Name(), out, false, "", false)
				if err != nil {
					return "", false, err
				}

				if optional {
					result.WriteString("(")
					result.WriteString(jsName)
					result.WriteString(" | undefined)")
				} else {
					result.WriteString(jsName)
				}
			}

			if typeDef.NumOut() > 1 {
				result.WriteString("]")
			}
		} else {
			result.WriteString("void")
		}

		if isPromise {
			result.WriteString(">")
		}

		return result.String(), false, nil
	case reflect.Map:
		var result strings.Builder
		result.WriteString("Record<")

		keyJsName, optional, err := d.typeToJSName(ctx, "", typeDef.Key(), false, "", false)
		if err != nil {
			return "", false, err
		}

		result.WriteString(keyJsName)
		if optional {
			result.WriteString(" | undefined")
		}

		result.WriteString(", ")

		valueJsName, optional, err := d.typeToJSName(ctx, "", typeDef.Elem(), false, "", false)
		if err != nil {
			return "", false, err
		}

		result.WriteString(valueJsName)
		if optional {
			result.WriteString(" | undefined")
		}

		result.WriteString(">")

		return result.String(), true, nil
	case reflect.Pointer:
		jsName, _, err := d.typeToJSName(withContextStep(ctx, name), name, typeDef.Elem(), false, "", false)
		if err != nil {
			return "", false, err
		}

		return jsName, true, nil
	case reflect.String:
		return "string", false, nil
	case reflect.Struct:
		noTypesName, _, _ := strings.Cut(typeDef.String(), "[")
		return noTypesName, false, nil
	case reflect.Interface:
		if typeDef.String() == "error" {
			return "Error", false, nil
		}
		return "unknown", true, nil
	}

	return "", false, fmt.Errorf("un-convertable type: %q - %s (%s)", getContextSteps(ctx), typeDef.Kind(), typeDef)
}

func (d *Definition) serializeDefinitions(ctx context.Context, definitions map[string]reflect.Type, path []string) (string, error) {
	var tsdFile strings.Builder

	for _, name := range SortedKeys(definitions) {
		typeDef := definitions[name]
		jsType, err := d.typeToInterface(withContextStep(ctx, strings.Join(path, ".")), name, typeDef)
		if err != nil {
			return "", err
		}

		indentation := strings.Repeat("  ", len(path))

		splitLines := strings.Split(strings.TrimSpace(jsType), "\n")
		indented := make([]string, len(splitLines))
		for i, line := range splitLines {
			indented[i] = indentation + line
		}

		tsdFile.WriteString(fmt.Sprintf("%s\n", strings.Join(indented, "\n")))
	}

	return tsdFile.String(), nil
}

func (d *Definition) serializeEntities(ctx context.Context, entities map[string]reflect.Type, path []string, appName string) (string, string, error) {
	var tsdFile strings.Builder
	var jsFile strings.Builder

	for i, name := range SortedKeys(entities) {
		typeDef := entities[name]
		jsType, optional, err := d.typeToJSName(withContextStep(ctx, name), name, typeDef, true, "", false)
		if err != nil {
			return "", "", err
		}

		if len(path) == 0 {
			jsFile.WriteString(strings.Replace(fmt.Sprintf(`%s = globalThis["go"]["%s"]["%s"];`, name, appName, name)+"\n", "\"", JSQuoteStyle, -1))
			if optional {
				tsdFile.WriteString(fmt.Sprintf("export const %s = %s | undefined;\n", name, jsType))
			} else {
				tsdFile.WriteString(fmt.Sprintf("export const %s = %s;\n", name, jsType))
			}
		} else {
			mergedPathJs := ""
			for _, s := range path {
				mergedPathJs += fmt.Sprintf(`["%s"]`, s)
			}

			indentation := strings.Repeat("  ", len(path))

			comma := ","
			if !JSTrailingComma && i == len(entities)-1 {
				comma = ""
			}

			if typeDef.Kind() != reflect.Func {
				jsFile.WriteString(strings.Replace(fmt.Sprintf(`%s%s: globalThis["go"]["%s"]%s["%s"]%s`, indentation, name, appName, mergedPathJs, name, comma)+"\n", "\"", JSQuoteStyle, -1))
				if optional {
					tsdFile.WriteString(fmt.Sprintf("%sconst %s: %s | undefined;\n", indentation, name, jsType))
				} else {
					tsdFile.WriteString(fmt.Sprintf("%sconst %s: %s;\n", indentation, name, jsType))
				}
			} else {
				jsFile.WriteString(strings.Replace(fmt.Sprintf(`%s%s: wrap(globalThis["go"]["%s"]%s["%s"])%s`, indentation, name, appName, mergedPathJs, name, comma)+"\n", "\"", JSQuoteStyle, -1))
				tsdFile.WriteString(fmt.Sprintf("%s%s;\n", indentation, jsType))
			}
		}
	}

	return tsdFile.String(), jsFile.String(), nil
}
