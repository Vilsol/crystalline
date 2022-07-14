package crystalline

import (
	"errors"
	"reflect"
	"testing"

	"github.com/MarvinJWendt/testza"
)

func TestInt(t *testing.T) {
	result, err := Map(1)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, int64(1), result)

	result, err = Map(int8(2))
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, int64(2), result)

	result, err = Map(int16(3))
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, int64(3), result)

	result, err = Map(int32(4))
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, int64(4), result)

	result, err = Map(int64(5))
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, int64(5), result)
}

func TestUint(t *testing.T) {
	result, err := Map(uint(1))
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, uint64(1), result)

	result, err = Map(uint8(2))
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, uint64(2), result)

	result, err = Map(uint16(3))
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, uint64(3), result)

	result, err = Map(uint32(4))
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, uint64(4), result)

	result, err = Map(uint64(5))
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, uint64(5), result)

	result, err = Map(uintptr(6))
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, uint64(6), result)
}

func TestFloat(t *testing.T) {
	result, err := Map(float32(1))
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, float64(1), result)

	result, err = Map(float64(2))
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, float64(2), result)
}

func TestChannel(t *testing.T) {
	result, err := Map(make(chan bool))
	testza.AssertNil(t, result)
	testza.AssertEqual(t, errors.New("channels cannot be converted to wasm"), err)
}

func TestComplex(t *testing.T) {
	result, err := Map(complex64(1))
	testza.AssertNil(t, result)
	testza.AssertEqual(t, errors.New("complex64 cannot be converted to wasm"), err)

	result, err = Map(complex128(2))
	testza.AssertNil(t, result)
	testza.AssertEqual(t, errors.New("complex128 cannot be converted to wasm"), err)

	result, err = Map(complex(3, 4))
	testza.AssertNil(t, result)
	testza.AssertEqual(t, errors.New("complex128 cannot be converted to wasm"), err)
}

func TestString(t *testing.T) {
	result, err := Map("Hello World")
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, "Hello World", result)
}

func TestBoolean(t *testing.T) {
	result, err := Map(true)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, true, result)

	result, err = Map(false)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, false, result)
}

func TestArray(t *testing.T) {
	result, err := Map([4]int{1, 2, 3, 4})
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, []interface{}{int64(1), int64(2), int64(3), int64(4)}, result)
}

func TestSlice(t *testing.T) {
	result, err := Map([]int{1, 2, 3, 4})
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, []interface{}{int64(1), int64(2), int64(3), int64(4)}, result)

	var fakeSlice []int
	result, err = Map(fakeSlice)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, nil, result)
}

func TestMap(t *testing.T) {
	result, err := Map(map[float64]map[int]string{
		1.23: {
			1: "hello",
			2: "world",
		},
		4.56: {
			3: "foo",
			4: "bar",
		},
	})
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, map[string]interface{}{
		"1.23": map[string]interface{}{
			"1": "hello",
			"2": "world",
		},
		"4.56": map[string]interface{}{
			"3": "foo",
			"4": "bar",
		},
	}, result)

	var fakeMap map[string]int
	result, err = Map(fakeMap)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, nil, result)
}

type Greetable interface {
	Greet(string) string
}

type Sample struct {
	Greeting string
}

func (s Sample) Greet(name string) string {
	return s.Greeting + name
}

func TestStructs(t *testing.T) {
	result, err := Map(Sample{Greeting: "Hello, "})
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, "Hello, ", (result.(map[string]interface{}))["Greeting"])
}

func TestInterface(t *testing.T) {
	var greetable Greetable = Sample{Greeting: "Hello, "}
	result, err := Map(&greetable)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, "Hello, ", (result.(map[string]interface{}))["Greeting"])
}

func TestPointer(t *testing.T) {
	var fakeInterface *Greetable
	result, err := Map(fakeInterface)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, nil, result)
}

func TestUnsafePointer(t *testing.T) {
	var greetable Greetable = &Sample{Greeting: "Hello, "}
	result, err := mapInternal(reflect.ValueOf(reflect.ValueOf(greetable).UnsafePointer()))
	testza.AssertNoError(t, err)
	testza.AssertGreater(t, result, 0)

	var fakeInterface *Greetable
	result, err = mapInternal(reflect.ValueOf(reflect.ValueOf(fakeInterface).UnsafePointer()))
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, nil, result)
}

func TestNilFunc(t *testing.T) {
	var fakeFunc func()
	result, err := Map(fakeFunc)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, nil, result)
}

func TestPanic(t *testing.T) {
	testza.AssertPanics(t, func() {
		MapOrPanic(make(chan bool))
	})
}

type WrappedTypeString string

func TestWrappedType(t *testing.T) {
	result, err := Map(WrappedTypeString("hello"))
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, "hello", result)
}
