# crystalline

Go to JavaScript bindings for WebAssembly, generated from source.

## Features

* Generates JS bindings and TypeScript declarations from Go source at build time.
* No `reflect` in the generated code — 23% smaller wasm on a real project.
* One compiler-checked manifest declares the whole surface, including symbols
  from packages you do not own. `//crystalline:export` is the shorthand for ones
  you do.
* Structs arrive as live wrappers, or as plain data with `bind.Plain()` — 9x
  cheaper for a result that is only read.
* `bind.MarshalledBy` maps a type onto a JavaScript counterpart with two Go
  functions. `time.Time` and `time.Duration` are mapped as standard.
* Enums keep their names: a union type plus a constants object.
* Interfaces go the other way — Go declares what it needs, JavaScript supplies
  an object with those methods.
* Promises by option, by doc directive, or automatically for callbacks, contexts
  and channel parameters.
* Generic instantiations stay distinct, embedded methods are promoted, and two
  packages may each declare a `Config`.
* The generated module starts itself: `const { api } = await boot("app.wasm")`.
* Skips and warnings carry `file:line`, so editors and CI annotate them.
* `-watch` regenerates on save, `-profile` counts crossings, and the output
  files, quote style, trailing commas and banner are all configurable.
* Builds under TinyGo too, at about a fifth of the size — with one caveat about
  panics.

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
	r.Type(vendor.Client{}, bind.Without("internalHelper"))

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

## Modelling

Go's own way of describing a domain crosses as JavaScript's.

```go
type Status int                     // an enum: a union type plus a constants
const (                             // object, so a caller names the value
	StatusDraft Status = iota
	StatusPublished
)

type Money struct{ Cents int }      // meaning is its text, not its field
func MoneyToText(Money) string
func MoneyFromText(string) (Money, error)

type Item struct {
	Audited                     // embedded: Listed() is promoted onto Item
	Price  Money
	Status Status
}

type Notifier interface {           // supplied from JavaScript
	Notify(message string)
}
```

One line in the manifest maps the type; everything else follows from the Go.

```go
r.Type(api.Money{}, bind.MarshalledBy(api.MoneyToText, api.MoneyFromText))
```

```ts
item.Price = "20.00";                 // refused if it cannot be read
item.Status = api.Status.StatusDraft; // named, not numbered
item.Listed();                        // promoted, as in Go

await api.Restock({ Notify: (m) => log(m) }, names);
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
| `struct` marked `bind.Plain()` | `interface`, read-only data, no methods |
| named int or string with constants | union type plus a constants object |
| `time.Time` | `Date` |
| `time.Duration` | `number` of milliseconds |
| type mapped with `bind.MarshalledBy` | whatever its functions carry |
| `func(...)` | function type |
| `...T` variadic | `Array<T>` |
| `error` as a value | `Error` |
| `(T, error)` return | `Result<T>`: `unwrap`, `unwrapOr`, or narrow on `ok` |
| `chan T`, `<-chan T` return | `AsyncIterable<T>` |
| `<-chan T` parameter | `AsyncIterable<T>` the caller supplies |
| `context.Context` first parameter | `AbortSignal` |
| `interface` parameter | object JavaScript supplies, with those methods |
| `panic` | thrown or rejected `Error` with the Go stack |

A struct arrives as a *live view*: each field read and write is a call into Go,
and each field and method holds a slot in the Go/JS bridge until the wrapper is
released. `r.Type(T{}, bind.Plain())` converts a type to ordinary JavaScript
data instead —
once, with no methods and no writing back — which is what a result that is only
read wants. See [Performance](#performance).

A channel parameter accepts any iterable, including a plain array, and must say
its direction — `chan T` is refused rather than guessed at. Send-only channels
and complex numbers have no counterpart and are reported.

## Nothing is dropped silently

Anything that cannot be bound is named, with the reason:

```
api/watch.go:31:1: api.Watch: type chan Event cannot be read from JS
```

Anything bound at a cost is named too, rather than left to be discovered:

```
api/report.go:12:1: api.Totals: int64 is bound as a JavaScript number, which
cannot represent values beyond 2^53 exactly
```

## Performance

Everything crosses a bridge, and the bridge is the cost. Measured with
`./bench/run.sh` — the ratios travel, the absolute numbers do not.

* A call costs about 5.5 µs whether crystalline wrote the binding or you did.
  **Count crossings, not conversions**: 98 small calls spend half a millisecond
  crossing before doing any work, where one call returning the same data as an
  aggregate spends 5.5 µs. This is the one cost `bind.Plain()` cannot remove,
  because
  it is per crossing rather than per conversion.
* A wrapper field is a call, not a property: about 6.8 µs against 6 ns on plain
  data. Read it into a local rather than in a loop.
* Building a struct wrapper costs about 68 µs, so a slice of them is expensive.
  `bind.Plain()` makes the same result about 9x cheaper.
* Bulk data crosses about 3x faster as `[]byte` than as a string.

## TinyGo

Every example builds and passes under TinyGo 0.41 with `-scheduler=asyncify`, at
about a fifth of the size:

```sh
./examples/tinygo.sh 04-catalogue
```

| example | tinygo | go |
| --- | --- | --- |
| 01-hello | 430K | 2.3M |
| 04-catalogue | 661K | 2.5M |

The generated test surface — 82 checks covering every kind of crossing, not just
what the examples reach — produces identical results under both toolchains.

One difference is not cosmetic. **TinyGo's wasm target implements no `recover`**,
so a panic aborts the module rather than arriving in JavaScript as an `Error`.
Generated code does not panic to report a failure — a mistyped argument, an
unknown property on an object literal, a field that cannot be written, a value a
channel could not carry, a source that rejects mid-stream are all reported and
returned — but two things still raise one:

* a panic in your own Go code;
* a JavaScript callback, or a method of an object supplied for an interface,
  that returns the wrong type or rejects.

The second converts inside a Go function whose signature is yours, so there is
nowhere to report to and nothing to return but a guess. Under the standard
toolchain both become a thrown or rejected `Error` carrying the Go stack.

## Examples and benchmarks

Four worked examples, each a page backed by a real wasm binary: the smallest
thing that works, structs and `Result`, channels and cancellation, and a
domain modelled with enums, mapped types and a supplied interface.

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

Requires Go 1.26 or newer. TinyGo 0.41 works as well; see [TinyGo](#tinygo).
