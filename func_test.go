//go:build js

package crystalline

import (
	"syscall/js"
	"testing"

	"github.com/MarvinJWendt/testza"
)

var invoker js.Value

func init() {
	js.Global().Get("eval").Invoke(`global.Invoker = (fn, ...args) => {
	console.log('invoking', fn, 'with', args);
	let obj = global;
	fn.forEach(k => obj = obj[k]);
	return obj(...args);
}`)

	invoker = js.Global().Get("Invoker")
}

func Run(method []interface{}, args ...interface{}) js.Value {
	return invoker.Invoke(append([]interface{}{method}, args...)...)
}

func TestFnBool(t *testing.T) {
	js.Global().Set("TestBool", MapOrPanic(func(a bool) bool {
		return a
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestBool"}, true).Bool())
}

func TestFnInt(t *testing.T) {
	js.Global().Set("TestInt", MapOrPanic(func(a int) bool {
		return a == 10
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestInt"}, 10).Bool())
	testza.AssertTrue(t, Run([]interface{}{"TestInt"}, "10").Bool())

	js.Global().Set("TestInt8", MapOrPanic(func(a int8) bool {
		return a == 10
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestInt8"}, 10).Bool())
	testza.AssertTrue(t, Run([]interface{}{"TestInt8"}, "10").Bool())

	js.Global().Set("TestInt16", MapOrPanic(func(a int16) bool {
		return a == 10
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestInt16"}, 10).Bool())
	testza.AssertTrue(t, Run([]interface{}{"TestInt16"}, "10").Bool())

	js.Global().Set("TestInt32", MapOrPanic(func(a int32) bool {
		return a == 10
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestInt32"}, 10).Bool())
	testza.AssertTrue(t, Run([]interface{}{"TestInt32"}, "10").Bool())

	js.Global().Set("TestInt64", MapOrPanic(func(a int64) bool {
		return a == 10
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestInt64"}, 10).Bool())
	testza.AssertTrue(t, Run([]interface{}{"TestInt64"}, "10").Bool())
}

func TestFnUint(t *testing.T) {
	js.Global().Set("TestUint", MapOrPanic(func(a uint) bool {
		return a == 10
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestUint"}, 10).Bool())
	testza.AssertTrue(t, Run([]interface{}{"TestUint"}, "10").Bool())

	js.Global().Set("TestUint8", MapOrPanic(func(a uint8) bool {
		return a == 10
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestUint8"}, 10).Bool())
	testza.AssertTrue(t, Run([]interface{}{"TestUint8"}, "10").Bool())

	js.Global().Set("TestUint16", MapOrPanic(func(a uint16) bool {
		return a == 10
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestUint16"}, 10).Bool())
	testza.AssertTrue(t, Run([]interface{}{"TestUint16"}, "10").Bool())

	js.Global().Set("TestUint32", MapOrPanic(func(a uint32) bool {
		return a == 10
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestUint32"}, 10).Bool())
	testza.AssertTrue(t, Run([]interface{}{"TestUint32"}, "10").Bool())

	js.Global().Set("TestUint64", MapOrPanic(func(a uint64) bool {
		return a == 10
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestUint64"}, 10).Bool())
	testza.AssertTrue(t, Run([]interface{}{"TestUint64"}, "10").Bool())

	js.Global().Set("TestUintPTR", MapOrPanic(func(a uintptr) bool {
		return a == 10
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestUintPTR"}, 10).Bool())
	testza.AssertTrue(t, Run([]interface{}{"TestUintPTR"}, "10").Bool())
}

func TestFnFloat(t *testing.T) {
	js.Global().Set("TestFloat32", MapOrPanic(func(a float32) bool {
		return a == 10
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestFloat32"}, 10).Bool())
	testza.AssertTrue(t, Run([]interface{}{"TestFloat32"}, "10").Bool())

	js.Global().Set("TestFloat64", MapOrPanic(func(a float64) bool {
		return a == 10
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestFloat64"}, 10).Bool())
	testza.AssertTrue(t, Run([]interface{}{"TestFloat64"}, "10").Bool())
}

func TestFnSlice(t *testing.T) {
	js.Global().Set("TestSlice", MapOrPanic(func(a []int) bool {
		return a[0] == 10 && a[1] == 20 && a[2] == 30 && a[3] == 40
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestSlice"}, []interface{}{10, 20, 30, 40}).Bool())
}

func TestFnArray(t *testing.T) {
	js.Global().Set("TestArray", MapOrPanic(func(a [4]int) bool {
		return a[0] == 10 && a[1] == 20 && a[2] == 30 && a[3] == 40
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestArray"}, []interface{}{10, 20, 30, 40}).Bool())
}

func TestFnMap(t *testing.T) {
	js.Global().Set("TestMap", MapOrPanic(func(a map[int]string) bool {
		return a[0] == "hello" && a[1] == "world"
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestMap"}, map[string]interface{}{
		"0": "hello",
		"1": "world",
	}).Bool())
}

func TestFnString(t *testing.T) {
	js.Global().Set("TestString", MapOrPanic(func(a string) bool {
		return a == "hello"
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestString"}, "hello").Bool())
}

type FnSample struct {
	FirstValue  string
	SecondValue int
	ThirdValue  float32
}

func (s FnSample) A(x bool) bool {
	return x
}

func (s *FnSample) B(x bool) bool {
	return x
}

func TestFnStruct(t *testing.T) {
	js.Global().Set("TestStruct", MapOrPanic(func(a FnSample) bool {
		return a.FirstValue == "hello" && a.SecondValue == 1 && a.ThirdValue == 2.345
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestStruct"}, MapOrPanic(FnSample{
		FirstValue:  "hello",
		SecondValue: 1,
		ThirdValue:  2.345,
	})).Bool())

	testza.AssertTrue(t, Run([]interface{}{"TestStruct"}, MapOrPanic(&FnSample{
		FirstValue:  "hello",
		SecondValue: 1,
		ThirdValue:  2.345,
	})).Bool())

	js.Global().Set("TestStructFn", MapOrPanic(FnSample{}))
	testza.AssertTrue(t, Run([]interface{}{"TestStructFn", "A"}, true).Bool())

	js.Global().Set("TestStructFnPtr", MapOrPanic(&FnSample{}))
	testza.AssertTrue(t, Run([]interface{}{"TestStructFnPtr", "A"}, true).Bool())
	testza.AssertTrue(t, Run([]interface{}{"TestStructFnPtr", "B"}, true).Bool())
}

type FnSampleFunc func(string) (bool, string)

func TestFnFunc(t *testing.T) {
	js.Global().Set("TestFunc", MapOrPanic(func(a FnSampleFunc) (bool, string) {
		return a("hello")
	}))

	result := Run([]interface{}{"TestFunc"}, MapOrPanic(func(data string) (bool, string) {
		return data == "hello", "bob"
	}))

	testza.AssertTrue(t, result.Index(0).Bool())
	testza.AssertEqual(t, "bob", result.Index(1).String())
}
