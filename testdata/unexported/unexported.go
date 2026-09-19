// Package unexported carries members JavaScript can never reach.
package unexported

// Awkward is convertible in no direction: a complex number has no JS
// counterpart. Nothing exported mentions it.
type Awkward struct{}

func (a Awkward) Complex() complex128 { return 0 }

// Holder exposes one string and hides the rest.
type Holder struct {
	Name   string
	hidden Awkward
}

func (h Holder) Visible() string { return h.Name }

func (h Holder) invisible(a Awkward) {}

//crystalline:export
func Hold() Holder { return Holder{} }
