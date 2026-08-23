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

	pkgName, valueName, err := splitFuncName(runtime.FuncForPC(pointer).Name())
	if err != nil {
		return err
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

// methodValueSuffix is appended by the runtime to bound method values,
// e.g. pkg.Type.Method-fm.
const methodValueSuffix = "-fm"

// splitFuncName resolves a runtime function name into its package and its own
// name. The trailing segment is always the function, so methods and method
// values resolve to the method rather than to the receiver type.
func splitFuncName(fullName string) (string, string, error) {
	splitDef := strings.Split(path.Base(fullName), ".")
	if len(splitDef) < 2 {
		return "", "", fmt.Errorf("could not determine function name or package from %q", fullName)
	}

	pkgName := splitDef[0]
	valueName := strings.TrimSuffix(splitDef[len(splitDef)-1], methodValueSuffix)

	if pkgName == "" || valueName == "" {
		return "", "", fmt.Errorf("could not determine function name or package from %q", fullName)
	}

	return pkgName, valueName, nil
}

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

	return e.checkAddDefinition(typeDef)
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

	if existing, ok := layer.Definitions[name]; ok {
		if existing == typeDef {
			return nil
		}

		// Generic instantiations flatten to the same bare name, so two of them
		// would silently replace one another in the emitted declarations.
		return fmt.Errorf("namespace %s already contains definition %s as %s, cannot also add %s", namespace, name, existing, typeDef)
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
					layer.NotNil = make(map[string]map[string]bool)
				}

				if layer.NotNil[name] == nil {
					layer.NotNil[name] = make(map[string]bool)
				}

				layer.NotNil[name][field.Name] = true
			}
		}

		if err := e.checkAddDefinition(field.Type); err != nil {
			return err
		}
	}

	for i := 0; i < typeDef.NumMethod(); i++ {
		method := typeDef.Method(i)
		if method.PkgPath != "" {
			continue
		}

		if isIgnored(typeDef.String(), method.Name) {
			continue
		}

		if err := e.checkAddDefinition(method.Type); err != nil {
			return err
		}

		e.processFunctionMeta(method.Func.Pointer(), name)
	}

	newInstance := reflect.New(typeDef)
	for i := 0; i < newInstance.NumMethod(); i++ {
		method := newInstance.Type().Method(i)
		if method.PkgPath != "" {
			continue
		}

		if isIgnored(typeDef.String(), method.Name) {
			continue
		}

		if err := e.checkAddDefinition(method.Type); err != nil {
			return err
		}

		e.processFunctionMeta(method.Func.Pointer(), name)
	}

	return nil
}

func (e *Exposer) checkAddDefinition(typeDef reflect.Type) error {
	switch typeDef.Kind() {
	case reflect.Struct:
		return e.AddDefinition(typeDef)
	case reflect.Map:
		if err := e.checkAddDefinition(typeDef.Key()); err != nil {
			return err
		}

		return e.checkAddDefinition(typeDef.Elem())
	case reflect.Pointer:
		fallthrough
	case reflect.Slice:
		fallthrough
	case reflect.Array:
		return e.checkAddDefinition(typeDef.Elem())
	case reflect.Func:
		for i := 0; i < typeDef.NumIn(); i++ {
			if err := e.checkAddDefinition(typeDef.In(i)); err != nil {
				return err
			}
		}

		for i := 0; i < typeDef.NumOut(); i++ {
			if err := e.checkAddDefinition(typeDef.Out(i)); err != nil {
				return err
			}
		}
	}

	return nil
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
	pkgName, valueName, err := splitFuncName(runtime.FuncForPC(pointer).Name())
	if err != nil {
		return
	}

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
