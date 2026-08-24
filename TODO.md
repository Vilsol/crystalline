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

### Chasing full TinyGo support

It already works. Every example and the generator's own test surface — 82 checks
over every kind of crossing — produce identical results under TinyGo 0.41 and
under the standard toolchain, at roughly a fifth of the size. `./examples/tinygo.sh`
runs it. README says so, with the caveat.

Trialled on timeless-jewels, which is the consumer that cares most about the
cost of a crossing: it worked, and it was slower overall. `-scheduler=asyncify`
rewrites every function that can block, and that is not free. For a tool built
to squeeze the boundary, a fifth of the download does not pay for it.

So the remaining work is parked rather than planned:

- Two panics that cannot be reported: a JavaScript callback, or a method of a
  supplied object, that returns the wrong type or rejects. Both convert inside a
  Go function whose signature belongs to the consumer. They are the only two
  probe checks that cannot run under TinyGo, and closing them needs either a
  TinyGo with recover or a JS contract that cannot supply the wrong type.
- Whether TinyGo's conservative collector disturbs the handle table.

Revisit if a consumer turns up whose binary size matters more than its latency —
a page loaded once and thrown away rather than one used for an hour. Nothing has
to change for them today; they get the caveat and the size.

### Binary size attribution

Reporting which binding dragged which bytes into the wasm binary. Attribution
through the Go linker is approximate enough that people stop trusting the
numbers, which is worse than not reporting them.
