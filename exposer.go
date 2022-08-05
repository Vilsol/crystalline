package crystalline

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
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
		panic(err)
	}
}

func (e *Exposer) ExposeFuncOrPanicPromise(entity any) {
	if err := e.ExposeFuncPromise(entity, true); err != nil {
		panic(err)
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
	e.extractArgNames(pointer, "")

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
		panic(err)
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
		e.checkAddDefinition(typeDef.Field(i).Type)
	}

	for i := 0; i < typeDef.NumMethod(); i++ {
		e.checkAddDefinition(typeDef.Method(i).Type)
		e.extractArgNames(typeDef.Method(i).Func.Pointer(), name)
	}

	newInstance := reflect.New(typeDef)
	for i := 0; i < newInstance.NumMethod(); i++ {
		e.checkAddDefinition(newInstance.Type().Method(i).Type)
		e.extractArgNames(newInstance.Type().Method(i).Func.Pointer(), name)
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

	defTsdFile, defJsFile, err := e.rootDefinition.Serialize(e.appName, []string{})
	if err != nil {
		return "", "", err
	}

	tsdFile.WriteString(defTsdFile)
	jsFile.WriteString(defJsFile)

	return strings.TrimSpace(tsdFile.String()), strings.TrimSpace(jsFile.String()), nil
}

func (e *Exposer) extractArgNames(pointer uintptr, interfaceName string) {
	pc := runtime.FuncForPC(pointer)

	splitDef := strings.Split(path.Base(pc.Name()), ".")
	pkgName := splitDef[0]
	valueName := splitDef[len(splitDef)-1]

	filePath, lineNumber := pc.FileLine(pointer)

	// Extract argument names if possible
	fileData, err := os.ReadFile(filePath)
	if err == nil {
		fileSet := token.NewFileSet()
		f, err := parser.ParseFile(fileSet, filePath, string(fileData), 0)
		if err == nil {
			for _, decl := range f.Decls {
				found := false
				switch castDecl := decl.(type) {
				case *ast.FuncDecl:
					if castDecl.Name.Name == valueName && castDecl.Type.Params != nil {
						pos := fileSet.Position(castDecl.Pos())
						if pos.Line == lineNumber || pos.Line == lineNumber-1 {
							layer := e.ensureNamespaceExists([]string{pkgName})
							if layer.FuncArgNames == nil {
								layer.FuncArgNames = make(map[string]map[string][]string)
							}

							if _, ok := layer.FuncArgNames[interfaceName]; !ok {
								layer.FuncArgNames[interfaceName] = make(map[string][]string)
							}

							argNames := make([]string, 0)
							for _, field := range castDecl.Type.Params.List {
								for _, name := range field.Names {
									argNames = append(argNames, name.Name)
								}
							}
							layer.FuncArgNames[interfaceName][valueName] = argNames

							found = true
						}
					}
				}

				if found {
					break
				}
			}
		}
	}
}
