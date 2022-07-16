package crystalline

import (
	"encoding/json"
	"testing"

	"github.com/Vilsol/crystalline/nested"
)

type ComplexObj struct {
	Name   string
	Data   map[uint32]float64
	Nested nested.AnotherObj
}

var data ComplexObj

func init() {
	data = ComplexObj{
		Name: "Bob",
		Data: map[uint32]float64{
			0: 1.23,
			1: 4.56,
			2: 7.89,
		},
		Nested: nested.AnotherObj{
			SomeValue: []int{0, 1, 1, 2, 3, 5, 8},
		},
	}
}

func BenchmarkJSON(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(data)
	}
}

func BenchmarkMap(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = Map(data)
	}
}
