# TODO

Work that is decided but not built, and work that was considered and declined.
Declined items carry their reasoning so they are not re-argued from scratch.

## Planned

### Broad JS to Go imports

A narrow form ships in 0.1.0: a Go interface marked `//crystalline:imports`
gets an implementation backed by a JavaScript object supplied once at start-up,
plus the TypeScript interface that object has to satisfy. It reuses what
callback parameters already do, so it is a bundle of existing pieces rather
than a new direction.

The broad form is binding arbitrary browser APIs from Go, the way wasm-bindgen's
`extern` blocks do. That is a second product: its own error mapping, its own
callback lifetimes, and an async story where every call that awaits blocks a
goroutine. It is also the wrong architecture for the cost model — a crossing is
about 5.5 microseconds, so driving the DOM from Go per node loses to letting
JavaScript render what Go computed.

Revisit if someone shows a workload that genuinely has to originate in Go.

### TinyGo

The generated runtime needs `js.FuncOf`, channels, two goroutines, `sync.Once`,
`sync.Mutex` and generics. It needs no `reflect`, no `fmt` and no runtime
finalizers, which is the usual obstacle, and it is absent because the reflect
runtime was removed for binary size.

Spiked with `examples/tinygo.sh` on TinyGo 0.41.1, and it mostly works:

| example | tinygo | go   | result                     |
| ------- | ------ | ---- | -------------------------- |
| 01-hello    | 420K | 2.2M | passes                 |
| 02-accounts | 560K | -    | fails on release       |
| 03-pipeline | 502K | 2.4M | passes                 |

The part expected to break does not. 03-pipeline exercises promises, streams in
both directions, aborting mid-stream and an explicit `AsPromise`, and it all
works under `-scheduler=asyncify`, at about a fifth of the size. So
`crystallineAwait` blocking a goroutine on a JS callback is fine.

What fails is one path: after `release()`, handing the wrapper back gives
`RuntimeError: unreachable` rather than the error naming the released handle.
Earlier `assert.throws` cases in the same file pass, so `recover` itself works;
the suspect is `js.Func.Release` semantics, since releasing a wrapper releases
the scope's funcs. Diagnose that before claiming support.

Still unchecked: `js.CopyBytesToGo` and `js.CopyBytesToJS`, and whether
TinyGo's collector disturbs the handle table.

### Opt-in bigint for 64-bit integers

`int64` and `uint64` are bound as JavaScript numbers with a warning, because
refusing them would break ordinary Go. A `crystalline:"bigint"` field tag would
let a project carry the exact value where the range genuinely matters.

Against it: `JSON.stringify` throws on a bigint, and mixing one with a number
throws a `TypeError`, so the choice leaks into the caller's arithmetic. Opt-in
rather than default for that reason.

### Opt-in camelCase

A `-case camel` flag, and a `crystalline:"name=timeout"` tag for individual
members. The tag parser in `tags.go` already handles options.

Not the default: an exported Go name being the JavaScript name means a grep
finds both sides, which is a live debugging aid in a two-language codebase.

## Declined

### Generated batching

`bind.Batched()` emitting a `GetAll(keys)` beside `Get(key)`. A crossing costs
the same whoever writes the binding, so 98 small calls cost about half a
millisecond against 5.5 microseconds for one aggregate call, and that gap is
real.

It is declined because it is a trap unless it forces plain data: 98 wrappers at
about 91 microseconds each is slower than the per-call plain path it would be
sold as replacing. If it is ever built it must require `r.Plain` on the return
type and refuse at generate time otherwise, fail the whole batch rather than
carrying a `Result` per element, and let the varying parameter be declared
rather than assumed to be the first.

The evidence against it is that the second hot spot in a real migration was a
map read rather than a call, which batching cannot touch, and `r.Plain` fixed
it for nothing.

### Binary size attribution

Reporting which binding dragged which bytes into the wasm binary. Attribution
through the Go linker is approximate enough that people stop trusting the
numbers, which is worse than not reporting them.
