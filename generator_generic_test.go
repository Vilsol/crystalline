//go:build !js

package crystalline

import (
	"strings"
	"testing"

	"github.com/MarvinJWendt/testza"
)

// TestGenericInstantiationsStayDistinct pins the case the reflect path got
// wrong: it flattened a generic name by cutting at the bracket, so Pair[int]
// and Pair[string] collided and one silently replaced the other.
func TestGenericInstantiationsStayDistinct(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/genericmanifest"))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	out, err := g.Build(declarations)
	testza.AssertNoError(t, err)

	// Naming both after the bare generic would produce two interfaces of one
	// name, which is not valid TypeScript and silently ambiguous to read.
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "interface PairOfString {"), out.TypeScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "interface PairOfInt {"), out.TypeScript)
	testza.AssertEqual(t, 0, strings.Count(out.TypeScript, "interface Pair {"),
		"the bare generic name must not be used:\n"+out.TypeScript)

	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function Strings(): generic.PairOfString;"), out.TypeScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function Numbers(): generic.PairOfInt;"), out.TypeScript)
}

// TestGenericBindingsCompile pins that the emitted Go for a generic type is
// valid, which is where instantiation names leak into identifiers.
//
// It checks the shape only. Whether the source really compiles is settled by
// TestGeneratedBindingsWork, which builds and runs both instantiations: this
// test passing while the generated file did not compile was possible until the
// generic fixtures joined the bindings manifest.
func TestGenericBindingsCompile(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/genericmanifest"))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	bindings, err := g.BuildGo(declarations, WithPackageName("gen"), WithImportPath("example.com/gen"))
	testza.AssertNoError(t, err, "generic bindings must render as valid Go")

	// One marshaller per instantiation, each typed to its own instantiation and
	// each carrying the package, since two packages may both declare a Pair.
	testza.AssertTrue(t, strings.Contains(bindings.Source, "func crystallineMarshalGenericPairOfString(v *generic.Pair[string])"), bindings.Source)
	testza.AssertTrue(t, strings.Contains(bindings.Source, "func crystallineMarshalGenericPairOfInt(v *generic.Pair[int])"), bindings.Source)
}
