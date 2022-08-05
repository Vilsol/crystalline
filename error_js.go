//go:build js

package crystalline

import (
	"syscall/js"
)

var errorConstructor js.Value

func init() {
	errorConstructor = js.Global().Get("Error")
}

// convertError is just a placeholder
func convertError(value error) (interface{}, error) {
	return errorConstructor.New(value.Error()), nil
}
