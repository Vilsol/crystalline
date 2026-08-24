// Command pipeline shows Go's concurrency reaching JavaScript: channels as
// async iterables, a context as an abort signal, and an explicit promise.
package main

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/examples/03-pipeline/feed"
)

//go:generate go tool crystalline -app pipeline -out . ./...

//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(feed.Primes)
	r.Func(feed.Average)
	r.Func(feed.Crunch)

	// Nothing about Digest forces it to be asynchronous, so ask for it.
	r.Func(feed.Digest, bind.AsPromise())
}

func main() {
	select {}
}
