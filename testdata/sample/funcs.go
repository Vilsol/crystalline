package sample

import (
	"context"
	"errors"
	"time"
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

// WithText takes a callback returning a string. js.Value.String() is the one
// accessor that does not panic on the wrong type: it renders undefined as the
// literal "<undefined>", which used to reach Go as a real value.
func (r Richer) WithText(cb func(v string) string) string {
	return cb("x")
}

func Rich() Richer {
	return Richer{Pointed: &FnSample{FirstValue: "pointed"}}
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

// Middle takes a pointer before a required parameter. A pointer means null is
// allowed, not that the argument may be left out, so the declaration must not
// mark it optional: a required parameter cannot follow an optional one.
func Middle(first *FnSample, label string) string {
	if first == nil {
		return label
	}

	return first.FirstValue + label
}

// Ticks streams count values under a context. The context governs the stream
// rather than the call that hands it back, so cancelling it must end the
// stream, and leaving it alone must let the stream finish.
func Ticks(ctx context.Context, count int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)

		for i := range count {
			select {
			case out <- i:
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// Keys takes a nilable slice before a required parameter. A nil slice arrives
// as null, which is not the same as the argument being absent, so like a
// pointer it must not be declared optional.
func Keys(ids []int, seed int) int {
	return len(ids) + seed
}

// Ticker carries a field that can be read but not written: a channel has no
// way back from JavaScript.
type Ticker struct {
	Name   string
	Events <-chan string
}

func NewTicker() Ticker {
	events := make(chan string)
	close(events)

	return Ticker{Name: "ticker", Events: events}
}

// Reading is only ever read on the JavaScript side, so the manifest asks for it
// as plain data rather than as a live wrapper.
type Reading struct {
	Label  string
	Values []float64
	Peak   Sample
}

// Sample is reached only through Reading, so it is plain too: a plain value
// cannot contain a live one.
type Sample struct {
	At    string
	Value float64
}

// Describe has nowhere to live on plain data, and is reported as skipped.
func (s Sample) Describe() string {
	return s.At
}

func Readings(count int) []Reading {
	out := make([]Reading, count)

	for i := range out {
		out[i] = Reading{
			Label:  "reading",
			Values: []float64{1, 2},
			Peak:   Sample{At: "noon", Value: float64(i)},
		}
	}

	return out
}

// Kind is a named type over a builtin, the shape a lookup table is keyed by.
type Kind uint32

// Big traffics in 64-bit integers, which JavaScript numbers cannot represent
// exactly beyond 2^53. It is still bound, with a warning.
func Big(n int64) uint64 {
	return uint64(n)
}

// Streamable takes a context and hands back a stream, but can fail before there
// is anything to stream. The context has to be torn down on that path: no
// iterator exists to take it over.
func Streamable(ctx context.Context, ok bool) (<-chan int, error) {
	if !ok {
		return nil, errors.New("asked to fail")
	}

	out := make(chan int)
	close(out)

	return out, nil
}

// Total is variadic. Go spreads the final slice at the call site; generated
// code that passes it as a plain argument does not compile.
func Total(nums ...int) int {
	sum := 0

	for _, n := range nums {
		sum += n
	}

	return sum
}

// Base is embedded, and its method is promoted onto whatever embeds it.
type Base struct {
	Tag string
}

func (b Base) Promoted() string {
	return b.Tag
}

// Embedder embeds Base, so Go's promotion rules put Tag and Promoted on it.
// Reading only the declared method set loses both, silently.
type Embedder struct {
	Base

	Own string
}

func (e Embedder) Direct() string {
	return e.Own
}

func MakeEmbedder() Embedder {
	return Embedder{Base: Base{Tag: "tagged"}, Own: "own"}
}

// Stamped carries a time, the shape almost every real struct has and the one
// crystalline used to bind as a field-less wrapper carrying thirty methods.
type Stamped struct {
	Label string
	At    time.Time
}

func TakesTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func MakeStamped() Stamped {
	return Stamped{Label: "stamped", At: time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)}
}

// Phase is an enum in the Go sense: a named integer with a fixed set of values
// and a String method. Bound as a bare number, the names and the text are lost.
type Phase int

const (
	PhaseIdle Phase = iota
	PhaseRunning
	PhaseDone
)

func (p Phase) String() string {
	switch p {
	case PhaseIdle:
		return "idle"
	case PhaseRunning:
		return "running"
	case PhaseDone:
		return "done"
	}

	return "unknown"
}

func Advance(p Phase) Phase {
	if p == PhaseDone {
		return PhaseDone
	}

	return p + 1
}

// Recorder is supplied by JavaScript. Go declares what it needs, and the app
// hands in an object providing it: the other direction from everything else
// here, without a second generator to bind arbitrary browser APIs.
type Recorder interface {
	Record(event string)
	Level() int
}

func Replay(r Recorder, events []string) int {
	for _, event := range events {
		r.Record(event)
	}

	return r.Level()
}
