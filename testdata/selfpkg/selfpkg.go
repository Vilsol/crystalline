// Package selfpkg declares both the manifest and the types it exposes, which is
// the layout the export directive implies.
package selfpkg

// Settings is declared in the package the bindings are generated into.
type Settings struct {
	Name string
}

//crystalline:export
func Configure() Settings { return Settings{Name: "set"} }
