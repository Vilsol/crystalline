package crystalline

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/MarvinJWendt/testza"
)

type NamedBytes []byte

type NotNilA struct {
	Shared []string `crystalline:"not_nil"`
}

type NotNilB struct {
	Shared []string
}

type Pair[T any] struct {
	Value T
}

func TestNamedByteSliceIsUint8Array(t *testing.T) {
	d := &definition{}

	for _, typeDef := range []reflect.Type{
		reflect.TypeOf([]byte(nil)),
		reflect.TypeOf(NamedBytes(nil)), // defined type over []byte
		reflect.TypeOf([4]byte{}),       // fixed size byte array
	} {
		name, _, err := d.typeToJSName(context.Background(), "", typeDef, false, "", false)
		testza.AssertNoError(t, err)
		testza.AssertEqual(t, "Uint8Array", name, typeDef.String())
	}
}

func TestContextStepsDoNotAlias(t *testing.T) {
	ctx := context.Background()
	ctx = withContextStep(ctx, "a")
	ctx = withContextStep(ctx, "b")
	// The third append leaves spare capacity in the backing array, which is
	// what sibling branches then race to write into.
	ctx = withContextStep(ctx, "c")

	first := withContextStep(ctx, "x")
	second := withContextStep(ctx, "y")

	testza.AssertEqual(t, "a.b.c.x", getContextSteps(first))
	testza.AssertEqual(t, "a.b.c.y", getContextSteps(second))
}

func TestContextStepKeyIsPrivate(t *testing.T) {
	// An unrelated package using the zero struct{} as its own key must not be
	// mistaken for our step list.
	ctx := context.WithValue(context.Background(), struct{}{}, "not a step list")
	testza.AssertEqual(t, ".", getContextSteps(ctx))
}

func TestNotNilIsScopedPerStruct(t *testing.T) {
	e := NewExposer("app")
	testza.AssertNoError(t, e.AddDefinition(reflect.TypeOf(NotNilA{})))
	testza.AssertNoError(t, e.AddDefinition(reflect.TypeOf(NotNilB{})))

	out, err := e.Build()
	testza.AssertNoError(t, err)

	tsd := out.TypeScript

	testza.AssertTrue(t, strings.Contains(tsd, "interface NotNilA {\n    Shared: Array<string>;"),
		"NotNilA.Shared must stay required:\n"+tsd)
	testza.AssertTrue(t, strings.Contains(tsd, "interface NotNilB {\n    Shared?: Array<string>;"),
		"NotNilB.Shared must stay optional:\n"+tsd)
}

func TestCollidingGenericDefinitionsError(t *testing.T) {
	e := NewExposer("app")

	testza.AssertNoError(t, e.AddDefinition(reflect.TypeOf(Pair[string]{})))

	// A second instantiation flattens to the same name and would silently
	// replace the first, so it has to be reported rather than dropped.
	err := e.AddDefinition(reflect.TypeOf(Pair[int]{}))
	testza.AssertNotNil(t, err)
	testza.AssertTrue(t, strings.Contains(err.Error(), "Pair"), err)
}

func TestReAddingSameDefinitionIsNotAnError(t *testing.T) {
	e := NewExposer("app")

	testza.AssertNoError(t, e.AddDefinition(reflect.TypeOf(NotNilA{})))
	testza.AssertNoError(t, e.AddDefinition(reflect.TypeOf(NotNilA{})))
}

func TestExposeMethodValueUsesMethodName(t *testing.T) {
	e := NewExposer("app")
	obj := SomeObj{}

	testza.AssertNoError(t, e.ExposeFunc(obj.NoPointer))

	out, err := e.Build()
	testza.AssertNoError(t, err)

	jsFile := out.JavaScript

	testza.AssertTrue(t, strings.Contains(jsFile, "NoPointer"),
		"method value must be exposed under its method name:\n"+jsFile)
	testza.AssertFalse(t, strings.Contains(jsFile, "-fm"),
		"method value suffix must be stripped:\n"+jsFile)
}

func TestUnconvertableTypeReturnsError(t *testing.T) {
	d := &definition{}

	_, _, err := d.typeToJSName(context.Background(), "", reflect.TypeOf(make(chan int)), false, "", false)
	testza.AssertNotNil(t, err)
	testza.AssertTrue(t, strings.Contains(err.Error(), "chan"), err)
}

func TestNonStructInterfaceReturnsError(t *testing.T) {
	d := &definition{}

	_, err := d.typeToInterface(context.Background(), "Nope", reflect.TypeOf(0))
	testza.AssertNotNil(t, err)
}

func TestUnsupportedKindReturnsError(t *testing.T) {
	_, err := Map(make(chan int))
	testza.AssertNotNil(t, err)
}

func TestMarkersAreConcurrencySafe(t *testing.T) {
	typ := reflect.TypeOf(MarkerObj{})

	var wg sync.WaitGroup

	for i := 0; i < 64; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for j := 0; j < 200; j++ {
				testza.AssertNoError(t, MarkPromise(typ, "Method"))
				testza.AssertNoError(t, MarkIgnored(typ, "PointerMethod"))

				testza.AssertTrue(t, isPromise(typ, "Method"))
				testza.AssertTrue(t, isIgnored(typ, "PointerMethod"))
				testza.AssertFalse(t, isIgnored(typ, "Method"))
			}
		}()
	}

	wg.Wait()
}

type badInner struct {
	Ch chan int
}

type BadOuter struct {
	Fine  string
	Inner badInner
}

type BadSliceHolder struct {
	Items []chan int
}

func TestMapErrorsNameTheOffendingField(t *testing.T) {
	_, err := Map(BadOuter{})
	testza.AssertNotNil(t, err)
	testza.AssertTrue(t, strings.Contains(err.Error(), "Inner.Ch"),
		"error must name the path to the bad field, got: "+err.Error())

	_, err = Map(BadSliceHolder{Items: []chan int{nil}})
	testza.AssertNotNil(t, err)
	testza.AssertTrue(t, strings.Contains(err.Error(), "Items[0]"),
		"error must name the slice index, got: "+err.Error())

	_, err = Map(map[string]badInner{"key": {}})
	testza.AssertNotNil(t, err)
	testza.AssertTrue(t, strings.Contains(err.Error(), "[key].Ch"),
		"error must name the map key, got: "+err.Error())
}

type MarkerObj struct {
	Value string
}

func (m MarkerObj) Method() {}

func (m *MarkerObj) PointerMethod() {}

type BadTagObj struct {
	Items []string `crystalline:"notnil"`
}

func TestMarkersRejectUnknownMethods(t *testing.T) {
	typ := reflect.TypeOf(MarkerObj{})

	testza.AssertNoError(t, MarkPromise(typ, "Method"))
	testza.AssertNoError(t, MarkIgnored(typ, "PointerMethod"))

	// A typo used to be a silent no-op.
	err := MarkPromise(typ, "Methd")
	testza.AssertNotNil(t, err)
	testza.AssertTrue(t, strings.Contains(err.Error(), "Methd"), err)

	err = MarkIgnored(reflect.TypeOf("not a struct"), "Method")
	testza.AssertNotNil(t, err)
}

func TestUnknownStructTagOptionIsRejected(t *testing.T) {
	e := NewExposer("app")

	err := e.AddDefinition(reflect.TypeOf(BadTagObj{}))
	testza.AssertNotNil(t, err, "a misspelled tag option must not be ignored")
	testza.AssertTrue(t, strings.Contains(err.Error(), "notnil"), err)
}

func TestOutputWriteFiles(t *testing.T) {
	e := NewExposer("app")
	testza.AssertNoError(t, e.ExposeFunc(SomeFunc))

	out, err := e.Build()
	testza.AssertNoError(t, err)

	dir := t.TempDir()
	jsPath := filepath.Join(dir, "nested", "crystalline.js")
	tsPath := filepath.Join(dir, "nested", "crystalline.d.ts")

	testza.AssertNoError(t, out.WriteFiles(jsPath, tsPath))

	js, err := os.ReadFile(jsPath)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, out.JavaScript, string(js))

	ts, err := os.ReadFile(tsPath)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, out.TypeScript, string(ts))
}

func TestJSStyleIsPerExposer(t *testing.T) {
	build := func(opts ...ExposerOption) string {
		e := NewExposer("app", opts...)
		testza.AssertNoError(t, e.ExposeFunc(SomeFunc))
		testza.AssertNoError(t, e.ExposeFunc(ErrorFunc))

		out, err := e.Build()
		testza.AssertNoError(t, err)

		return out.JavaScript
	}

	single := build()
	testza.AssertTrue(t, strings.Contains(single, "globalThis['go']['app']"), single)

	// Styling used to be a package global, so two Exposers could not differ.
	double := build(WithQuoteStyle(`"`), WithTrailingComma())
	testza.AssertTrue(t, strings.Contains(double, `globalThis["go"]["app"]`), double)
	testza.AssertTrue(t, strings.Contains(double, "]),\n  };"), double)

	testza.AssertTrue(t, strings.Contains(single, "globalThis['go']['app']"),
		"the first Exposer's style must be unaffected by the second")
}

func TestInitGuardIsEmitted(t *testing.T) {
	e := NewExposer("app")
	testza.AssertNoError(t, e.ExposeFunc(SomeFunc))

	out, err := e.Build()
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(out.JavaScript, "Start the Go wasm module"),
		"calling initializeCrystalline() too early must fail with a clear message:\n"+out.JavaScript)
}
