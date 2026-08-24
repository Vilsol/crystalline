package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MarvinJWendt/testza"
)

// TestCLIGenerates runs the command the way a go:generate directive would.
func TestCLIGenerates(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the command")
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, "crystalline")

	build := exec.Command("go", "build", "-o", binary, ".")
	out, err := build.CombinedOutput()
	testza.AssertNoError(t, err, string(out))

	repo, err := filepath.Abs("..")
	testza.AssertNoError(t, err)
	repo = filepath.Dir(repo)

	generated := filepath.Join(dir, "gen")

	cmd := exec.Command(binary,
		"-app", "app",
		"-dir", repo,
		"-out", filepath.Join(dir, "dist"),
		"-go-out", filepath.Join(generated, "crystalline_gen.go"),
		"-go-package", "gen",
		"-go-import-path", "example.com/gen",
		"./testdata/bindings",
	)

	out, err = cmd.CombinedOutput()
	testza.AssertNoError(t, err, string(out))

	for _, name := range []string{
		filepath.Join(dir, "dist", "crystalline.js"),
		filepath.Join(dir, "dist", "crystalline.d.ts"),
		filepath.Join(generated, "crystalline_gen.go"),
	} {
		content, err := os.ReadFile(name)
		testza.AssertNoError(t, err, "expected "+name)
		testza.AssertTrue(t, len(content) > 0, name+" is empty")
	}

	declarations, err := os.ReadFile(filepath.Join(dir, "dist", "crystalline.d.ts"))
	testza.AssertNoError(t, err)
	testza.AssertTrue(t, strings.Contains(string(declarations), "namespace sample"), string(declarations))
}

func TestCLIRequiresApp(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the command")
	}

	dir := t.TempDir()
	binary := filepath.Join(dir, "crystalline")

	build := exec.Command("go", "build", "-o", binary, ".")
	out, err := build.CombinedOutput()
	testza.AssertNoError(t, err, string(out))

	combined, err := exec.Command(binary).CombinedOutput()
	testza.AssertNotNil(t, err, "missing -app must fail")
	testza.AssertTrue(t, strings.Contains(string(combined), "-app is required"), string(combined))
}
