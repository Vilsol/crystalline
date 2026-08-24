// Package marshal declares a type that crosses as something other than its
// fields, using a pair of ordinary Go functions.
package marshal

import (
	"errors"
	"strings"
)

// Colour is a value whose meaning is its text, not its structure.
type Colour struct {
	Red   int
	Green int
	Blue  int
}

// ColourToHex is the way out. Its signature is what tells the generator both
// the Go type and the type it crosses as.
func ColourToHex(c Colour) string {
	return "#" + hex(c.Red) + hex(c.Green) + hex(c.Blue)
}

// ColourFromHex is the way back, and reports what it cannot read.
func ColourFromHex(text string) (Colour, error) {
	if len(text) != 7 || !strings.HasPrefix(text, "#") {
		return Colour{}, errors.New("expected a colour like #aabbcc")
	}

	var c Colour
	for i, target := range []*int{&c.Red, &c.Green, &c.Blue} {
		value, err := parseHex(text[1+i*2 : 3+i*2])
		if err != nil {
			return Colour{}, err
		}

		*target = value
	}

	return c, nil
}

func Brighten(c Colour) Colour {
	return Colour{Red: min(c.Red+16, 255), Green: min(c.Green+16, 255), Blue: min(c.Blue+16, 255)}
}

const digits = "0123456789abcdef"

func hex(v int) string {
	return string([]byte{digits[(v>>4)&0xf], digits[v&0xf]})
}

func parseHex(text string) (int, error) {
	out := 0

	for _, r := range text {
		index := strings.IndexRune(digits, r)
		if index < 0 {
			return 0, errors.New("expected a colour like #aabbcc")
		}

		out = out*16 + index
	}

	return out, nil
}
