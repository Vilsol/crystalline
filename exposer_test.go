package crystalline

import (
	"errors"
	"reflect"
	"testing"

	"github.com/MarvinJWendt/testza"

	"github.com/Vilsol/crystalline/nested"
)

type SomeObj struct {
	Name             string
	Data             map[uint32]float64
	Nested           nested.AnotherObj
	VoidFunc         func()
	MightBeNil       []string
	DefinitelyNotNil []string        `crystalline:"not_nil"`
	NotNilMap        map[string]bool `crystalline:"not_nil"`
}

type GenericStruct[K string, V nested.AnotherObj] struct {
	FieldOne K
	FieldTwo V
}

type InheritedObj struct {
	nested.AnotherObj
}

func (g GenericStruct[K, V]) GenericFunc(a K) (K, V) {
	return g.FieldOne, g.FieldTwo
}

func (g SomeObj) NoPointer(first string, second int) {
}

func (g *SomeObj) WithPointer(first float64, second bool) {
}

// crystalline:promise
func (g *SomeObj) Promised() {
}

func SomeFunc(name string, a bool) (string, bool) {
	return "hello, " + name, !a
}

func ErrorFunc() error {
	return errors.New("sample error")
}

func InterfaceFunc() interface{} {
	return "test"
}

func PromiseFunc() int {
	return 10
}

func FuncFunc(f func() string) string {
	return "Hello, " + f()
}

func ByteFunc(f func() []byte) []byte {
	return f()
}

var (
	ExposeArrayTest  = [1]string{"hello"}
	ExposeSliceTest  = []float64{10}
	ExposeStringTest = "hello"
	ExposeIntTest    = 10
	ExposeStructTest = SomeObj{
		Name: "Bob",
		Data: map[uint32]float64{
			0: 1.23,
			1: 4.56,
			2: 7.89,
		},
		Nested: nested.AnotherObj{
			SomeValue: []int{0, 1, 1, 2, 3, 5, 8},
		},
	}
	ExposePointerTest   = &ExposeStructTest
	ExposeMapTest       = map[uint32]float32{0: 1.23}
	ExposeGenericStruct = GenericStruct[string, nested.AnotherObj]{
		FieldOne: "hello",
		FieldTwo: nested.AnotherObj{
			SomeValue: []int{100, 200, 300, 400},
		},
	}
	ExposeInheritedStructTest = InheritedObj{
		ExposeStructTest.Nested,
	}
)

type GlobalTestObj struct {
}

func TestExposer(t *testing.T) {
	appName := "app"

	e := NewExposer(appName)

	testza.AssertNoError(t, e.ExposeFunc(SomeFunc))
	testza.AssertNoError(t, e.ExposeFunc(ErrorFunc))
	testza.AssertNoError(t, e.ExposeFunc(InterfaceFunc))
	testza.AssertNoError(t, e.ExposeFuncPromise(PromiseFunc, true))
	testza.AssertNoError(t, e.ExposeFunc(FuncFunc))
	testza.AssertNoError(t, e.ExposeFunc(ByteFunc))

	testza.AssertNoError(t, e.Expose(ExposeArrayTest, "crystalline", "ExposeArrayTest"))
	testza.AssertNoError(t, e.Expose(ExposeSliceTest, "crystalline", "ExposeSliceTest"))
	testza.AssertNoError(t, e.Expose(ExposeStringTest, "crystalline", "ExposeStringTest"))
	testza.AssertNoError(t, e.Expose(ExposeIntTest, "crystalline", "ExposeIntTest"))
	testza.AssertNoError(t, e.Expose(ExposeStructTest, "crystalline", "ExposeStructTest"))
	testza.AssertNoError(t, e.Expose(ExposePointerTest, "crystalline", "ExposePointerTest"))
	testza.AssertNoError(t, e.Expose(ExposeMapTest, "crystalline", "ExposeMapTest"))
	testza.AssertNoError(t, e.Expose(ExposeGenericStruct, "crystalline", "ExposeGenericStruct"))
	testza.AssertNoError(t, e.Expose(ExposeInheritedStructTest, "crystalline", "ExposeInheritedStructTest"))

	testza.AssertNoError(t, e.AddEntity(nil, "GlobalTest", reflect.TypeOf(GlobalTestObj{}), false))

	tsdFile, jsFile, err := e.Build()
	testza.AssertNoError(t, err)

	testza.AssertEqual(t, `const wrap = (fn) => {
  return (...args) => {
    const result = fn.call(undefined, ...args);
    if (globalThis.goInternalError) {
      const error = new Error(result.message);
      globalThis.goInternalError = undefined;
      throw error;
    }
    return result;
  }
};

export let GlobalTest;
export let crystalline;

export const initializeCrystalline = () => {
  GlobalTest = globalThis['go']['app']['GlobalTest'];
  crystalline = {
    ByteFunc: wrap(globalThis['go']['app']['crystalline']['ByteFunc']),
    ErrorFunc: wrap(globalThis['go']['app']['crystalline']['ErrorFunc']),
    ExposeArrayTest: wrap(globalThis['go']['app']['crystalline']['ExposeArrayTest']),
    ExposeGenericStruct: wrap(globalThis['go']['app']['crystalline']['ExposeGenericStruct']),
    ExposeInheritedStructTest: wrap(globalThis['go']['app']['crystalline']['ExposeInheritedStructTest']),
    ExposeIntTest: wrap(globalThis['go']['app']['crystalline']['ExposeIntTest']),
    ExposeMapTest: wrap(globalThis['go']['app']['crystalline']['ExposeMapTest']),
    ExposePointerTest: wrap(globalThis['go']['app']['crystalline']['ExposePointerTest']),
    ExposeSliceTest: wrap(globalThis['go']['app']['crystalline']['ExposeSliceTest']),
    ExposeStringTest: wrap(globalThis['go']['app']['crystalline']['ExposeStringTest']),
    ExposeStructTest: wrap(globalThis['go']['app']['crystalline']['ExposeStructTest']),
    FuncFunc: wrap(globalThis['go']['app']['crystalline']['FuncFunc']),
    InterfaceFunc: wrap(globalThis['go']['app']['crystalline']['InterfaceFunc']),
    PromiseFunc: wrap(globalThis['go']['app']['crystalline']['PromiseFunc']),
    SomeFunc: wrap(globalThis['go']['app']['crystalline']['SomeFunc'])
  };
};`, jsFile)

	testza.AssertEqual(t, `export const GlobalTest = crystalline.GlobalTestObj;
export declare namespace crystalline {
  interface GenericStruct {
    FieldOne: string;
    FieldTwo: nested.AnotherObj;
    GenericFunc(a: string): [string, nested.AnotherObj];
  }
  interface GlobalTestObj {
  }
  interface InheritedObj {
    AnotherObj: nested.AnotherObj;
  }
  interface SomeObj {
    Name: string;
    Data?: Record<number, number>;
    Nested: nested.AnotherObj;
    VoidFunc: () => void;
    MightBeNil?: Array<string>;
    DefinitelyNotNil: Array<string>;
    NotNilMap: Record<string, boolean>;
    NoPointer(first: string, second: number): void;
    Promised(): Promise<void>;
    WithPointer(first: number, second: boolean): void;
  }
  function ByteFunc(f: () => Promise<(Uint8Array | undefined)>): Promise<(Uint8Array | undefined)>;
  function ErrorFunc(): Error;
  const ExposeArrayTest: Array<string> | undefined;
  const ExposeGenericStruct: crystalline.GenericStruct;
  const ExposeInheritedStructTest: crystalline.InheritedObj;
  const ExposeIntTest: number;
  const ExposeMapTest: Record<number, number> | undefined;
  const ExposePointerTest: crystalline.SomeObj | undefined;
  const ExposeSliceTest: Array<number> | undefined;
  const ExposeStringTest: string;
  const ExposeStructTest: crystalline.SomeObj;
  function FuncFunc(f: () => Promise<string>): Promise<string>;
  function InterfaceFunc(): (unknown | undefined);
  function PromiseFunc(): Promise<number>;
  function SomeFunc(name: string, a: boolean): [string, boolean];
}
export declare namespace nested {
  interface AnotherObj {
    SomeValue?: Array<number>;
  }
}
export const initializeCrystalline: () => void;`, tsdFile)
}
