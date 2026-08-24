// Command crystalline generates JavaScript bindings and TypeScript
// declarations from Go source.
//
// It is meant to be driven from a go:generate directive next to the manifest:
//
//	//go:generate crystalline -app myapp -out ./dist ./...
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Vilsol/crystalline"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "crystalline:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		app          = flag.String("app", "", "name the bindings are published under (required)")
		dir          = flag.String("dir", ".", "directory the patterns are resolved against")
		out          = flag.String("out", "dist", "directory the JavaScript and declarations are written to")
		goOut        = flag.String("go-out", "", "file the Go bindings are written to (defaults to crystalline_gen.go beside the manifest)")
		goPackage    = flag.String("go-package", "", "package name for the generated Go file (defaults to the manifest's package)")
		goImportPath = flag.String("go-import-path", "", "import path of the generated Go file (defaults to the manifest's package)")
		jsOut        = flag.String("js-out", "", "file the JavaScript module is written to (defaults to <out>/crystalline.js)")
		tsOut        = flag.String("ts-out", "", "file the declarations are written to (defaults to <out>/crystalline.d.ts)")
		banner       = flag.String("banner", "", "text prepended to the generated JavaScript and declarations")
		profile      = flag.Bool("profile", false, "count and time every call, reported by stats() on the generated module")
		watching     = flag.Bool("watch", false, "regenerate whenever a Go file under -dir changes")
		quote        = flag.String("quote", "'", "quote character used in the generated JavaScript")
		trailing     = flag.Bool("trailing-comma", false, "emit trailing commas in the generated JavaScript")
	)

	flag.Parse()

	if *app == "" {
		flag.Usage()

		return fmt.Errorf("-app is required")
	}

	patterns := flag.Args()
	if len(patterns) == 0 {
		patterns = []string{"./..."}
	}

	options := []crystalline.GeneratorOption{crystalline.WithQuoteStyle(*quote)}
	if *trailing {
		options = append(options, crystalline.WithTrailingComma())
	}

	if *banner != "" {
		options = append(options, crystalline.WithBanner(*banner))
	}

	if *profile {
		options = append(options, crystalline.WithProfiling())
	}

	// A fresh generator each time: it holds the loaded packages, and the point
	// of regenerating is that they have changed.
	generate := func() error {
		generator := crystalline.NewGenerator(*app, options...)

		if err := generator.Load(*dir, patterns...); err != nil {
			return err
		}

		return build(generator, patterns, *out, *jsOut, *tsOut, *goOut, *goPackage, *goImportPath)
	}

	if *watching {
		return watch(*dir, generate)
	}

	return generate()
}

// build runs one generation, from loaded packages to written files.
func build(generator *crystalline.Generator, patterns []string, out string, jsOut string, tsOut string, goOut string, goPackage string, goImportPath string) error {
	declarations, err := generator.Declarations()
	if err != nil {
		return err
	}

	if declarations.Empty() {
		return fmt.Errorf("nothing to generate: no //crystalline:exports manifest or //crystalline:export directive found in %v", patterns)
	}

	rendered, err := generator.Build(declarations)
	if err != nil {
		return err
	}

	for _, skipped := range rendered.Skipped {
		fmt.Fprintln(os.Stderr, "crystalline: skipped", skipped)
	}

	for _, warning := range rendered.Warnings {
		fmt.Fprintln(os.Stderr, "crystalline: warning", warning)
	}

	if err := rendered.WriteFiles(orDefault(jsOut, out, "crystalline.js"), orDefault(tsOut, out, "crystalline.d.ts")); err != nil {
		return err
	}

	return writeGo(generator, declarations, goOut, goPackage, goImportPath)
}

// orDefault resolves an output path, falling back to a default name inside the
// output directory.
func orDefault(target string, dir string, name string) string {
	if target != "" {
		return target
	}

	return filepath.Join(dir, name)
}

// writeGo renders the Go bindings, defaulting their location to sit beside the
// manifest that declared them.
func writeGo(generator *crystalline.Generator, declarations crystalline.Declarations, target string, pkgName string, importPath string) error {
	if len(declarations.Manifests) == 0 {
		return nil
	}

	manifest := declarations.Manifests[0]

	if importPath == "" {
		importPath = manifest.Package
	}

	if pkgName == "" {
		pkgName = manifest.PackageName
	}

	if target == "" {
		if manifest.Dir == "" {
			return fmt.Errorf("could not locate %s on disk, pass -go-out", manifest.Package)
		}

		target = filepath.Join(manifest.Dir, "crystalline_gen.go")
	}

	bindings, err := generator.BuildGo(declarations, pkgName, importPath)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", target, err)
	}

	return os.WriteFile(target, []byte(bindings.Source), 0o644)
}
