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
	X      float64
	Y      float64
	Label  string
	Origin Anchor
}

// Anchor is a struct-typed field, whose wrapper is cached per parent so that
// reading it twice gives the same object.
type Anchor struct {
	X float64
	Y float64
}

// Reading is the same shape as Point, marshalled as plain data instead.
type Reading struct {
	X      float64
	Y      float64
	Label  string
	Origin Anchor
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

// MakeReadings is MakePoints for a type the manifest marks plain, so the two
// can be compared directly.
func MakeReadings(n int) []Reading {
	out := make([]Reading, n)

	for i := range out {
		out[i] = Reading{X: float64(i), Y: float64(i), Label: "p"}
	}

	return out
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

// Record is the shape a real result has: about ten fields, some of them nested
// slices and structs, and a couple of methods. What it costs to hand one over,
// and to copy it out as data, is what BenchmarkRecord measures.
type Record struct {
	ID     int
	Name   string
	Score  float64
	Active bool
	Tags   []string
	Lines  []Line
	Grid   [][]float64
	Totals map[string]float64
	Main   Line
	Parent *Line
}

// Line is the struct a Record nests, by value, by pointer and in a slice.
type Line struct {
	Label  string
	Values []float64
}

func (r *Record) Total() float64 {
	sum := r.Score

	for _, line := range r.Lines {
		for _, value := range line.Values {
			sum += value
		}
	}

	return sum
}

func (r *Record) Describe() string {
	return r.Name + " #" + strconv.Itoa(r.ID)
}

func (l *Line) Sum() float64 {
	var sum float64

	for _, value := range l.Values {
		sum += value
	}

	return sum
}

// MakeRecord returns a filled Record with four of everything.
func MakeRecord() *Record {
	line := func(label string) Line {
		return Line{Label: label, Values: []float64{1, 2, 3, 4}}
	}

	parent := line("parent")

	return &Record{
		ID:     7,
		Name:   "record",
		Score:  1.5,
		Active: true,
		Tags:   []string{"a", "b", "c", "d"},
		Lines:  []Line{line("l0"), line("l1"), line("l2"), line("l3")},
		Grid:   [][]float64{{1, 2, 3, 4}, {5, 6, 7, 8}, {9, 10, 11, 12}, {13, 14, 15, 16}},
		Totals: map[string]float64{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6, "g": 7, "h": 8},
		Main:   line("main"),
		Parent: &parent,
	}
}
