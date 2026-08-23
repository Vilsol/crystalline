package crystalline

import (
	"context"
	"fmt"
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
	d := &Definition{}

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

	tsd, _, err := e.Build()
	testza.AssertNoError(t, err)

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

	_, jsFile, err := e.Build()
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(jsFile, "NoPointer"),
		"method value must be exposed under its method name:\n"+jsFile)
	testza.AssertFalse(t, strings.Contains(jsFile, "-fm"),
		"method value suffix must be stripped:\n"+jsFile)
}

func TestUnconvertableTypeReturnsError(t *testing.T) {
	d := &Definition{}

	_, _, err := d.typeToJSName(context.Background(), "", reflect.TypeOf(make(chan int)), false, "", false)
	testza.AssertNotNil(t, err)
	testza.AssertTrue(t, strings.Contains(err.Error(), "chan"), err)
}

func TestNonStructInterfaceReturnsError(t *testing.T) {
	d := &Definition{}

	_, err := d.typeToInterface(context.Background(), "Nope", reflect.TypeOf(0))
	testza.AssertNotNil(t, err)
}

func TestUnsupportedKindReturnsError(t *testing.T) {
	_, err := Map(make(chan int))
	testza.AssertNotNil(t, err)
}

func TestMarkersAreConcurrencySafe(t *testing.T) {
	var wg sync.WaitGroup

	for i := 0; i < 64; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			for j := 0; j < 200; j++ {
				entity := fmt.Sprintf("pkg.Type%d", i*200+j)
				MarkPromise(entity, "Fn")
				MarkIgnored(entity, "Fn")

				testza.AssertTrue(t, isPromise(entity, "Fn"))
				testza.AssertTrue(t, isIgnored(entity, "Fn"))
				testza.AssertFalse(t, isIgnored(entity, "Other"))
			}
		}(i)
	}

	wg.Wait()
}
