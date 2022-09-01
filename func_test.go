//go:build js

package crystalline

import (
	"syscall/js"
	"testing"
	"unsafe"

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

// crystalline:promise
func (s FnSample) C(x bool) bool {
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

	noPtrPromise := Run([]interface{}{"TestStructFn", "C"}, true)
	noPtrResults := testResolvePromise(noPtrPromise)
	testza.AssertTrue(t, noPtrResults.Bool())

	js.Global().Set("TestStructFnPtr", MapOrPanic(&FnSample{}))
	testza.AssertTrue(t, Run([]interface{}{"TestStructFnPtr", "A"}, true).Bool())
	testza.AssertTrue(t, Run([]interface{}{"TestStructFnPtr", "B"}, true).Bool())

	ptrPromise := Run([]interface{}{"TestStructFnPtr", "C"}, true)
	ptrResults := testResolvePromise(ptrPromise)
	testza.AssertTrue(t, ptrResults.Bool())
}

func TestFnFunc(t *testing.T) {
	type FnSampleFunc func(string) (bool, string)

	js.Global().Set("TestFunc", MapOrPanic(func(a FnSampleFunc) (bool, string) {
		return a("hello")
	}))

	result := Run([]interface{}{"TestFunc"}, MapOrPanic(func(data string) (bool, string) {
		return data == "hello", "bob"
	}))

	testFuncArgs := testResolvePromise(result)
	testza.AssertTrue(t, testFuncArgs.Index(0).Bool())
	testza.AssertEqual(t, "bob", testFuncArgs.Index(1).String())

	type FnSampleFuncOne func(string) bool
	js.Global().Set("TestFuncOneReturn", MapOrPanic(func(a FnSampleFuncOne) bool {
		return a("hello")
	}))

	result = Run([]interface{}{"TestFuncOneReturn"}, MapOrPanic(func(data string) bool {
		return data == "hello"
	}))

	testFuncOneReturnArgs := testResolvePromise(result)
	testza.AssertTrue(t, testFuncOneReturnArgs.Bool())

	js.Global().Set("TestFuncNoReturn", MapOrPanic(func(a FnSampleFunc) {
		x, y := a("hello")
		testza.AssertTrue(t, x)
		testza.AssertEqual(t, "bob", y)
	}))

	Run([]interface{}{"TestFuncNoReturn"}, MapOrPanic(func(data string) (bool, string) {
		return data == "hello", "bob"
	}))
}

type FnInterface interface {
}

func TestUnsupported(t *testing.T) {
	// Should fail on channels
	testza.AssertPanics(t, func() {
		fn, _ := Map(func(chan bool) {})
		fn.(js.Value).Invoke()
	})

	// Should fail on complex types
	testza.AssertPanics(t, func() {
		fn, _ := Map(func(complex64) {})
		fn.(js.Value).Invoke()
	})
	testza.AssertPanics(t, func() {
		fn, _ := Map(func(complex128) {})
		fn.(js.Value).Invoke()
	})

	// Should fail on unsafe pointers
	testza.AssertPanics(t, func() {
		fn, _ := Map(func(unsafe.Pointer) {})
		fn.(js.Value).Invoke()
	})

	// Should fail on interfaces
	testza.AssertPanics(t, func() {
		fn, _ := Map(func(FnInterface) {})
		fn.(js.Value).Invoke()
	})
}

type (
	WrappedTypeInt     int
	WrappedTypeInt8    int8
	WrappedTypeInt16   int16
	WrappedTypeInt32   int32
	WrappedTypeInt64   int64
	WrappedTypeUint    uint
	WrappedTypeUint8   uint8
	WrappedTypeUint16  uint16
	WrappedTypeUint32  uint32
	WrappedTypeUint64  uint64
	WrappedTypeUintPTR uintptr
	WrappedTypeFloat32 float32
	WrappedTypeFloat64 float64
	WrappedTypeBool    bool
	WrappedTypeArray   [1]string
	WrappedTypeFunc    func() string
	WrappedTypeMap     map[int]string
	WrappedTypePointer *string
	WrappedTypeSlice   []string
	WrappedTypeStruct  Sample
)

func TestFnWrappedType(t *testing.T) {
	js.Global().Set("TestWrappedTypeString", MapOrPanic(func(a WrappedTypeString) bool { return a == "hello" }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeString"}, "hello").Bool())

	js.Global().Set("TestWrappedTypeInt", MapOrPanic(func(a WrappedTypeInt) bool { return a == 10 }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeInt"}, 10).Bool())

	js.Global().Set("TestWrappedTypeInt8", MapOrPanic(func(a WrappedTypeInt8) bool { return a == 10 }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeInt8"}, 10).Bool())

	js.Global().Set("TestWrappedTypeInt16", MapOrPanic(func(a WrappedTypeInt16) bool { return a == 10 }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeInt16"}, 10).Bool())

	js.Global().Set("TestWrappedTypeInt32", MapOrPanic(func(a WrappedTypeInt32) bool { return a == 10 }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeInt32"}, 10).Bool())

	js.Global().Set("TestWrappedTypeInt64", MapOrPanic(func(a WrappedTypeInt64) bool { return a == 10 }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeInt64"}, 10).Bool())

	js.Global().Set("TestWrappedTypeUint", MapOrPanic(func(a WrappedTypeUint) bool { return a == 10 }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeUint"}, 10).Bool())

	js.Global().Set("TestWrappedTypeUint8", MapOrPanic(func(a WrappedTypeUint8) bool { return a == 10 }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeUint8"}, 10).Bool())

	js.Global().Set("TestWrappedTypeUint16", MapOrPanic(func(a WrappedTypeUint16) bool { return a == 10 }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeUint16"}, 10).Bool())

	js.Global().Set("TestWrappedTypeUint32", MapOrPanic(func(a WrappedTypeUint32) bool { return a == 10 }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeUint32"}, 10).Bool())

	js.Global().Set("TestWrappedTypeUint64", MapOrPanic(func(a WrappedTypeUint64) bool { return a == 10 }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeUint64"}, 10).Bool())

	js.Global().Set("TestWrappedTypeUintPTR", MapOrPanic(func(a WrappedTypeUintPTR) bool { return a == 10 }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeUintPTR"}, 10).Bool())

	js.Global().Set("TestWrappedTypeFloat32", MapOrPanic(func(a WrappedTypeFloat32) bool { return a == 10 }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeFloat32"}, 10).Bool())

	js.Global().Set("TestWrappedTypeFloat64", MapOrPanic(func(a WrappedTypeFloat64) bool { return a == 10 }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeFloat64"}, 10).Bool())

	js.Global().Set("TestWrappedTypeBool", MapOrPanic(func(a WrappedTypeBool) bool { return bool(a) }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeBool"}, true).Bool())

	js.Global().Set("TestWrappedTypeArray", MapOrPanic(func(a WrappedTypeArray) bool { return a[0] == "hello" }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeArray"}, MapOrPanic([]string{"hello"})).Bool())

	js.Global().Set("TestWrappedTypeFunc", MapOrPanic(func(a WrappedTypeFunc) bool { return a() == "hello" }))
	TestWrappedTypeFuncResult := Run([]interface{}{"TestWrappedTypeFunc"}, MapOrPanic(func() string { return "hello" }))
	TestWrappedTypeFuncArgs := testResolvePromise(TestWrappedTypeFuncResult)
	testza.AssertTrue(t, TestWrappedTypeFuncArgs.Bool())

	js.Global().Set("TestWrappedTypeMap", MapOrPanic(func(a WrappedTypeMap) bool { return a[0] == "hello" }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeMap"}, MapOrPanic(map[int]string{0: "hello"})).Bool())

	js.Global().Set("TestWrappedTypePointer", MapOrPanic(func(a WrappedTypePointer) bool { return *a == "hello" }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypePointer"}, "hello").Bool())

	js.Global().Set("TestWrappedTypeSlice", MapOrPanic(func(a WrappedTypeSlice) bool { return a[0] == "hello" }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeSlice"}, MapOrPanic([]string{"hello"})).Bool())

	js.Global().Set("TestWrappedTypeStruct", MapOrPanic(func(a WrappedTypeStruct) bool { return a.Greeting == "hello" }))
	testza.AssertTrue(t, Run([]interface{}{"TestWrappedTypeStruct"}, MapOrPanic(Sample{Greeting: "hello"})).Bool())
}

func TestFnPointers(t *testing.T) {
	js.Global().Set("TestPointersValue", MapOrPanic(func(a *string) bool {
		return *a == "hello"
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestPointersValue"}, "hello").Bool())

	js.Global().Set("TestPointersNil", MapOrPanic(func(a *string) bool {
		return a == nil
	}))
	testza.AssertTrue(t, Run([]interface{}{"TestPointersNil"}, nil).Bool())
}
