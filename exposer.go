package crystalline

import (
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
		panic(err)
	}
}

func (e *Exposer) ExposeFunc(entity any) error {
	value := reflect.ValueOf(entity)
	valueType := value.Type()

	if valueType.Kind() != reflect.Func {
		return errors.New("can only expose functions without specifying package and name")
	}

	splitDef := strings.Split(path.Base(runtime.FuncForPC(value.Pointer()).Name()), ".")
	pkgName := splitDef[0]
	valueName := splitDef[1]

	if valueName == "" || pkgName == "" {
		return errors.New("could not determine function name or package")
	}

	return e.Expose(entity, pkgName, valueName)
}

func (e *Exposer) ExposeOrPanic(entity any, packageName string, name string) {
	if err := e.Expose(entity, packageName, name); err != nil {
		panic(err)
	}
}

func (e *Exposer) Expose(entity any, packageName string, name string) error {
	setNamespace(e.appName, packageName, name, MapOrPanic(entity))
	return e.AddEntity([]string{packageName}, name, reflect.ValueOf(entity).Type())
}

var namespaceCleaner = regexp.MustCompile(`(\W)`)

func (e *Exposer) AddEntity(namespace []string, name string, typeDef reflect.Type) error {
	layer := e.ensureNamespaceExists(namespace)

	if layer.Entities == nil {
		layer.Entities = make(map[string]reflect.Type)
	}

	if _, ok := layer.Entities[name]; ok {
		return fmt.Errorf("namespace %s already contains entity %s", strings.Join(namespace, "."), name)
	}

	layer.Entities[name] = typeDef

	e.checkAddDefinition(typeDef)

	return nil
}

func (e *Exposer) AddDefinition(typeDef reflect.Type) error {
	if typeDef.Kind() != reflect.Struct {
		return fmt.Errorf("only struct types can be added as definitions")
	}

	splitDef := strings.Split(typeDef.String(), ".")
	namespace := splitDef[:1]
	name := splitDef[1]

	layer := e.ensureNamespaceExists(namespace)

	if layer.Definitions == nil {
		layer.Definitions = make(map[string]reflect.Type)
	}

	if _, ok := layer.Definitions[name]; ok {
		return fmt.Errorf("namespace %s already contains definition %s", strings.Join(namespace, "."), name)
	}

	layer.Definitions[name] = typeDef

	for i := 0; i < typeDef.NumField(); i++ {
		e.checkAddDefinition(typeDef.Field(i).Type)
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
