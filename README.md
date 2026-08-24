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
import { boot } from "./dist/crystalline.js";

const { api } = await boot("main.wasm");

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
| `int64`, `uint64` | `number`, with a warning past 2^53 |
| `string` | `string` |
| `[]byte`, `[N]byte` | `Uint8Array` |
| `[]T`, `[N]T` | `Array<T>` |
| `[]T` with `crystalline:"not_nil"` | `Array<T>`, empty instead of null |
| `map[K]V` | `Record<K, V>` |
| `*T` | `T \| undefined` |
| `struct` | `interface`, live fields and methods |
| `struct` marked `r.Plain` | `interface`, read-only data, no methods |
| named int or string with constants | union type plus a constants object |
| `time.Time` | `Date` |
| `time.Duration` | `number` of milliseconds |
| type mapped with `r.Marshal` | whatever its functions carry |
| `func(...)` | function type |
| `...T` variadic | `Array<T>` |
| `error` as a value | `Error` |
| `(T, error)` return | `Result<T>` with `unwrap` and `unwrapOr` |
| `chan T`, `<-chan T` return | `AsyncIterable<T>` |
| `<-chan T` parameter | `AsyncIterable<T>` the caller supplies |
| `context.Context` first parameter | `AbortSignal` |
| `panic` | thrown or rejected `Error` with the Go stack |

A struct arrives as a *live view*: each field read and write is a call into Go,
and each field and method holds a slot in the Go/JS bridge until the wrapper is
released. `r.Plain(T{})` converts a type to ordinary JavaScript data instead —
once, with no methods and no writing back — which is what a result that is only
read wants. See [Performance](#performance).

A channel parameter accepts any iterable, including a plain array, and must say
its direction — `chan T` is refused rather than guessed at. Send-only channels
and complex numbers have no counterpart and are reported.

## Nothing is dropped silently

Anything that cannot be bound is named, with the reason:

```
crystalline: skipped api.Watch: type chan Event cannot be read from JS
```

## Performance

Everything crosses a bridge, and the bridge is the cost. Measured with
`./bench/run.sh` — the ratios travel, the absolute numbers do not.

* A call costs about 5.5 µs whether crystalline wrote the binding or you did.
  **Count crossings, not conversions**: 98 small calls spend half a millisecond
  crossing before doing any work, where one call returning the same data as an
  aggregate spends 5.5 µs. This is the one cost `r.Plain` cannot remove, because
  it is per crossing rather than per conversion.
* A wrapper field is a call, not a property: about 6.8 µs against 6 ns on plain
  data. Read it into a local rather than in a loop.
* Building a struct wrapper costs about 68 µs, so a slice of them is expensive.
  `r.Plain` makes the same result about 10x cheaper.
* Bulk data crosses about 3x faster as `[]byte` than as a string.

## Examples and benchmarks

Three worked examples, each a page backed by a real wasm binary:

```sh
mise run examples        # generate, build and check them under node
mise run examples:serve  # then open http://localhost:8000/01-hello/
```

`./bench/run.sh` measures what each kind of crossing costs, against a
hand-written `syscall/js` binding doing the same conversions.

Generating with `-profile` counts them in your own app, reported by `stats()`:

```
greeting.Greet: 98 calls, 0.54ms
```

Requires Go 1.26 or newer.
