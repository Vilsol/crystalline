// Package outer returns a type declared elsewhere.
package outer

import "github.com/Vilsol/crystalline/testdata/crosspkg/inner"

// Make hands out an inner.Payload.
func Make() inner.Payload {
	return inner.Payload{Label: "hello"}
}
