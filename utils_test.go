package crystalline

import (
	"reflect"
	"testing"

	"github.com/MarvinJWendt/testza"
)

func TestFindFunctionIsCached(t *testing.T) {
	pointer := reflect.ValueOf(SomeFunc).Pointer()

	first := findFunction(pointer)
	testza.AssertNotNil(t, first)
	testza.AssertEqual(t, "SomeFunc", first.Name.Name)

	second := findFunction(pointer)

	// A cache hit must return the very same AST node, not a freshly parsed one.
	testza.AssertTrue(t, first == second, "findFunction re-parsed the source file")
}

func TestFindFunctionCachesMisses(t *testing.T) {
	// Resolving the same pointer twice must stay consistent, including when no
	// declaration is found.
	pointer := reflect.ValueOf(ErrorFunc).Pointer()
	testza.AssertEqual(t, findFunction(pointer), findFunction(pointer))
}
