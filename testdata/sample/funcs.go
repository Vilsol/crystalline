package sample

import (
	"context"
	"errors"
)

type FnSample struct {
	FirstValue  string
	SecondValue int
	ThirdValue  float32
}

func (s FnSample) A(x bool) bool {
	return x
}

func (s *FnSample) B(x bool) bool {
	return x
}

// crystalline:promise
func (s FnSample) C(x bool) bool {
	return x
}

func (s FnSample) One() string {
	return s.FirstValue
}

func (s FnSample) Two() int {
	return s.SecondValue
}

func (s FnSample) Three() float32 {
	return s.ThirdValue
}

func FooBar() FnSample {
	return FnSample{
		FirstValue:  "hello",
		SecondValue: 123,
		ThirdValue:  4.56,
	}
}

func Basic() int {
	return 420
}

type Richer struct {
	Blob       []byte
	Lookup     map[string]int
	MightBeNil []string
	NeverNil   []string `crystalline:"not_nil"`
	Pointed    *FnSample
	Inner      FnSample
}

func (r Richer) Fails() error {
	return nil
}

func (r Richer) WithCallback(cb func(v string) int) bool {
	return cb("x") > 0
}

func Rich() Richer {
	return Richer{}
}

// Configure takes a struct by value, and echoes a field back so a caller can
// tell whether the value survived the crossing.
func (r Richer) Configure(s FnSample) string {
	return s.FirstValue
}

// Apply takes a struct whose fields cannot all be read back from JS, so the
// generated bindings decline to bind it rather than dropping fields.
func (r Richer) Apply(other Richer) int {
	return len(other.MightBeNil)
}

// MayFail returns a value and an error, the shape JavaScript expects to be a
// rejecting promise rather than a tuple.
func MayFail(ok bool) (string, error) {
	if !ok {
		return "", errors.New("asked to fail")
	}

	return "fine", nil
}

// OnlyFails returns nothing but an error.
func OnlyFails(ok bool) error {
	if !ok {
		return errors.New("asked to fail")
	}

	return nil
}

// Stream hands back a receive-only channel, which JavaScript consumes with
// for await.
func Stream(count int) <-chan string {
	out := make(chan string)

	go func() {
		defer close(out)

		for i := 0; i < count; i++ {
			out <- "item"
		}
	}()

	return out
}

// Cancellable takes a context, which JavaScript drives with an AbortSignal.
func Cancellable(ctx context.Context, label string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
		return label, nil
	}
}

// Sum drains a channel the caller supplies, which JavaScript fills from any
// iterable.
func Sum(values <-chan int) int {
	total := 0

	for value := range values {
		total += value
	}

	return total
}

// First takes one value and abandons the rest, so the caller's feed has to be
// stopped rather than left blocked forever.
func First(values <-chan int) int {
	for value := range values {
		return value
	}

	return -1
}
