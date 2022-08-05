package crystalline

import (
	"errors"
	"reflect"
	"testing"

	"github.com/MarvinJWendt/testza"

	"github.com/Vilsol/crystalline/nested"
)

type SomeObj struct {
	Name     string
	Data     map[uint32]float64
	Nested   nested.AnotherObj
	VoidFunc func()
}

func (g SomeObj) NoPointer(first string, second int) {
}

func (g *SomeObj) WithPointer(first float64, second bool) {
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
	ExposePointerTest = &ExposeStructTest
	ExposeMapTest     = map[uint32]float32{0: 1.23}
)

type GlobalTestObj struct {
}

func TestExposer(t *testing.T) {
	appName := "app"

	e := NewExposer(appName)

	testza.AssertNoError(t, e.ExposeFunc(SomeFunc))
	testza.AssertNoError(t, e.ExposeFunc(ErrorFunc))
	testza.AssertNoError(t, e.ExposeFunc(InterfaceFunc))

	testza.AssertNoError(t, e.Expose(ExposeArrayTest, "crystalline", "ExposeArrayTest"))
	testza.AssertNoError(t, e.Expose(ExposeSliceTest, "crystalline", "ExposeSliceTest"))
	testza.AssertNoError(t, e.Expose(ExposeStringTest, "crystalline", "ExposeStringTest"))
	testza.AssertNoError(t, e.Expose(ExposeIntTest, "crystalline", "ExposeIntTest"))
	testza.AssertNoError(t, e.Expose(ExposeStructTest, "crystalline", "ExposeStructTest"))
	testza.AssertNoError(t, e.Expose(ExposePointerTest, "crystalline", "ExposePointerTest"))
	testza.AssertNoError(t, e.Expose(ExposeMapTest, "crystalline", "ExposeMapTest"))

	testza.AssertNoError(t, e.AddEntity(nil, "GlobalTest", reflect.TypeOf(GlobalTestObj{})))

	tsdFile, jsFile, err := e.Build()
	testza.AssertNoError(t, err)

	testza.AssertEqual(t, `export let GlobalTest;
export let crystalline;

export const initializeCrystalline = () => {
  GlobalTest = globalThis["go"]["app"]["GlobalTest"];
  crystalline = {
    ErrorFunc: globalThis["go"]["app"]["crystalline"]["ErrorFunc"],
    ExposeArrayTest: globalThis["go"]["app"]["crystalline"]["ExposeArrayTest"],
    ExposeIntTest: globalThis["go"]["app"]["crystalline"]["ExposeIntTest"],
    ExposeMapTest: globalThis["go"]["app"]["crystalline"]["ExposeMapTest"],
    ExposePointerTest: globalThis["go"]["app"]["crystalline"]["ExposePointerTest"],
    ExposeSliceTest: globalThis["go"]["app"]["crystalline"]["ExposeSliceTest"],
    ExposeStringTest: globalThis["go"]["app"]["crystalline"]["ExposeStringTest"],
    ExposeStructTest: globalThis["go"]["app"]["crystalline"]["ExposeStructTest"],
    InterfaceFunc: globalThis["go"]["app"]["crystalline"]["InterfaceFunc"],
    SomeFunc: globalThis["go"]["app"]["crystalline"]["SomeFunc"],
  }
}`, jsFile)

	testza.AssertEqual(t, `export const GlobalTest = crystalline.GlobalTestObj;
export declare namespace crystalline {
  interface GlobalTestObj {
  }
  interface SomeObj {
    Name: string;
    Data?: Record<number, number>;
    Nested: nested.AnotherObj;
    VoidFunc: () => void;
    NoPointer(first: string, second: number): void;
    WithPointer(first: number, second: boolean): void;
  }
  function ErrorFunc(): Error;
  const ExposeArrayTest: Array<string> | undefined;
  const ExposeIntTest: number;
  const ExposeMapTest: Record<number, number> | undefined;
  const ExposePointerTest: crystalline.SomeObj | undefined;
  const ExposeSliceTest: Array<number> | undefined;
  const ExposeStringTest: string;
  const ExposeStructTest: crystalline.SomeObj;
  function InterfaceFunc(): (unknown | undefined);
  function SomeFunc(name: string, a: boolean): [string, boolean];
}
export declare namespace nested {
  interface AnotherObj {
    SomeValue?: Array<number>;
  }
}
export const initializeCrystalline: () => void;`, tsdFile)
}
