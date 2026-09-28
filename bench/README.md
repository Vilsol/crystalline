# bench

What it costs to cross the Go/JavaScript boundary.

Every operation is bound twice: once by crystalline, and once by hand through
`syscall/js`, doing the same conversions. The hand-written line is the floor —
the wasm bridge itself, which no binding layer avoids — so the gap between a
pair is what crystalline is responsible for.

```sh
./bench/run.sh                          # everything, about a minute
./bench/run.sh -bench BenchmarkString   # one group
```

The benchmarks hand JavaScript a loop to run rather than calling each binding
from Go. A Go benchmark calling in directly would pay a Go to JS to Go round
trip per call, several microseconds of it, which is not what a page pays and is
large enough to bury what is being measured. The reported `ns/op` is therefore
one call as JavaScript makes it, with the batch divided back out.

## Measured

One machine, node 24, Go 1.26. The ratios travel; the absolute numbers do not.

| | crystalline | hand-written | |
| --- | ---: | ---: | --- |
| call, no arguments | 5.7 µs | 5.5 µs | the bridge, and nothing else |
| two ints in, one out | 5.3 µs | 5.6 µs | conversion is free at this size |
| string, 16 B round trip | 7.2 µs | 7.3 µs | |
| string, 64 KB round trip | 78 µs | 64 µs | 1.2 ns per byte, both ways |
| bytes, 64 KB round trip | 22 µs | 23 µs | 3.5× faster than the same size as a string |
| array in, 1024 numbers | 90 µs | 58 µs | 88 ns per element |
| array out, 1024 numbers | 120 µs | 115 µs | 118 ns per element |
| object in, 256 keys | 465 µs | 537 µs | 1.8 µs per key |
| object out, 256 keys | 54 µs | 105 µs | as one JSON string |
| 32 structs out, wrappers | 330 µs | 204 µs | 10 µs per wrapper |
| **32 structs out, `bind.Plain()`** | **24 µs** | 204 µs | as one JSON string, 8.5× under the hand-written line |
| record out and read in full, wrapper | 240 µs | 65 µs | `BenchmarkRecord/dump`: 10 fields, 7 wrappers, 424 crossings |
| **record out, `bind.PlainResult()`** | **15 µs** | 65 µs | `BenchmarkRecord/plain`: the same data, 13 crossings |
| one wrapper out, released | 17 µs | | two calls: the function and `release` |
| field read, wrapper | 6.7 µs | 9.1 ns (plain data) | |
| field read, nested struct | 6.8 µs | | cached per parent, so no worse than a scalar |
| field write, wrapper | 6.0 µs | 7.5 ns (plain data) | |
| method on a wrapper | 5.7 µs | | no worse than a free function |
| struct in, as a wrapper | 7.8 µs | | resolved through the handle table |
| struct in, as a literal | 17.7 µs | | validated field by field |
| `Result`, success | 10.4 µs | | |
| `Result`, success + `unwrap` | 9.9 µs | | the methods are on a shared prototype |
| `Result`, failure | 18.4 µs | | building the `Error` |
| promise | 22.4 µs | 9.8 µs (`Promise.resolve`) | goroutine and channel |
| stream, 32 items | 1115 µs | | 35 µs per item |
| channel parameter, 32 items | 748 µs | | 23 µs per item |

## What it says

1. **A crossing costs about 5.5 µs, whoever writes it.** For scalar calls
   crystalline is indistinguishable from a hand-written binding, so the thing to
   count is crossings, not conversions.
2. **Mark a read-only result `bind.Plain()`, or the call `bind.PlainResult()`.**
   A live wrapper is one call to build, since its accessors and methods belong
   to its type, but reading it back out is a crossing per field and per nested
   wrapper: a ten-field record read in full costs 16× the same record handed
   over as plain data. Plain data crosses as one JSON string, because every Go
   string that crosses on its own — field names and map keys included — is a
   `TextDecoder` call of its own, and that call is the expensive part.
3. **A wrapper field is a function call, not a property**: 6.7 µs against 9.1 ns
   on plain data. Read it into a local rather than in a loop. A struct-typed
   field is cached per parent, so reading one is no worse than reading a scalar
   and gives the same object every time.
4. **Send bulk data as `[]byte`.** 64 KB crosses 3.5× faster as a `Uint8Array`
   than as a string, because it copies in one go instead of encoding.
5. **`Result` is cheap and `unwrap` is free**, but failing costs 1.8× succeeding,
   since the `Error` is built on the way out.

The benchmarks do not include the one extra closure the generated ES module
wraps around each binding, because a Go test cannot import an ES module.
