// Command catalogue shows Go's own way of modelling reaching JavaScript: an
// enum as a set of values, a value type as the text that means it, embedding as
// promotion, and an interface as something JavaScript provides.
package main

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/examples/04-catalogue/catalogue"
)

//go:generate go tool crystalline -app catalogue -out . ./...

//crystalline:exports
func Exports(r bind.Registry) {
	// Money's meaning is its text, so it crosses as text. The two signatures
	// say the rest: func(Money) string out, func(string) (Money, error) back.
	r.Type(catalogue.Money{}, bind.MarshalledBy(catalogue.MoneyToText, catalogue.MoneyFromText))

	r.Func(catalogue.All)
	r.Func(catalogue.Describe)
	r.Func(catalogue.Restock)
}

func main() {
	select {}
}
