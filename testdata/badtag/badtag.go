// Package badtag carries a tag the field cannot honour.
package badtag

import "github.com/Vilsol/crystalline/bind"

// Sample is a struct with a pointer field, which has no empty form.
type Sample struct {
	Name string
}

// Tagged asks for not_nil on a pointer. Only a slice or a map has an empty
// value to stand in for nil, so the tag promises something it cannot deliver.
type Tagged struct {
	Pointer *Sample `crystalline:"not_nil"`
}

func Make() Tagged {
	return Tagged{}
}

//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(Make)
}
