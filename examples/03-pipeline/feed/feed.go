// Package feed shows the parts of Go that have a JavaScript counterpart:
// channels are async iterables, a context is an abort signal, and a slow call
// can be handed over as a promise.
package feed

import (
	"context"
	"errors"
	"strconv"
	"time"
)

// Primes streams the primes below limit, pausing between them so a page has
// time to paint. A receive-only channel arrives as an AsyncIterable, and the
// context makes the stream cancellable from JavaScript.
func Primes(ctx context.Context, limit int) <-chan int {
	out := make(chan int)

	go func() {
		defer close(out)

		for n := 2; n < limit; n++ {
			if !prime(n) {
				continue
			}

			select {
			case out <- n:
			case <-ctx.Done():
				return
			}

			select {
			case <-time.After(40 * time.Millisecond):
			case <-ctx.Done():
				return
			}
		}
	}()

	return out
}

// Average drains a channel JavaScript fills. Anything iterable will do: an
// array, a generator, or a stream that produces values over time.
func Average(values <-chan float64) float64 {
	var sum float64

	count := 0

	for value := range values {
		sum += value
		count++
	}

	if count == 0 {
		return 0
	}

	return sum / float64(count)
}

// Crunch is slow and can fail. The context makes it interruptible, which is the
// only way to stop Go work that would otherwise hold the single JS thread.
func Crunch(ctx context.Context, rounds int) (int, error) {
	if rounds <= 0 {
		return 0, errors.New("rounds must be positive")
	}

	total := 0

	for i := range rounds {
		select {
		case <-ctx.Done():
			return 0, errors.New("cancelled after " + strconv.Itoa(i) + " rounds")
		case <-time.After(100 * time.Millisecond):
		}

		total += i * i
	}

	return total, nil
}

// Digest is ordinary and synchronous. The manifest hands it over as a promise
// anyway, so the call site does not sit waiting for the result.
func Digest(text string, rounds int) string {
	hash := uint32(2166136261)

	for range rounds {
		for _, r := range text {
			hash ^= uint32(r)
			hash *= 16777619
		}
	}

	return strconv.FormatUint(uint64(hash), 16)
}

func prime(n int) bool {
	for d := 2; d*d <= n; d++ {
		if n%d == 0 {
			return false
		}
	}

	return n >= 2
}
