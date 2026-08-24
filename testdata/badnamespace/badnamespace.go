// Package badnamespace asks for a namespace JavaScript cannot name.
package badnamespace

import "github.com/Vilsol/crystalline/bind"

func Value() int { return 1 }

//crystalline:exports
func Exports(r bind.Registry) {
	r.Func(Value, bind.InNamespace("my-api"))
}
