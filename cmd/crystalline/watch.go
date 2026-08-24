package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Regenerating on save, so the declarations a page imports keep up with the Go
// they came from.
//
// The sources are polled rather than watched through the operating system,
// because a notification API would be a dependency every consumer of the
// library inherits, and a generate takes long enough that a third of a second
// of latency is not what anyone notices.

// pollInterval is how often the sources are compared.
const pollInterval = 300 * time.Millisecond

// sources maps every Go file under dir to when it was last written.
//
// Directories that cannot hold Go worth watching are skipped: a hidden
// directory, and the two that hold other people's code.
func sources(dir string) (map[string]time.Time, error) {
	found := make(map[string]time.Time)

	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			name := entry.Name()
			if path != dir && (strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor") {
				return filepath.SkipDir
			}

			return nil
		}

		if !strings.HasSuffix(entry.Name(), ".go") {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			// Vanished between the walk and the stat, which the next poll sees.
			return nil
		}

		found[path] = info.ModTime()

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s: %w", dir, err)
	}

	return found, nil
}

// changed reports whether anything was added, removed or written since before.
func changed(before map[string]time.Time, after map[string]time.Time) bool {
	if len(before) != len(after) {
		return true
	}

	for path, when := range after {
		was, ok := before[path]
		if !ok || !was.Equal(when) {
			return true
		}
	}

	return false
}

// watch regenerates on every change, and keeps going when one fails: a
// half-finished edit should leave the watcher waiting for the next save rather
// than exiting.
func watch(dir string, generate func() error) error {
	report := func(err error) {
		if err != nil {
			fmt.Fprintln(os.Stderr, "crystalline:", err)

			return
		}

		fmt.Fprintln(os.Stderr, "crystalline: generated")
	}

	report(generate())

	last, err := sources(dir)
	if err != nil {
		return err
	}

	for {
		time.Sleep(pollInterval)

		now, err := sources(dir)
		if err != nil {
			return err
		}

		if !changed(last, now) {
			continue
		}

		last = now

		report(generate())
	}
}
