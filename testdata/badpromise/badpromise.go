// Package badpromise asks for a promise where one cannot exist.
package badpromise

import "github.com/Vilsol/crystalline/bind"

//crystalline:exports
func Exports(r bind.Registry) {
	// A promise is a way of returning; a value does not return, so this asks
	// for something that cannot happen and used to be accepted in silence.
	r.Value("Answer", 42, bind.AsPromise())
}
