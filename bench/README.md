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
| call, no arguments | 5.7 µs | 5.6 µs | the bridge, and nothing else |
| two ints in, one out | 5.1 µs | 5.5 µs | conversion is free at this size |
| string, 16 B round trip | 7.6 µs | 6.9 µs | |
| string, 64 KB round trip | 78 µs | 76 µs | 1.2 ns per byte, both ways |
| bytes, 64 KB round trip | 23 µs | 22 µs | 3.4× faster than the same size as a string |
| array in, 1024 numbers | 88 µs | 60 µs | 86 ns per element |
| array out, 1024 numbers | 115 µs | 117 µs | 113 ns per element |
| object in, 256 keys | 508 µs | 499 µs | 2 µs per key |
| object out, 256 keys | 107 µs | 115 µs | 0.4 µs per key |
| **32 structs out** | **2184 µs** | **114 µs** | 68 µs per wrapper |
| one struct out, released | 79 µs | | |
| field read, wrapper | 6.8 µs | 6.4 ns (plain object) | |
| field write, wrapper | 7.7 µs | 3.5 ns (plain object) | |
| method on a wrapper | 5.8 µs | | no worse than a free function |
| struct in, as a wrapper | 7.2 µs | | resolved through the handle table |
| struct in, as a literal | 17.9 µs | | validated field by field |
| `Result`, success | 10.0 µs | | |
| `Result`, success + `unwrap` | 10.1 µs | | the methods are on a shared prototype |
| `Result`, failure | 18.2 µs | | building the `Error` |
| promise | 23.6 µs | 9.7 µs (`Promise.resolve`) | goroutine and channel |
| stream, 32 items | 1020 µs | | 32 µs per item |
| channel parameter, 32 items | 756 µs | | 24 µs per item |

## What it says

1. **A crossing costs about 5.5 µs, whoever writes it.** For scalar calls
   crystalline is indistinguishable from a hand-written binding, so the thing to
   count is crossings, not conversions.
2. **A struct wrapper costs 68 µs, and a slice of them costs that each.**
   Wrappers are live: every field is an accessor pair and every method a bound
   function, each holding a slot in the bridge. Returning a few is fine;
   returning thousands is not.
3. **A wrapper field is a function call, not a property.** 6.8 µs against 6.4 ns
   for a plain object. Read a field once into a local rather than in a loop.
4. **Send bulk data as `[]byte`.** 64 KB crosses 3.4× faster as a `Uint8Array`
   than as a string, because it copies in one go instead of encoding.
5. **`Result` is cheap and `unwrap` is free**, but failing costs 1.8× succeeding,
   since the `Error` is built on the way out.

The benchmarks do not include the one extra closure the generated ES module
wraps around each binding, because a Go test cannot import an ES module.
