package crystalline

import (
	"context"
	"strings"
)

type stepKeyType struct{}

var stepKey stepKeyType

func withContextStep(ctx context.Context, step string) context.Context {
	var steps []string
	if value, ok := ctx.Value(stepKey).([]string); ok {
		// Copy instead of appending in place: sibling contexts derived from the
		// same parent share its backing array and would otherwise overwrite
		// each other's step.
		steps = make([]string, len(value), len(value)+1)
		copy(steps, value)
	}

	return context.WithValue(ctx, stepKey, append(steps, step))
}

func getContextSteps(ctx context.Context) string {
	value, ok := ctx.Value(stepKey).([]string)
	if !ok {
		return "."
	}
	return strings.Join(value, ".")
}
