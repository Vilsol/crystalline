// Package client is one of two packages that want the same JS namespace.
package client

// Alpha is exposed from the first client package.
func Alpha() string { return "alpha" }
