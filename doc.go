// Package crystalline exposes Go code to JavaScript from a WebAssembly binary,
// and generates the TypeScript declarations to go with it.
//
// A Go program compiled with GOOS=js GOARCH=wasm can already reach JavaScript
// through syscall/js, but every binding has to be written by hand, lands in a
// flat global, and carries no type information. Crystalline takes Go functions
// and values, publishes them under a namespaced object graph, and emits an ES
// module plus a .d.ts so the JavaScript side gets real autocompletion.
//
// # Generating bindings
//
// Build an Exposer, register what should be visible, and write the result:
//
//	e := crystalline.NewExposer("myapp")
//
//	e.MustExposeFunc(api.Greet)                   // go.myapp.api.Greet
//	e.MustExposeFunc(api.Load, crystalline.AsPromise())
//	e.MustExposeValue("Version", api.Version)     // go.myapp.<caller pkg>.Version
//
//	out, err := e.Build()
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	if err := out.WriteFiles("dist/crystalline.js", "dist/crystalline.d.ts"); err != nil {
//		log.Fatal(err)
//	}
//
// A function's name and package are recovered from the Go runtime, so there is
// nothing to keep in sync by hand. A value carries no name at runtime, so
// ExposeValue takes one; its namespace still defaults to the calling package.
//
// # Using the bindings
//
// The generated module exports one initializer, which has to run after the
// wasm module has started:
//
//	import { initializeCrystalline, api } from "./crystalline.js";
//
//	const go = new Go();
//	const { instance } = await WebAssembly.instantiateStreaming(fetch("main.wasm"), go.importObject);
//	go.run(instance);          // publishes globalThis.go.myapp
//	initializeCrystalline();   // binds the exported namespaces
//
//	api.Greet("world");
//
// Calling initializeCrystalline before the module is running throws with an
// explanation rather than a TypeError.
//
// # Promises
//
// A Go call blocks the single JS thread. Marking a function as a promise runs
// it on its own goroutine and hands JavaScript a Promise instead:
//
//	e.MustExposeFunc(api.Load, crystalline.AsPromise())
//
// Methods can be marked with a comment on the declaration, which also flows
// into the generated types:
//
//	// crystalline:promise
//	func (s Service) Load() Result { ... }
//
// or programmatically, which validates that the method exists:
//
//	err := crystalline.MarkPromise(reflect.TypeOf(Service{}), "Load")
//
// Any function taking a callback is always a promise, whether or not it is
// marked: the callback cannot be serviced without yielding to the event loop.
//
// # Errors
//
// A Go panic surfaces in JavaScript as an Error carrying the panic message and
// stack: thrown for a synchronous call, and as a rejection for a promise.
//
// A Go error return maps to a JS Error object.
//
// # Struct tags
//
// A nil slice or map maps to null, which JS code that expects a collection
// usually does not want. The not_nil option emits an empty collection instead,
// and makes the field non optional in the generated types:
//
//	type Config struct {
//		Hosts []string `crystalline:"not_nil"`
//	}
//
// Unrecognised options are rejected rather than ignored.
//
// # Unsupported types
//
// Channels, complex numbers and unsafe pointers cannot cross the boundary, and
// interfaces cannot be used as parameters. These are reported when the entity
// is exposed, not when JavaScript first calls it, and the error names the path
// to the offending field.
package crystalline
