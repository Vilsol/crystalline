//go:build !js

package crystalline

// convertError is just a placeholder
func convertError(_ error) (interface{}, error) {
	return nil, nil
}
