// Package importer is a fixture calling out to an object JavaScript already
// has, rather than one handed in as a parameter.
package importer

// Storage is the few methods this package needs, not the whole Web Storage
// API. Declaring what is used is what keeps a generated import small.
type Storage interface {
	GetItem(key string) string
	SetItem(key string, value string)
}

// Slow is implemented in JavaScript as an async function, and is not declared
// to be one here. Awaiting it would block this goroutine inside a synchronous
// js.FuncOf callback, which hands JavaScript undefined and finishes the work
// afterwards.
type Slow interface {
	Fetch() string
}

// Local is filled at start-up from globalThis.localStorage.
var Local Storage

// Roundtrip writes through the imported object and reads it back, so a test can
// see that Go really reached JavaScript.
func Roundtrip(key string, value string) string {
	Local.SetItem(key, value)

	return Local.GetItem(key)
}

// Remote is filled from an object whose method returns a promise.
var Remote Slow

// Fetch calls the undeclared-async method, which must be refused rather than
// awaited.
func Fetch() string {
	return Remote.Fetch()
}

// Awaited is the same object, declared so that its promise is awaited.
var Awaited Slow

// FetchAwaited is exposed as a promise, so blocking its goroutine on the
// JavaScript event loop is safe: nothing synchronous is waiting above it.
func FetchAwaited() string {
	return Awaited.Fetch()
}

// Renamed is the same contract reached under the spellings a JavaScript API
// actually uses, without renaming the whole surface.
var Renamed Storage

// Fetched reads through the renamed import.
func Fetched(key string) string {
	return Renamed.GetItem(key)
}
