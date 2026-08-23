# crystalline

Expose Go code to JavaScript from a WebAssembly binary, with TypeScript declarations generated to match.

[![Go Reference](https://pkg.go.dev/badge/github.com/Vilsol/crystalline.svg)](https://pkg.go.dev/github.com/Vilsol/crystalline)

A Go program built with `GOOS=js GOARCH=wasm` can already reach JavaScript through `syscall/js`, but every binding is hand-written, lands in a flat global, and carries no type information. Crystalline takes your Go functions and values, publishes them under a namespaced object graph, and emits an ES module plus a `.d.ts` so the JavaScript side gets real autocompletion.

```go
e := crystalline.NewExposer("myapp")
e.MustExposeFunc(api.Greet)
```

```ts
import { api } from "./crystalline.js";

api.Greet("world");   // typed, autocompleted, throws a real Error on panic
```

## Install

```sh
go get github.com/Vilsol/crystalline
```

Requires Go 1.27 or newer.

## Generating bindings

Write a small program that registers what should be visible and writes the output. It has to be built for wasm, since exposing publishes into the JS global.

```go
//go:build js

package main

import (
	"log"

	"github.com/Vilsol/crystalline"

	"example.com/myapp/api"
)

func main() {
	e := crystalline.NewExposer("myapp")

	e.MustExposeFunc(api.Greet)                        // go.myapp.api.Greet
	e.MustExposeFunc(api.Load, crystalline.AsPromise()) // returns a JS Promise
	e.MustExposeValue("Version", api.Version)          // go.myapp.<caller pkg>.Version

	out, err := e.Build()
	if err != nil {
		log.Fatal(err)
	}

	if err := out.WriteFiles("dist/crystalline.js", "dist/crystalline.d.ts"); err != nil {
		log.Fatal(err)
	}

	select {} // keep the module alive so JS can call in
}
```

A function's name and package are recovered from the Go runtime, so there is nothing to keep in sync by hand. A value carries no name at runtime, so `ExposeValue` takes one — its namespace still defaults to the calling package.

Every `Expose*` method returns an error; the `Must*` variants panic instead, for use during initialisation.

## Using the bindings

The generated module exports one initializer, which must run **after** the wasm module has started:

```ts
import { initializeCrystalline, api } from "./crystalline.js";

const go = new Go();
const { instance } = await WebAssembly.instantiateStreaming(
  fetch("main.wasm"),
  go.importObject,
);
go.run(instance);        // publishes globalThis.go.myapp
initializeCrystalline(); // binds the exported namespaces

api.Greet("world");
```

Calling `initializeCrystalline()` too early throws with an explanation rather than a `TypeError`.

## What maps across

| Go | JavaScript | TypeScript |
| --- | --- | --- |
| `bool` | `boolean` | `boolean` |
| numeric types | `number` | `number` |
| `string` | `string` | `string` |
| `[]byte`, `[N]byte` | `Uint8Array` | `Uint8Array` |
| slice, array | `Array` | `Array<T>` |
| `map[K]V` | object | `Record<K, V>` |
| struct | object with getters, setters and methods | `interface` |
| pointer | the pointee, or `null` | `T \| undefined` |
| `error` | `Error` | `Error` |
| `func` | function | function type |

A pointer to a struct is live: assigning to a field from JavaScript writes through to the Go value. A struct passed by value is a copy.

Channels, complex numbers and unsafe pointers cannot cross the boundary, and interfaces cannot be used as parameters. These are reported when the entity is exposed — not when JavaScript first calls it — and the error names the path to the offending field:

```
exposing api.Config: Server.Done: channels cannot be converted to wasm
```

## Promises

A Go call blocks the single JS thread. Mark a function as a promise to run it on its own goroutine and hand JavaScript a `Promise` instead:

```go
e.MustExposeFunc(api.Load, crystalline.AsPromise())
```

Methods are marked with a comment on the declaration, which also flows into the generated types:

```go
// crystalline:promise
func (s Service) Load() Result { ... }
```

or programmatically, which checks that the method actually exists:

```go
err := crystalline.MarkPromise(reflect.TypeOf(Service{}), "Load")
```

Any function taking a callback is always a promise, whether or not it is marked — the callback cannot be serviced without yielding to the event loop.

## Errors and panics

A Go `error` return maps to a JS `Error`. A Go panic surfaces as an `Error` carrying the panic message and stack — thrown for a synchronous call, rejected for a promise:

```ts
try {
  api.Boom();
} catch (e) {
  console.error(e.message); // "Panic: something went wrong\ngoroutine ..."
}

await api.LoadThatPanics().catch((e) => e.message);
```

## Struct tags

A nil slice or map maps to `null`, which JS code expecting a collection usually does not want. The `not_nil` option emits an empty collection instead, and makes the field non-optional in the generated types:

```go
type Config struct {
	Hosts []string `crystalline:"not_nil"`
}
```

```ts
interface Config {
  Hosts: Array<string>;   // rather than Hosts?: Array<string>
}
```

Unrecognised options are rejected rather than ignored.

## Hiding methods

```go
err := crystalline.MarkIgnored(reflect.TypeOf(Service{}), "internalHelper")
```

Both the type and the method name are checked, so a rename or a typo is reported instead of quietly leaving the method exposed.

## Output style

The generated JavaScript can be matched to your formatter, per Exposer:

```go
e := crystalline.NewExposer("myapp",
	crystalline.WithQuoteStyle(`"`),
	crystalline.WithTrailingComma(),
)
```

## Development

```sh
mise install     # Go toolchain
./run_tests.sh   # builds for wasm and runs the suite under node
```

Tests run under `GOOS=js GOARCH=wasm` via node, since most of the library only exists in that build.

## License

See [LICENSE](LICENSE).
