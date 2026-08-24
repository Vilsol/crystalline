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
- `Result<T>` for fallible calls, with `unwrap` and `unwrapOr`. Methods live on
  a shared prototype, so results cost nothing per call.
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
- Three worked examples under `examples`, each a page backed by a real wasm
  binary, built and run under node by CI.

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
- Minimum Go version is 1.26.

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
