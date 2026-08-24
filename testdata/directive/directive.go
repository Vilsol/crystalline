// Package directive uses the shorthand for code you own.
package directive

//crystalline:export
func Owned() string { return "owned" }

//crystalline:export
//crystalline:promise
func Slow() string { return "slow" }

// Unmarked is not exported to JS.
func Unmarked() string { return "no" }
