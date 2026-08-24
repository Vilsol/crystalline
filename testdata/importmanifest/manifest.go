// Package importmanifest declares an object Go reaches into JavaScript for.
package importmanifest

import (
	"github.com/Vilsol/crystalline/bind"
	"github.com/Vilsol/crystalline/testdata/importer"
)

//crystalline:exports
func Exports(r bind.Registry) {
	r.Import(&importer.Local, bind.At("localStorage"))

	r.Import(&importer.Remote, bind.At("remote"))

	r.Import(&importer.Awaited, bind.At("remote"), bind.AsPromise("Fetch"))

	r.Func(importer.Roundtrip)
	r.Func(importer.FetchAwaited, bind.AsPromise())
	r.Func(importer.Fetch)
}
