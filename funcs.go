//go:build !js

package crystalline

import "reflect"

// convertFunc is just a placeholder
func convertFunc(_ reflect.Value) (interface{}, error) {
	return nil, nil
}
