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

All four examples build and pass under TinyGo 0.41.1 with
`-scheduler=asyncify`, at roughly a fifth of the size:

| example | tinygo | go   |
| ------- | ------ | ---- |
| 01-hello    | 430K | 2.3M |
| 02-accounts | 574K | 2.4M |
| 03-pipeline | 516K | 2.5M |
| 04-catalogue| 661K | 2.5M |

Run `./examples/tinygo.sh <example>`. TinyGo is not pinned in `mise.toml`, so
either install it or run the script through `mise exec tinygo@0.41.1 --`.

Between them the examples cover promises, streams in both directions, aborting
mid-stream, an explicit `AsPromise`, live wrappers, deterministic release,
enums, a mapped type, a promoted embedded method, an interface supplied from
JavaScript, and every error path the smoke tests can reach. `js.CopyBytesToGo`
and `js.CopyBytesToJS` copy correctly.

What made this work is that generated code no longer panics to report a
conversion failure. **`recover()` is not implemented on TinyGo's wasm target**:
`runtime.panicOrGoexit` consults `supportsRecover()`, which is false because
unwinding needs `tinygo_longjmp`, and `asm_tinygowasm.S` is the only
architecture stub that does not define it. Measured rather than reasoned — the
same probe recovers under `tinygo build` for linux/amd64 and traps under
`-target wasm`, in `main` itself, through a callee and inside a goroutine,
written as a closure and as a named deferred function, from a plain callee and
from a generic one.

Two panics remain, and cannot be removed: what a JavaScript callback returned
and what a method of a JavaScript-supplied object returned are converted inside
a Go function whose signature belongs to the consumer, so there is nowhere to
report to and nothing to return but a guess. A consumer's own panic is the same
problem one level up. Under the standard toolchain all three become a thrown or
rejected `Error` carrying the Go stack; under TinyGo they abort the module.

The README states this, with the caveat. What is left before it can drop the
caveat:

- The two conversions above, which need either a TinyGo with recover or a JS
  contract that cannot supply the wrong type in the first place. A consumer's
  own panic is the same problem one level up and is not crystalline's to fix.
- Whether TinyGo's conservative collector disturbs the handle table.
- A consumer of real size. The examples are small, and `-scheduler=asyncify`
  rewrites every function that can block.

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
