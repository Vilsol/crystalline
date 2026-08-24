// Package payload holds the operations the benchmarks drive across the
// boundary. Each one is deliberately trivial in Go, so that what is measured is
// the crossing rather than the work.
package payload

import (
	"errors"
	"strconv"
)

// Point is a small struct, the shape a wrapper is built around.
type Point struct {
	X     float64
	Y     float64
	Label string
}

// Noop measures the cost of a call and nothing else.
func Noop() {}

func AddInts(a int, b int) int {
	return a + b
}

func EchoString(text string) string {
	return text
}

func EchoBytes(data []byte) []byte {
	return data
}

func SumFloats(values []float64) float64 {
	var sum float64

	for _, value := range values {
		sum += value
	}

	return sum
}

func MakeInts(n int) []int {
	out := make([]int, n)

	for i := range out {
		out[i] = i
	}

	return out
}

func CountKeys(index map[string]int) int {
	return len(index)
}

func MakeMap(n int) map[string]int {
	out := make(map[string]int, n)

	for i := range n {
		out[strconv.Itoa(i)] = i
	}

	return out
}

// MakePoints returns a slice of structs, which is what a real payload usually
// looks like and the most expensive thing to hand over.
func MakePoints(n int) []Point {
	out := make([]Point, n)

	for i := range out {
		out[i] = Point{X: float64(i), Y: float64(i), Label: "p"}
	}

	return out
}

func NewPoint(label string) *Point {
	return &Point{Label: label}
}

func (p *Point) Shift(dx float64, dy float64) {
	p.X += dx
	p.Y += dy
}

func (p *Point) Norm() float64 {
	return p.X*p.X + p.Y*p.Y
}

// TakePoint accepts a struct by value, so it can be handed either a wrapper or
// a plain object literal.
func TakePoint(p Point) float64 {
	return p.X + p.Y
}

func MayFail(ok bool) (int, error) {
	if !ok {
		return 0, errors.New("asked to fail")
	}

	return 1, nil
}

// Rounds is exposed as a promise, so its cost includes a trip through the JS
// event loop.
func Rounds(n int) int {
	return n
}

func Stream(n int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)

		for i := range n {
			out <- i
		}
	}()

	return out
}

func Drain(values <-chan int) int {
	count := 0

	for range values {
		count++
	}

	return count
}
