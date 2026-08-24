// Package greeting is ordinary Go. Nothing in it knows about JavaScript, wasm
// or crystalline: the bindings are generated from the outside.
package greeting

import "strings"

// Greet builds a greeting for name, defaulting to the world at large.
func Greet(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "world"
	}

	return "Hello, " + name + "!"
}

// Add exists to show a non-string crossing the boundary: Go ints arrive in
// JavaScript as numbers.
func Add(a int, b int) int {
	return a + b
}
