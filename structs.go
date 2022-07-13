//go:build !js

package crystalline

import "reflect"

// convertFunc is just a placeholder
func convertStruct(value reflect.Value) (interface{}, error) {
	return nil, nil
}
