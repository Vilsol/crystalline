// Package only is named while a method wrapper's signature is rendered, and by
// nothing else once that wrapper is thrown away.
package only

// Token cannot be read from JS, so nothing is emitted that refers to it.
type Token struct {
	N       int
	Runtime interface{}
}
