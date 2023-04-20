package crystalline

import (
	"context"
	"strings"
)

var stepKey struct{}

func withContextStep(ctx context.Context, step string) context.Context {
	var steps []string
	if value := ctx.Value(stepKey); value != nil {
		steps = value.([]string)
	} else {
		steps = make([]string, 0)
	}

	return context.WithValue(ctx, stepKey, append(steps, step))
}

func getContextSteps(ctx context.Context) string {
	value := ctx.Value(stepKey)
	if value == nil {
		return "."
	}
	return strings.Join(value.([]string), ".")
}
