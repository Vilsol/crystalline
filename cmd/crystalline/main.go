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

	generator := crystalline.NewGenerator(*app, options...)

	if err := generator.Load(*dir, patterns...); err != nil {
		return err
	}

	declarations, err := generator.Declarations()
	if err != nil {
		return err
	}

	if len(declarations.Manifests) == 0 && len(declarations.Entries) == 0 {
		return fmt.Errorf("nothing to generate: no //crystalline:exports manifest or //crystalline:export directive found in %v", patterns)
	}

	rendered, err := generator.Build(declarations)
	if err != nil {
		return err
	}

	if err := rendered.WriteFiles(filepath.Join(*out, "crystalline.js"), filepath.Join(*out, "crystalline.d.ts")); err != nil {
		return err
	}

	return writeGo(generator, declarations, *goOut, *goPackage, *goImportPath)
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

	for _, skipped := range bindings.Skipped {
		fmt.Fprintln(os.Stderr, "crystalline: skipped", skipped)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("creating directory for %s: %w", target, err)
	}

	return os.WriteFile(target, []byte(bindings.Source), 0o644)
}
