package crystalline

import (
	"context"
	"strings"
)

var stepKey struct{}

func withContextStep(ctx context.Context, step string) context.Context {
	value := ctx.Value(stepKey)
	var steps []string
	if value == nil {
		steps = make([]string, 0)
	} else {
		steps = value.([]string)
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
