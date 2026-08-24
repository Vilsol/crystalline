# TODO

Work that is decided but not built, and work that was considered and declined.
Declined items carry their reasoning so they are not re-argued from scratch.

## Planned

### Broad JS to Go imports

The narrow form shipped in 0.1.0, as an interface parameter: Go declares what it
needs and JavaScript passes an object with those methods. No directive was
needed in the end, because a parameter already says where the value comes from.

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

Spiked with `examples/tinygo.sh` on TinyGo 0.41.1.

| example | tinygo | go   | result                            |
| ------- | ------ | ---- | --------------------------------- |
| 01-hello    | 420K | 2.2M | passes                        |
| 02-accounts | 562K | -    | traps on every error path     |
| 03-pipeline | 502K | 2.4M | passes                        |

The async surface works. 03-pipeline exercises promises, streams in both
directions, aborting mid-stream and an explicit `AsPromise`, all under
`-scheduler=asyncify` at about a fifth of the size, so `crystallineAwait`
blocking a goroutine on a JS callback is fine. `js.CopyBytesToGo` and
`js.CopyBytesToJS` both copy correctly.

**`recover()` is not implemented on TinyGo's wasm target.** Every panic prints
and then traps, and JavaScript sees `RuntimeError: unreachable` rather than the
Error the panic was meant to become. `runtime.panicOrGoexit` consults
`supportsRecover()`, which is false because unwinding needs `tinygo_longjmp`,
and `asm_tinygowasm.S` is the only architecture stub that does not define it —
a wasm stack is not addressable, so there is nothing to jump to.

Measured rather than reasoned: the same probe recovers under `tinygo build` for
linux/amd64 and traps under `-target wasm`, in `main` itself, through a callee
and inside a goroutine, with the recovery written as a closure and as a named
deferred function, from a plain callee and from a generic one. Six shapes, one
outcome.

So this is not one bad path. Every error crystalline reports from generated code
travels through `panic` and `crystallineRecover`: a mistyped argument, an object
literal with an unknown property, a released handle. All of them abort the
module under TinyGo. 01-hello and 03-pipeline pass because their smoke tests
never take an error path, and 02-accounts hid the same trap behind an
`assert.throws` with no pattern, which `RuntimeError: unreachable` satisfies as
happily as the real message. That assertion now names what it expects.

Supporting TinyGo therefore means not raising panics in generated code:
`crystallineMust` would return its error to the wrapper, which already knows how
to fail, and the recover would be left to catch only what it cannot prevent.
That is a change to every emitted wrapper and worth measuring, since it trades
one deferred call for a branch per argument.

A panic in the consumer's own Go code still cannot be reported under TinyGo. It
aborts, and nothing crystalline emits can change that. Under the standard
toolchain it is caught and thrown with the Go stack, so that is a real
difference in what the two toolchains can promise.

Still unchecked: whether TinyGo's conservative collector disturbs the handle
table.

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
