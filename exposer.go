package crystalline

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
)

// Exposer collects the Go functions, values and types that should be visible
// from JavaScript, and renders them into a JS module plus a matching
// TypeScript declaration file.
//
// Exposing has two effects: it publishes the entity on the JS side under
// go.<app>.<namespace>.<name>, and it records the type so that Build can
// declare it. An Exposer is not safe for concurrent use.
type Exposer struct {
	appName        string
	rootDefinition *definition
	style          jsStyle
}

// ExposerOption configures an Exposer at construction time.
type ExposerOption func(*Exposer)

// WithQuoteStyle sets the quote character used in the generated JavaScript,
// so the output can match the project's formatter. Defaults to a single quote.
func WithQuoteStyle(quote string) ExposerOption {
	return func(e *Exposer) {
		e.style.quote = quote
	}
}

// WithTrailingComma emits trailing commas in the generated JavaScript.
func WithTrailingComma() ExposerOption {
	return func(e *Exposer) {
		e.style.trailingComma = true
	}
}

// NewExposer creates an Exposer that publishes everything under the global
// go.<appName> object on the JS side.
func NewExposer(appName string, opts ...ExposerOption) *Exposer {
	e := &Exposer{
		appName:        appName,
		rootDefinition: &definition{},
		style:          defaultStyle(),
	}

	for _, opt := range opts {
		opt(e)
	}

	return e
}

// Option customises how an entity is exposed.
type Option func(*exposeOptions)

type exposeOptions struct {
	promise   bool
	namespace string
}

// AsPromise makes an exposed function return a JS Promise. The Go call runs on
// its own goroutine, so it does not block the JS event loop.
//
// Functions taking a callback are always promises, whether or not this is set:
// the callback cannot be serviced without yielding to the event loop.
func AsPromise() Option {
	return func(o *exposeOptions) {
		o.promise = true
	}
}

// InNamespace overrides the JS namespace an entity is placed under. It
// defaults to the name of the Go package the entity comes from.
func InNamespace(name string) Option {
	return func(o *exposeOptions) {
		o.namespace = name
	}
}

func buildOptions(opts []Option) exposeOptions {
	var built exposeOptions

	for _, opt := range opts {
		opt(&built)
	}

	return built
}

// ExposeFunc exposes fn to JS as go.<app>.<package>.<FuncName>.
//
// Both the package and the function name are recovered from the Go runtime,
// so there is nothing to keep in sync by hand:
//
//	e.ExposeFunc(mypkg.Greet)          // go.app.mypkg.Greet
//	e.ExposeFunc(mypkg.Load, AsPromise())
func (e *Exposer) ExposeFunc(fn any, opts ...Option) error {
	options := buildOptions(opts)

	value := reflect.ValueOf(fn)
	if !value.IsValid() || value.Kind() != reflect.Func {
		return fmt.Errorf("ExposeFunc needs a function, got %T (use ExposeValue for anything else)", fn)
	}

	pointer := value.Pointer()
	e.processFunctionMeta(pointer, "")

	pkgName, valueName, err := splitFuncName(runtime.FuncForPC(pointer).Name())
	if err != nil {
		return err
	}

	if options.namespace != "" {
		pkgName = options.namespace
	}

	return e.expose(pkgName, valueName, fn, value.Type(), options)
}

// MustExposeFunc is ExposeFunc, panicking instead of returning an error. It is
// meant for package initialisation, where a failure is a programming mistake.
func (e *Exposer) MustExposeFunc(fn any, opts ...Option) {
	if err := e.ExposeFunc(fn, opts...); err != nil {
		panic(fmt.Errorf("failed exposing func: %w", err))
	}
}

// ExposeValue exposes a non-function value as go.<app>.<package>.<name>.
//
// A value carries no name at runtime, so name has to be given; the package is
// taken from the caller. Override it with InNamespace.
func (e *Exposer) ExposeValue(name string, value any, opts ...Option) error {
	options := buildOptions(opts)

	if options.namespace == "" {
		pkgName, err := callerPackage(2)
		if err != nil {
			return err
		}

		options.namespace = pkgName
	}

	return e.exposeValue(name, value, options)
}

// MustExposeValue is ExposeValue, panicking instead of returning an error.
func (e *Exposer) MustExposeValue(name string, value any, opts ...Option) {
	options := buildOptions(opts)

	if options.namespace == "" {
		pkgName, err := callerPackage(2)
		if err != nil {
			panic(fmt.Errorf("failed exposing value: %w", err))
		}

		options.namespace = pkgName
	}

	if err := e.exposeValue(name, value, options); err != nil {
		panic(fmt.Errorf("failed exposing value: %w", err))
	}
}

func (e *Exposer) exposeValue(name string, value any, options exposeOptions) error {
	if name == "" {
		return errors.New("cannot expose a value without a name")
	}

	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return fmt.Errorf("cannot expose nil as %q", name)
	}

	return e.expose(options.namespace, name, value, reflected.Type(), options)
}

func (e *Exposer) expose(namespace string, name string, value any, typeDef reflect.Type, options exposeOptions) error {
	mapped, err := mapValue(value, options.promise)
	if err != nil {
		return fmt.Errorf("exposing %s.%s: %w", namespace, name, err)
	}

	setNamespace(e.appName, namespace, name, mapped)

	return e.AddEntity([]string{namespace}, name, typeDef, options.promise)
}

// callerPackage recovers the package name of the frame skip levels above it.
func callerPackage(skip int) (string, error) {
	pc, _, _, ok := runtime.Caller(skip)
	if !ok {
		return "", errors.New("could not determine the calling package, pass InNamespace to set it explicitly")
	}

	pkgName, _, err := splitFuncName(runtime.FuncForPC(pc).Name())
	if err != nil {
		return "", fmt.Errorf("could not determine the calling package, pass InNamespace to set it explicitly: %w", err)
	}

	return pkgName, nil
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

// AddEntity declares an entity in the generated output without publishing a
// value for it. It is the low level building block behind ExposeFunc and
// ExposeValue; reach for it when a binding is set up by hand, or to declare
// something that only exists on the JS side.
//
// A nil or empty namespace places the entity at the root of the module.
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

// AddDefinition records a struct type so that Build emits a TypeScript
// interface for it, pulling in every type it references.
//
// Exposing a value does this automatically. Call it directly only to declare a
// type that no exposed entity mentions.
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

		if value, ok := field.Tag.Lookup(tagName); ok {
			if err := validateTag(value); err != nil {
				return fmt.Errorf("%s.%s: %w", name, field.Name, err)
			}

			if tagHasOption(value, tagNotNil) {
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

		if isIgnored(typeDef, method.Name) {
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

		if isIgnored(typeDef, method.Name) {
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

func (e *Exposer) ensureNamespaceExists(namespace []string) *definition {
	cleanNamespace := make([]string, len(namespace))
	for i, s := range namespace {
		cleanNamespace[i] = namespaceCleaner.ReplaceAllLiteralString(s, "_")
	}

	layer := e.rootDefinition
	for _, ns := range cleanNamespace {
		if layer.Nested == nil {
			layer.Nested = make(map[string]*definition)
		}

		if _, ok := layer.Nested[ns]; !ok {
			layer.Nested[ns] = &definition{
				Name: ns,
			}
		}

		layer = layer.Nested[ns]
	}

	return layer
}

// Output is the pair of files a build produces.
type Output struct {
	// JavaScript is the ES module that binds the wasm exports into a
	// namespaced object graph. Import it and call initializeCrystalline()
	// once the wasm module is running.
	JavaScript string

	// TypeScript is the matching .d.ts declaration file.
	TypeScript string
}

// WriteFiles writes the generated sources to the given paths, creating parent
// directories as needed.
func (o Output) WriteFiles(jsPath string, tsPath string) error {
	for target, content := range map[string]string{jsPath: o.JavaScript, tsPath: o.TypeScript} {
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", target, err)
		}

		if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", target, err)
		}
	}

	return nil
}

// Build renders everything exposed so far into a JS module and a matching
// TypeScript declaration file.
func (e *Exposer) Build() (Output, error) {
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

	defTsdFile, defJsFile, err := e.rootDefinition.serialize(context.Background(), e.appName, []string{}, e.style)
	if err != nil {
		return Output{}, err
	}

	tsdFile.WriteString(defTsdFile)
	jsFile.WriteString(defJsFile)

	return Output{
		JavaScript: strings.TrimSpace(jsFile.String()),
		TypeScript: strings.TrimSpace(tsdFile.String()),
	}, nil
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
		layer.FuncMeta = make(map[string]map[string]*funcMeta)
	}

	if _, ok := layer.FuncMeta[interfaceName]; !ok {
		layer.FuncMeta[interfaceName] = make(map[string]*funcMeta)
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

	layer.FuncMeta[interfaceName][valueName] = &funcMeta{
		ArgNames: argNames,
		Promise:  promise,
	}
}
