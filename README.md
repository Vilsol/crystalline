# crystalline

Go to JavaScript bindings for WebAssembly, generated from source.

## Features

* Generates JS bindings and TypeScript declarations from Go source at build time.
* No `reflect` in the generated code — around 25% smaller wasm binaries.
* One compiler-checked manifest function declares the whole JS surface.
* Exposes symbols from third-party packages you cannot annotate.
* `//crystalline:export` directive as shorthand for your own code.
* Hide or promote individual methods per type.
* Promises via option, doc directive, or automatically for callbacks and contexts.
* Generic instantiations kept distinct.
* Configurable quote style and trailing commas.
* `go:generate` command and library API.

## Install

Pin the generator as a module tool, so everyone building the project gets the
same version without installing anything globally:

```sh
go get -tool github.com/Vilsol/crystalline/cmd/crystalline
```

That records it in `go.mod`:

```
tool github.com/Vilsol/crystalline/cmd/crystalline
```

## Usage

Declare the surface. The manifest runs, so values can be built however you like —
only their types are read at generate time.

```go
//go:generate go tool crystalline -app myapp -out ./dist ./...

//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(api.Greet)
	r.Func(api.Load, bind.AsPromise())
	r.Ignore(vendor.Client{}, "internalHelper")

	index := make(map[uint32]*api.Node)
	for _, node := range api.Nodes {
		index[node.ID] = node
	}

	r.Value("NodesByID", index, bind.InNamespace("api"))
}
```

Consume it, once the module is running:

```ts
import { initializeCrystalline, api } from "./dist/crystalline.js";

go.run(instance);
initializeCrystalline();

const greeting = api.Greet("world").unwrap();
```

## Structs

Wrappers are live, keep their identity across the boundary, and can be released
deterministically.

```ts
using config = api.LoadConfig();  // released at scope exit

config.Timeout = 30;              // writes through to the Go value
config.Validate();                // methods are bound alongside

api.Apply(config);                // Go receives the original, not a copy
api.Apply({ Timeout: 30 });       // literals work too, validated field by field
```

## Type mapping

| Go | TypeScript |
| --- | --- |
| `bool` | `boolean` |
| `int`, `uint`, `float`, `uintptr` | `number` |
| `string` | `string` |
| `[]byte`, `[N]byte` | `Uint8Array` |
| `[]T`, `[N]T` | `Array<T>` |
| `[]T` with `crystalline:"not_nil"` | `Array<T>`, empty instead of null |
| `map[K]V` | `Record<K, V>` |
| `*T` | `T \| undefined` |
| `struct` | `interface`, live fields and methods |
| `func(...)` | function type |
| `error` as a value | `Error` |
| `(T, error)` return | `Result<T>` with `unwrap` and `unwrapOr` |
| `chan T`, `<-chan T` return | `AsyncIterable<T>` |
| `<-chan T` parameter | `AsyncIterable<T>` the caller supplies |
| `context.Context` first parameter | `AbortSignal` |
| `panic` | thrown or rejected `Error` with the Go stack |

A channel parameter accepts any iterable, including a plain array, and must say
its direction — `chan T` is refused rather than guessed at. Send-only channels
and complex numbers have no counterpart and are reported.

## Nothing is dropped silently

Anything that cannot be bound is named, with the reason:

```
crystalline: skipped api.Watch: type chan Event cannot be read from JS
```

## Examples

Three worked examples, each a page backed by a real wasm binary:

```sh
mise run examples        # generate, build and check them under node
mise run examples:serve  # then open http://localhost:8000/01-hello/
```

Requires Go 1.26 or newer.
