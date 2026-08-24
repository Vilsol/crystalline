// Package outer returns a type declared elsewhere.
package outer

import "github.com/Vilsol/crystalline/testdata/crosspkg/inner"

// Make hands out an inner.Payload.
func Make() inner.Payload {
	return inner.Payload{Label: "hello"}
}

// Switch traffics in a type from inner, which is the only reason inner is
// loaded at all.
func Switch(m inner.Mode) inner.Mode {
	return m
}
