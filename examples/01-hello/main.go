// Command hello is the smallest useful crystalline binding: two functions,
// exposed to JavaScript from a wasm binary.
package main

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/examples/01-hello/greeting"
)

//go:generate go tool crystalline -app hello -out . ./...

// Exports declares the JavaScript surface.
//
// The generator reads this function to learn which symbols are exposed and what
// their types are. The function itself runs at start-up, once the generated
// bindings have been registered.
//
//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(greeting.Greet)
	r.Func(greeting.Add)
}

// main must not return. A Go wasm program that exits takes its bindings with
// it, so the entrypoint parks and lets JavaScript drive.
func main() {
	select {}
}
