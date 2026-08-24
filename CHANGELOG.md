# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### 0.1.0

Crystalline is now a build-time generator rather than a reflection runtime. It
reads packages with `go/packages` and emits the bindings, so mistakes surface
when you build instead of when a user first calls in, and the wasm binary no
longer links `reflect`.

This release replaces the entire public API. See [Migrating](#migrating-from-0015).

#### Added

- `crystalline` command, installable as a module tool with `go get -tool` and
  runnable from `go:generate`.
- `bind` package: a reflect-free `Registry` that manifests are written against.
- Manifest functions marked `//crystalline:exports`, declaring the whole JS
  surface in one compiler-checked place, including symbols from packages you do
  not own.
- `//crystalline:export` directive as shorthand for declarations you own.
- `Result<T>` for fallible calls. Reaching the value needs no narrowing, since
  `unwrap` and `unwrapOr` are there whichever half you hold, and checking `ok`
  narrows for code that would rather branch. Methods live on a shared
  prototype, so results cost nothing per call.
- `AsyncIterable<T>` for channels in both directions. A channel parameter
  accepts any iterable, including a plain array, and is drained on a goroutine
  scoped to the call.
- `AbortSignal` for a leading `context.Context`, so long Go work can be
  cancelled from JavaScript.
- Deterministic release of struct wrappers through `release()` and
  `Symbol.dispose`, plus automatic release when JavaScript collects them.
- Round-trip identity: a wrapper handed back to Go resolves to the value it came
  from rather than a copy. Object literals are accepted in the same position.
- Validation of objects arriving from JavaScript against the exact field set,
  rejecting unknown properties and mistyped values.
- `Skipped` reporting: anything that cannot be bound is named with its reason.
- Golden-file tests for the emitted declarations and module.
- Four worked examples under `examples`, each a page backed by a real wasm
  binary, built and run under node by CI.
- Boundary benchmarks under `bench`, measuring each kind of crossing against
  a hand-written `syscall/js` binding doing the same conversions.
- `-watch` regenerates when a Go file changes. A failure reports where and
  keeps watching, so a half-finished edit does not end the session.
- An interface parameter is supplied from JavaScript: Go declares the methods
  it needs and the app hands in an object providing them, checked on arrival.
  This is the narrow form of calling out, reusing what a callback parameter
  already does one method at a time.
- `-profile` and `WithProfiling` count and time every call made through the
  module, reported by `stats()`. The advice to count crossings rather than
  conversions had nothing to count them with. Off by default.
- The generated module exports `boot`, which loads the binary, runs the Go
  program without awaiting it, initialises the bindings and returns the
  namespaces. It takes a URL or the bytes, so a bundler that inlines the wasm
  and a test that reads it from disk both work. Every project was writing
  those steps by hand, one of which hangs forever if awaited.
- A named integer or string type with constants is declared as the union of
  its values, and its constants are bound beside it, so a caller can name a
  value rather than write the number behind it. The whole enum used to
  collapse into `number`.
- `r.Marshal(to, from)` maps a type onto a JavaScript counterpart with a pair
  of ordinary Go functions. The signatures carry the declaration and are
  checked when generating. `time.Time` and `time.Duration` are mapped this
  way as standard, to a `Date` and to milliseconds; `time.Time` previously
  bound as a wrapper with thirty methods and no readable data, and silently
  read a real `Date` as the zero time.
- `r.Plain(T{})` marshals a type as ordinary JavaScript data instead of a
  live wrapper: converted once, no methods, no bridge slots and nothing to
  release. Measured at 9x cheaper for a slice of 32 structs. The mark reaches
  every struct the type contains, since a plain value cannot hold a live one.
- `-js-out`, `-ts-out` and `-banner` on the command, and `WithBanner` on the
  generator, so a project can name its own output files and put the pragmas
  its linters expect at the top of them.
- `Output.Skipped`, so a caller that only builds the declarations still sees
  what could not be bound. The report was reachable from `BuildGo` alone.
- Skips and warnings carry the position of what they name, rendered the way a
  compiler does and relative to the working directory, so an editor or CI can
  turn one into an annotation. The position is where the symbol is declared
  rather than where the manifest mentioned it.
- `Warnings` on both builders, for what was bound at a cost rather than not
  bound at all. The first is 64-bit integers: `int64` and `uint64` stay
  JavaScript numbers, since refusing them would break ordinary Go, and every
  member carrying one is named at generate time.
- Two packages may each declare a type of the same name. Generated identifiers
  were built from the bare Go name, so `api.Config` and `db.Config` produced
  one marshaller called with both. `format.Source` only parses, so nothing
  noticed: the generator reported no error and wrote a file that failed to
  compile in the consumer's own build. Type identity now has one owner.
- A complex parameter is refused by the bindings as well as the declarations.
  `types.IsNumeric` includes the complex kinds, so the converter accepted one
  and emitted `complex128(value.Float())` while the declarations refused it,
  and a package containing one generated nothing at all.
- A callback parameter accepts a plain value as well as a promise. Go awaits
  whatever comes back, so `(v) => v.length` always worked and the declaration
  rejected it, forcing an `async` that did nothing.
- The generated registry embeds `bind.Registry`, so a method added to that
  interface later does not stop an already-committed `crystalline_gen.go`
  compiling before it has been regenerated.
- An enum declared in a package reached only through another package's
  signature is still an enum. Which packages to load was decided with the
  same walk that decides what to declare, and that walk asks whether a type
  has constants, which cannot be answered before the package holding them is
  loaded. The type quietly became a number.
- A pointer to a mapped type honours the mapping. `*time.Time` reached for the
  struct marshaller before asking, so it produced the thirty-method wrapper
  that mapping `time.Time` exists to avoid, while the declarations said
  `Date`. Dereferencing is also parenthesised now, since the element's own
  conversion may call a method on it.
- A mapped type used as a map key runs its mapping. Keys read through to the
  underlying basic type, so the same `time.Duration` was milliseconds as a
  value and nanoseconds as a key, both declared `number`. A mapping that
  cannot become a property name, such as one to a `Date`, is refused.

#### Changed

- Generated bindings import only `syscall/js` and the standard library, keeping
  `reflect` out of the binary. Measured on a real project: 8.35 MB to 6.45 MB.
- A trailing `error` no longer becomes a tuple. It becomes a `Result<T>`, and
  the call stays synchronous unless something else makes it asynchronous, so a
  fallible call does not force its callers to be `async`.
- Panics are now distinct from errors: they arrive as a thrown or rejected
  `Error` carrying the Go stack, while an `error` is carried by the value.
- Generic instantiations keep distinct names, so `Pair[string]` and `Pair[int]`
  no longer collide.
- Types are declared in the namespace of the package that defines them, however
  they were reached.
- Unsupported types, unknown `crystalline` tag options and namespace collisions
  are reported when generating rather than at run time.
- Method marks are checked against the real method set, so a rename fails the
  build instead of silently changing the surface.
- A value exposed with `r.Value` defaults to the namespace of its own type's
  package rather than the manifest's. Exposing `data.Nodes` from a manifest in
  package `exposition` used to land it in `exposition`, so every call needed
  an explicit `bind.InNamespace`.
- That inference now looks through a map's key as well as its element, so
  `map[data.Kind]string` lands in `data` like `map[uint32]*data.Node` does,
  rather than falling back to the manifest's package.
- A binding captured before `initializeCrystalline()` ran says that it is a
  stale copy, instead of repeating advice the caller has already followed.
  Destructuring an export snapshots it, so the local keeps pointing at the
  placeholder however many times the real binding is reassigned.
- A call that returns a channel is no longer wrapped in a promise just for
  taking a context. The stream is already asynchronous, so the wrapper only
  made callers write `for await (const x of await f())`. Whether a call is a
  promise is now decided in one place shared by both renderers.
- `Result` is declared only when something on the surface can fail. A surface
  with nothing fallible used to carry a type a consumer could name but never
  receive.
- Bindings generated into the package they bind now compile. The qualifier is
  empty for the generated file's own package and was joined with a dot
  regardless, emitting `.Owned()`, so `//crystalline:export` had never worked
  in the layout it documents.
- Something that cannot be bound is now skipped and reported by both
  builders. They disagreed: `BuildGo` skipped, while `Build` failed outright
  for a channel and silently declared an `interface{}` as `unknown`. One
  unbindable member therefore stopped the command generating anything at all,
  and where it did generate, the declarations described functions the
  bindings never published, giving `wrap(undefined)` and a `TypeError` at the
  call site.
- Reading a struct-typed field returns the same wrapper every time. It used to
  build a fresh one per read, which broke `===`, `Map` keys and every
  framework's memo comparison, and allocated a handle and a set of bridge
  slots on each access. A struct field has a stable address, so the cached
  wrapper is still a live view; a pointer field is keyed on the pointer.
- A namespace read before `initializeCrystalline()` throws an error naming
  itself, rather than being `undefined` and failing somewhere unrelated.
- Fields declared `readonly` when they cannot be written back, and plain data
  declared `readonly` throughout.
- Minimum Go version is 1.26.
- A wrapper declares `release()` and `[Symbol.dispose]()`. The runtime set
  both on every wrapper and the declarations mentioned neither, so the
  deterministic release the documentation recommends did not typecheck.
  Plain data holds nothing, so it declares neither.
- A callback's return value is validated like any other value crossing into
  Go. `js.Value.String()` is the one accessor that does not panic on the
  wrong type, so a callback returning nothing handed Go the literal
  `"<undefined>"` as though it were the answer.
- `null` where a struct is expected is an error rather than a zero value.
  A pointer position handles its own nil before reaching the struct.
- A `[]byte` parameter checks that it was given a `Uint8Array`. Every typed
  array has a `byteLength`, so the old check never rejected anything and the
  failure surfaced as a `syscall/js` panic instead of a named error.
- A value a channel parameter cannot carry fails the call. Feeding
  `[1, "two", 3]` into a `<-chan int` used to end the stream at the bad
  value, so Go saw a clean end of input and answered for the values that had
  arrived.
- The Promise executor's callback is released once the constructor has run.
  It leaked a slot in the Go/JS bridge on every promise-returning call, for
  the life of the page.
- A call that takes a context and fails before handing back a stream now
  cancels that context. Nothing existed to take it over, so the abort
  listener and its callback were stranded.
- A variadic function compiles. Go spreads the final slice at the call site
  and the generated file passed it whole, so binding any `...T` function
  produced a package that would not build. JavaScript passes the values as
  an array.
- `bind.AsPromise()` on something that is not a function is refused. A
  promise is a way of returning and a value does not return, so the option
  was recorded and then never read.
- A method promoted from an embedded field is bound. The method set was read
  with `NumMethods`, which reports only what a type declares, so a struct
  that embeds another lost part of its surface with nothing reported.

#### Removed

- The whole reflection runtime: `Exposer`, `Map`, `MapPromise`, `MarkPromise`,
  `MarkIgnored` and the `js`-tagged conversion code.
- Package-level `JSQuoteStyle` and `JSTrailingComma`, replaced by
  `WithQuoteStyle` and `WithTrailingComma` on the generator.

#### Fixed

- `// crystalline:promise` on a method no longer depends on the Go source being
  readable at run time. The runtime parsed source files by absolute path, so the
  directive worked on the developer's machine and silently did nothing in a
  deployed binary.
- A panic inside a promise-returning function now rejects that promise. It used
  to resolve with `null` and leave the error in a global, where an unrelated
  later call would throw it.
- Unsupported parameter types are rejected when the entity is exposed. They used
  to produce a converter that silently did nothing, leaving a zero value that
  failed obscurely at call time.
- Errors name the entity and the parameter or field responsible, for example
  `unbindable.Send: parameter values: a send-only channel has no JS counterpart,
  return a <-chan instead`.
- A struct parameter containing unexported fields no longer panics.
- A struct that can only be partly converted is declined outright rather than
  silently dropping the fields that cannot be read.
- A bidirectional `chan T` parameter is refused rather than guessed at. It used
  to be read as a source, which would have discarded anything the function sent.
- `initializeCrystalline()` called before the wasm module starts throws an
  explanatory error rather than a `TypeError`.
- Coverage is collected in `atomic` mode; the flag was being parsed as a
  filename.
- A pointer parameter is declared as `T | undefined` rather than as an
  optional parameter. TypeScript rejects a required parameter that follows an
  optional one, so any function with a pointer before another parameter
  produced declarations that would not compile.
- A context combined with a returned channel no longer cancels the stream
  before anything has been read from it. The cancellation was scoped to the
  call, which returns as soon as it has handed the stream over, so the
  iterable arrived silently empty.
- An async iterable returned from Go now has a `return` method, so breaking
  out of a `for await` tears down whatever governs the stream instead of
  leaving it running.
- Writing a slice, map, `[]byte`, struct or pointer field no longer does
  nothing. Only basic types had a setter; every other write was accepted and
  discarded, leaving the Go value untouched and reporting nothing. Writes now
  go through the same converters a parameter does, a bad value throws where
  the write happened, and a field with no way back is `readonly`.

### Migrating from 0.0.15

Registration moves from calls made at start-up into a manifest the generator
reads:

```go
// before
func Expose() *crystalline.Exposer {
	e := crystalline.NewExposer("myapp")
	e.ExposeFuncOrPanic(api.Greet)
	e.ExposeFuncOrPanicPromise(api.Load)
	crystalline.MarkIgnored("vendor.Client", "internalHelper")
	e.ExposeOrPanic(api.Version, "api", "Version")
	return e
}

// after
//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(api.Greet)
	r.Func(api.Load, bind.AsPromise())
	r.Ignore(vendor.Client{}, "internalHelper")
	r.Value("Version", api.Version, bind.InNamespace("api"))
}
```

The manifest still runs, so values may be computed however you like; only their
types are read when generating.

Then generate instead of building and running an exposition binary:

```sh
go get -tool github.com/Vilsol/crystalline/cmd/crystalline
```

```go
//go:generate go tool crystalline -app myapp -out ./dist ./...
```

On the JavaScript side, calls that return an `error` now hand back a `Result`:

```ts
// before
const [config, err] = api.LoadConfig();
if (err) { throw err; }

// after
const config = api.LoadConfig().unwrap();
```

An undirected `chan T` parameter must state its direction; write `<-chan T` to
receive what the caller supplies.
