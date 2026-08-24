//go:build !js

package crystalline

import (
	"strconv"
	"strings"
	"testing"

	"github.com/MarvinJWendt/testza"
)

func TestManifestIsRead(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/manifest", "./testdata/sample"))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	testza.AssertEqual(t, 1, len(declarations.Manifests), "the annotated manifest must be found")

	found := make([]string, 0, len(declarations.entries))
	for _, entry := range declarations.entries {
		found = append(found, entry.String())
	}

	joined := strings.Join(found, "\n")

	for _, expected := range []string{
		"func sample.Basic",
		"func sample.FooBar promise",
		// A value carries no package of its own, so its type names one: this
		// belongs in sample even though manifest declared it.
		"value sample.Prototype",
		// Nothing names a package here, so the manifest's own is the fallback.
		"value manifest.Lookup map[string]int",
		"value data.Index map[uint32]string",
		"value data.Name string",
		"ignore sample.FnSample.Three",
		"promise sample.FnSample.Two",
	} {
		testza.AssertTrue(t, strings.Contains(joined, expected),
			"missing "+expected+" in:\n"+joined)
	}
}

func TestManifestRejectsComputedName(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/badmanifest"))

	_, err := g.Declarations()
	testza.AssertNotNil(t, err)
	testza.AssertTrue(t, strings.Contains(err.Error(), "literal name"), err)
}

func TestNamespaceCollisionIsAnError(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/collide/..."))

	_, err := g.Declarations()
	testza.AssertNotNil(t, err, "two packages claiming one namespace must be reported")
	testza.AssertTrue(t, strings.Contains(err.Error(), "InNamespace"),
		"the error must say how to fix it, got: "+errText(err))
}

func TestExportDirectiveIsShorthand(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/directive"))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	found := make([]string, 0, len(declarations.entries))
	for _, entry := range declarations.entries {
		found = append(found, entry.String())
	}

	joined := strings.Join(found, "\n")

	testza.AssertTrue(t, strings.Contains(joined, "func directive.Owned"), joined)
	testza.AssertTrue(t, strings.Contains(joined, "func directive.Slow promise"), joined)
	testza.AssertFalse(t, strings.Contains(joined, "Unmarked"),
		"an unmarked function must not be exposed:\n"+joined)
}

func errText(err error) string {
	if err == nil {
		return "<nil>"
	}

	return err.Error()
}

// TestInterfacesLandInTheirOwnNamespace pins that a type is declared where it
// is defined, not where it happened to be reached from. The reflect path does
// this, and a mismatch produces declarations that reference a namespace which
// never declares the type.
func TestInterfacesLandInTheirOwnNamespace(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/crosspkg/manifest"))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	out, err := g.Build(declarations)
	testza.AssertNoError(t, err)

	// Which namespace block it falls in, rather than which line: the block also
	// holds whatever else inner declares.
	opens := strings.Index(out.TypeScript, "export declare namespace inner {")
	closes := strings.Index(out.TypeScript[opens:], "\nexport declare namespace ")

	testza.AssertNotEqual(t, -1, opens, "inner must have a namespace:\n"+out.TypeScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript[opens:opens+closes], "interface Payload {"),
		"Payload must be declared in its own namespace:\n"+out.TypeScript)
	testza.AssertTrue(t, strings.Contains(out.TypeScript, "function Make(): inner.Payload;"),
		"the reference must resolve:\n"+out.TypeScript)
}

// TestGeneratingIntoTheManifestPackage pins that the generated file can live
// next to the manifest, which is the layout a consumer reaches for first.
func TestGeneratingIntoTheManifestPackage(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/crosspkg/manifest"))

	declarations, err := g.Declarations()
	testza.AssertNoError(t, err)

	self := "github.com/Vilsol/crystalline/testdata/crosspkg/manifest"

	bindings, err := g.BuildGo(declarations, "manifest", self)
	testza.AssertNoError(t, err)

	testza.AssertTrue(t, strings.Contains(bindings.Source, "\tExports(crystallineRegistry{})"),
		"the manifest must be called unqualified:\n"+bindings.Source)
	testza.AssertFalse(t, strings.Contains(bindings.Source, strconv.Quote(self)),
		"generated code must not import its own package:\n"+bindings.Source)
}

// TestManifestRejectsPromiseOnAValue pins that asking for a promise where one
// cannot exist is refused.
//
// A promise is a way of returning, and a value does not return. The option was
// recorded and then read only for function types, so it compiled, generated and
// did nothing, in a project whose whole claim is that nothing is dropped
// silently.
func TestManifestRejectsPromiseOnAValue(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/badpromise"))

	_, err := g.Declarations()
	testza.AssertNotNil(t, err, "a promise on a non-function value must be refused")
	testza.AssertTrue(t, strings.Contains(errText(err), "AsPromise"),
		"the error must name the option, got: "+errText(err))
}

// TestNamespacesMustBeIdentifiers pins that a namespace which cannot be a
// JavaScript identifier is refused.
//
// The name went straight into "export let <name>", so bind.InNamespace("my-api")
// produced a module that does not parse — found by whoever imported it rather
// than by whoever wrote it.
func TestNamespacesMustBeIdentifiers(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/badnamespace"))

	_, err := g.Declarations()
	testza.AssertNotNil(t, err, "a namespace that cannot be an identifier must be refused")
	testza.AssertTrue(t, strings.Contains(errText(err), "my-api"),
		"the error must name it, got: "+errText(err))
}

// A type's option on a function used to be folded into the resolved options and
// then read only where it applied, so asking for it compiled, generated and did
// nothing.
func TestTypeOptionOnAFunctionIsAnError(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/badoption", "./testdata/sample"))

	_, err := g.Declarations()
	testza.AssertNotNil(t, err)
	testza.AssertTrue(t, strings.Contains(errText(err), "bind.Plain() says how a type crosses, so it belongs on r.Type"),
		"the error must name the option and where it belongs, got: "+errText(err))
}

// The type a mapping is declared on and the type its functions describe are two
// statements of one fact. Nothing compared them before the mapping moved onto
// r.Type, because the functions were the only statement there was.
func TestMappingMustDescribeTheTypeItIsDeclaredOn(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/badmarshal", "./testdata/marshal", "./testdata/sample"))

	_, err := g.Declarations()
	testza.AssertNotNil(t, err)
	testza.AssertTrue(t, strings.Contains(errText(err), "ColourToHex takes"),
		"the error must name the function and what it takes, got: "+errText(err))
	testza.AssertTrue(t, strings.Contains(errText(err), "the mapping was declared on"),
		"the error must say the declaration disagrees, got: "+errText(err))
}

// A type does not return, so the bare bind.AsPromise() has nothing to apply to.
func TestBarePromiseOnATypeIsAnError(t *testing.T) {
	g := NewGenerator("app")
	testza.AssertNoError(t, g.Load(".", "./testdata/badtypepromise", "./testdata/sample"))

	_, err := g.Declarations()
	testza.AssertNotNil(t, err)
	testza.AssertTrue(t, strings.Contains(errText(err), "is not a function; name the methods that return a promise"),
		"the error must say how to name the methods instead, got: "+errText(err))
}
