package crystalline

import (
	"context"
	"errors"
	"fmt"
	"path"
	"reflect"
	"regexp"
	"runtime"
	"strings"
)

type Exposer struct {
	appName        string
	rootDefinition *Definition
}

func NewExposer(appName string) *Exposer {
	return &Exposer{
		appName:        appName,
		rootDefinition: &Definition{},
	}
}

func (e *Exposer) ExposeFuncOrPanic(entity any) {
	if err := e.ExposeFunc(entity); err != nil {
		panic(fmt.Errorf("failed exposing func: %w", err))
	}
}

func (e *Exposer) ExposeFuncOrPanicPromise(entity any) {
	if err := e.ExposeFuncPromise(entity, true); err != nil {
		panic(fmt.Errorf("failed exposing func promise: %w", err))
	}
}

func (e *Exposer) ExposeFunc(entity any) error {
	return e.ExposeFuncPromise(entity, false)
}

func (e *Exposer) ExposeFuncPromise(entity any, promise bool) error {
	value := reflect.ValueOf(entity)
	valueType := value.Type()

	if valueType.Kind() != reflect.Func {
		return errors.New("can only expose functions without specifying package and name")
	}

	pointer := value.Pointer()
	e.processFunctionMeta(pointer, "")

	splitDef := strings.Split(path.Base(runtime.FuncForPC(pointer).Name()), ".")
	pkgName := splitDef[0]
	valueName := splitDef[1]

	if valueName == "" || pkgName == "" {
		return errors.New("could not determine function name or package")
	}

	setNamespace(e.appName, pkgName, valueName, MapOrPanicPromise(entity, promise))
	return e.AddEntity([]string{pkgName}, valueName, valueType, promise)
}

func (e *Exposer) ExposeOrPanic(entity any, packageName string, name string) {
	if err := e.Expose(entity, packageName, name); err != nil {
		panic(fmt.Errorf("failed to expose: %w", err))
	}
}

func (e *Exposer) Expose(entity any, packageName string, name string) error {
	setNamespace(e.appName, packageName, name, MapOrPanic(entity))
	return e.AddEntity([]string{packageName}, name, reflect.ValueOf(entity).Type(), false)
}

var namespaceCleaner = regexp.MustCompile(`(\W)`)

func (e *Exposer) AddEntity(namespace []string, name string, typeDef reflect.Type, promise bool) error {
	layer := e.ensureNamespaceExists(namespace)

	if layer.Entities == nil {
		layer.Entities = make(map[string]reflect.Type)
	}

	if _, ok := layer.Entities[name]; ok {
		return fmt.Errorf("namespace %s already contains entity %s", strings.Join(namespace, "."), name)
	}

	layer.Entities[name] = typeDef

	if promise {
		if layer.Promises == nil {
			layer.Promises = make(map[string]bool)
		}

		layer.Promises[name] = promise
	}

	e.checkAddDefinition(typeDef)

	return nil
}

func (e *Exposer) AddDefinition(typeDef reflect.Type) error {
	if typeDef.Kind() != reflect.Struct {
		return fmt.Errorf("only struct types can be added as definitions")
	}

	namespace, nameWithTypes, _ := strings.Cut(typeDef.String(), ".")
	name, _, _ := strings.Cut(nameWithTypes, "[")

	layer := e.ensureNamespaceExists([]string{namespace})

	if layer.Definitions == nil {
		layer.Definitions = make(map[string]reflect.Type)
	}

	if _, ok := layer.Definitions[name]; ok {
		return fmt.Errorf("namespace %s already contains definition %s", namespace, name)
	}

	layer.Definitions[name] = typeDef

	for i := 0; i < typeDef.NumField(); i++ {
		field := typeDef.Field(i)
		if field.PkgPath != "" {
			continue
		}

		if value, ok := field.Tag.Lookup("crystalline"); ok {
			if strings.Contains(value, "not_nil") {
				if layer.NotNil == nil {
					layer.NotNil = make(map[string]bool)
				}

				layer.NotNil[field.Name] = true
			}
		}
		e.checkAddDefinition(field.Type)
	}

	for i := 0; i < typeDef.NumMethod(); i++ {
		method := typeDef.Method(i)
		if method.PkgPath != "" {
			continue
		}

		if inner, ok := ignored[typeDef.String()]; ok {
			if inner[method.Name] {
				continue
			}
		}

		e.checkAddDefinition(method.Type)
		e.processFunctionMeta(method.Func.Pointer(), name)
	}

	newInstance := reflect.New(typeDef)
	for i := 0; i < newInstance.NumMethod(); i++ {
		method := newInstance.Type().Method(i)
		if method.PkgPath != "" {
			continue
		}

		if inner, ok := ignored[typeDef.String()]; ok {
			if inner[method.Name] {
				continue
			}
		}

		e.checkAddDefinition(method.Type)
		e.processFunctionMeta(method.Func.Pointer(), name)
	}

	return nil
}

func (e *Exposer) checkAddDefinition(typeDef reflect.Type) {
	switch typeDef.Kind() {
	case reflect.Struct:
		_ = e.AddDefinition(typeDef)
	case reflect.Map:
		e.checkAddDefinition(typeDef.Key())
		fallthrough
	case reflect.Pointer:
		fallthrough
	case reflect.Slice:
		fallthrough
	case reflect.Array:
		e.checkAddDefinition(typeDef.Elem())
	case reflect.Func:
		for i := 0; i < typeDef.NumIn(); i++ {
			e.checkAddDefinition(typeDef.In(i))
		}
		for i := 0; i < typeDef.NumOut(); i++ {
			e.checkAddDefinition(typeDef.Out(i))
		}
	}
}

func (e *Exposer) ensureNamespaceExists(namespace []string) *Definition {
	cleanNamespace := make([]string, len(namespace))
	for i, s := range namespace {
		cleanNamespace[i] = namespaceCleaner.ReplaceAllLiteralString(s, "_")
	}

	layer := e.rootDefinition
	for _, ns := range cleanNamespace {
		if layer.Nested == nil {
			layer.Nested = make(map[string]*Definition)
		}

		if _, ok := layer.Nested[ns]; !ok {
			layer.Nested[ns] = &Definition{
				Name: ns,
			}
		}

		layer = layer.Nested[ns]
	}

	return layer
}

func (e *Exposer) Build() (string, string, error) {
	var tsdFile strings.Builder
	var jsFile strings.Builder

	jsFile.WriteString(`const wrap = (fn) => {
  return (...args) => {
    const result = fn.call(undefined, ...args);
    if (globalThis.goInternalError) {
      const error = new Error(globalThis.goInternalError);
      globalThis.goInternalError = undefined;
      throw error;
    }
    return result;
  }
};`)
	jsFile.WriteString("\n\n")

	defTsdFile, defJsFile, err := e.rootDefinition.Serialize(context.Background(), e.appName, []string{})
	if err != nil {
		return "", "", err
	}

	tsdFile.WriteString(defTsdFile)
	jsFile.WriteString(defJsFile)

	return strings.TrimSpace(tsdFile.String()), strings.TrimSpace(jsFile.String()), nil
}

func (e *Exposer) processFunctionMeta(pointer uintptr, interfaceName string) {
	pc := runtime.FuncForPC(pointer)

	splitDef := strings.Split(path.Base(pc.Name()), ".")
	pkgName := splitDef[0]
	valueName := splitDef[len(splitDef)-1]

	funcDecl := findFunction(pointer)
	if funcDecl == nil {
		return
	}

	layer := e.ensureNamespaceExists([]string{pkgName})
	if layer.FuncMeta == nil {
		layer.FuncMeta = make(map[string]map[string]*FuncMeta)
	}

	if _, ok := layer.FuncMeta[interfaceName]; !ok {
		layer.FuncMeta[interfaceName] = make(map[string]*FuncMeta)
	}

	argNames := make([]string, 0)
	for _, field := range funcDecl.Type.Params.List {
		for _, name := range field.Names {
			argNames = append(argNames, name.Name)
		}
	}

	promise := false
	if funcDecl.Doc != nil {
		for _, comment := range funcDecl.Doc.List {
			if comment != nil {
				if strings.Contains(comment.Text, "crystalline:promise") {
					promise = true
				}
			}
		}
	}

	layer.FuncMeta[interfaceName][valueName] = &FuncMeta{
		ArgNames: argNames,
		Promise:  promise,
	}
}
