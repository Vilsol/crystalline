package crystalline

import (
	"fmt"
	"reflect"
	"strings"
)

type Definition struct {
	Name         string
	Entities     map[string]reflect.Type
	Definitions  map[string]reflect.Type
	Nested       map[string]*Definition
	FuncArgNames map[string]map[string][]string
}

func (d *Definition) Serialize(appName string, path []string) (string, string, error) {
	var tsdFile strings.Builder
	var jsFile strings.Builder

	subPath := path

	if d.Name != "" {
		jsFile.WriteString(fmt.Sprintf("%s = {\n", d.Name))
		tsdFile.WriteString(fmt.Sprintf("export declare namespace %s {\n", d.Name))
		subPath = append(path, d.Name)
	}

	defTsdFile, err := d.serializeDefinitions(d.Definitions, subPath)
	if err != nil {
		return "", "", err
	}

	tsdFile.WriteString(defTsdFile)

	defTsdFile, defJsFile, err := d.serializeEntities(d.Entities, subPath, appName)
	if err != nil {
		return "", "", err
	}

	tsdFile.WriteString(defTsdFile)
	jsFile.WriteString(defJsFile)

	hasEntities := len(defJsFile) > 0
	writtenJs := make(map[string]bool)
	for _, key := range SortedKeys(d.Nested) {
		definition := d.Nested[key]
		defTsdFile, defJsFile, err = definition.Serialize(appName, subPath)
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
		jsFile.WriteString("}\n")
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

		jsFile.WriteString("\n}")

		tsdFile.WriteString("export const initializeCrystalline: () => void;")
	}

	return tsdFile.String(), jsFile.String(), nil
}

func (d *Definition) typeToInterface(name string, typeDef reflect.Type) string {
	if typeDef.Kind() != reflect.Struct {
		panic("cannot be converted to interface: " + typeDef.Kind().String())
	}

	var result strings.Builder
	result.WriteString("interface ")
	result.WriteString(name)
	result.WriteString(" {\n")

	for i := 0; i < typeDef.NumField(); i++ {
		field := typeDef.Field(i)
		jsName, optional := d.typeToJSName("", field.Type, false, name)

		result.WriteString("  ")
		result.WriteString(field.Name)
		if optional {
			result.WriteString("?")
		}
		result.WriteString(": ")
		result.WriteString(jsName)
		result.WriteString(";\n")
	}

	newInstance := reflect.New(typeDef)
	for i := 0; i < newInstance.NumMethod(); i++ {
		typeMethod := newInstance.Type().Method(i)
		instanceMethod := newInstance.Method(i)
		jsName, _ := d.typeToJSName(typeMethod.Name, instanceMethod.Type(), true, name)
		result.WriteString("  ")
		result.WriteString(jsName)
		result.WriteString(";\n")
	}

	result.WriteString("}\n")
	return result.String()
}

func (d *Definition) typeToJSName(name string, typeDef reflect.Type, topLevel bool, interfaceName string) (string, bool) {
	switch typeDef.Kind() {
	case reflect.Bool:
		return "boolean", false
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
		return "number", false
	case reflect.Slice:
		fallthrough
	case reflect.Array:
		var result strings.Builder
		result.WriteString("Array<")
		jsName, undefined := d.typeToJSName("", typeDef.Elem(), false, "")
		result.WriteString(jsName)
		if undefined {
			result.WriteString(" | undefined")
		}
		result.WriteString(">")
		return result.String(), true
	case reflect.Func:
		var result strings.Builder

		if name != "" && topLevel {
			if interfaceName == "" {
				result.WriteString("function ")
			}
			result.WriteString(name)
		}

		result.WriteString("(")

		for i := 0; i < typeDef.NumIn(); i++ {
			if i > 0 {
				result.WriteString(", ")
			}

			in := typeDef.In(i)
			jsName, optional := d.typeToJSName(in.Name(), in, false, "")

			argName := fmt.Sprintf("arg%d", i+1)

			if d.FuncArgNames != nil {
				if funcs, ok := d.FuncArgNames[interfaceName]; ok {
					if f, ok := funcs[name]; ok {
						if len(f)-1 >= i {
							argName = f[i]
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

		if typeDef.NumOut() > 0 {
			if typeDef.NumOut() > 1 {
				result.WriteString("[")
			}

			for i := 0; i < typeDef.NumOut(); i++ {
				if i > 0 {
					result.WriteString(", ")
				}

				out := typeDef.Out(i)
				jsName, optional := d.typeToJSName(out.Name(), out, false, "")
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

		return result.String(), false
	case reflect.Map:
		var result strings.Builder
		result.WriteString("Record<")

		keyJsName, optional := d.typeToJSName("", typeDef.Key(), false, "")
		result.WriteString(keyJsName)
		if optional {
			result.WriteString(" | undefined")
		}

		result.WriteString(", ")

		valueJsName, optional := d.typeToJSName("", typeDef.Elem(), false, "")
		result.WriteString(valueJsName)
		if optional {
			result.WriteString(" | undefined")
		}

		result.WriteString(">")
		return result.String(), true
	case reflect.Pointer:
		jsName, _ := d.typeToJSName(name, typeDef.Elem(), false, "")
		return jsName, true
	case reflect.String:
		return "string", false
	case reflect.Struct:
		return typeDef.String(), false
	}

	panic("un-convertable type: " + typeDef.Kind().String())
}

func (d *Definition) serializeDefinitions(definitions map[string]reflect.Type, path []string) (string, error) {
	var tsdFile strings.Builder

	for _, name := range SortedKeys(definitions) {
		typeDef := definitions[name]
		jsType := d.typeToInterface(name, typeDef)
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

func (d *Definition) serializeEntities(entities map[string]reflect.Type, path []string, appName string) (string, string, error) {
	var tsdFile strings.Builder
	var jsFile strings.Builder

	for _, name := range SortedKeys(entities) {
		typeDef := entities[name]
		jsType, optional := d.typeToJSName(name, typeDef, true, "")
		if len(path) == 0 {
			jsFile.WriteString(fmt.Sprintf(`%s = globalThis["go"]["%s"]["%s"];`+"\n", name, appName, name))
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
			jsFile.WriteString(fmt.Sprintf(`%s%s: globalThis["go"]["%s"]%s["%s"],`, indentation, name, appName, mergedPathJs, name) + "\n")

			if typeDef.Kind() != reflect.Func {
				if optional {
					tsdFile.WriteString(fmt.Sprintf("%sconst %s: %s | undefined;\n", indentation, name, jsType))
				} else {
					tsdFile.WriteString(fmt.Sprintf("%sconst %s: %s;\n", indentation, name, jsType))
				}
			} else {
				tsdFile.WriteString(fmt.Sprintf("%s%s;\n", indentation, jsType))
			}
		}
	}

	return tsdFile.String(), jsFile.String(), nil
}
