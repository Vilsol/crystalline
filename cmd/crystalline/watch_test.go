package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MarvinJWendt/testza"
)

func TestSourcesNoticeAChange(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	testza.AssertNoError(t, os.WriteFile(file, []byte("package main\n"), 0o644))

	before, err := sources(dir)
	testza.AssertNoError(t, err)
	testza.AssertEqual(t, 1, len(before))

	testza.AssertFalse(t, changed(before, before), "nothing changed yet")

	// Written a second later, because a file system may only keep whole seconds.
	testza.AssertNoError(t, os.Chtimes(file, time.Now(), time.Now().Add(time.Second)))

	after, err := sources(dir)
	testza.AssertNoError(t, err)
	testza.AssertTrue(t, changed(before, after), "a write must be noticed")

	testza.AssertNoError(t, os.Remove(file))

	gone, err := sources(dir)
	testza.AssertNoError(t, err)
	testza.AssertTrue(t, changed(after, gone), "a removal must be noticed")
}

func TestSourcesSkipDependencies(t *testing.T) {
	dir := t.TempDir()

	for _, path := range []string{"main.go", "node_modules/pkg/index.go", ".git/hook.go", "vendor/dep/dep.go"} {
		full := filepath.Join(dir, path)
		testza.AssertNoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		testza.AssertNoError(t, os.WriteFile(full, []byte("package main\n"), 0o644))
	}

	found, err := sources(dir)
	testza.AssertNoError(t, err)

	testza.AssertEqual(t, 1, len(found), "only the project's own Go is watched")
}
