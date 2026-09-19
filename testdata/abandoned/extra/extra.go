// Package extra is named by a type crystalline tries and fails to convert, and
// by nothing else.
package extra

import "github.com/Vilsol/crystalline/testdata/abandoned/sub"

// Thing cannot be read from JS: Runtime has no counterpart. Good is reached
// first and converts, so the converter emitted for it outlives the attempt.
type Thing struct {
	Name    string
	Good    sub.Fine
	Runtime interface{}
}
