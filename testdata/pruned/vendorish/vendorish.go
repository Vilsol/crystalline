// Package vendorish stands in for a dependency whose types leak through
// methods the manifest does not expose.
package vendorish

type Doc struct {
	Title string
}

// Wire is excluded by the manifest, so Writer is reachable from nothing.
func (d Doc) Wire() Writer { return Writer{} }

type Writer struct{ N int }

type Plainish struct {
	Title string
}

// Helper is reachable only through a method of a plain type, and plain data
// carries no methods.
func (p Plainish) Helper() Helper { return Helper{} }

type Helper struct{ N int }
