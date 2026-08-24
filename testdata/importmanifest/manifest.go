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

	r.Import(&importer.Renamed, bind.At("webish"),
		bind.Called("GetItem", "getItem"), bind.Called("SetItem", "setItem"))

	r.Func(importer.Roundtrip)
	r.Func(importer.Fetched)
	r.Func(importer.FetchAwaited, bind.AsPromise())
	r.Func(importer.Fetch)
}
