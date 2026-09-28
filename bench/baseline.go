//go:build js

package bench

import (
	"strconv"
	"syscall/js"

	"github.com/Vilsol/crystalline/bench/payload"
)

// The same operations, bound by hand through syscall/js and doing the same
// conversions the generated code does.
//
// These are the floor: whatever they cost is the wasm bridge itself, which no
// binding layer can avoid. The gap between a pair is what crystalline adds.
func init() {
	raw := js.Global().Get("Object").New()

	js.Global().Set("raw", raw)

	raw.Set("Noop", js.FuncOf(func(this js.Value, args []js.Value) any {
		payload.Noop()

		return nil
	}))

	raw.Set("AddInts", js.FuncOf(func(this js.Value, args []js.Value) any {
		return payload.AddInts(int(args[0].Float()), int(args[1].Float()))
	}))

	raw.Set("EchoString", js.FuncOf(func(this js.Value, args []js.Value) any {
		return payload.EchoString(args[0].String())
	}))

	raw.Set("EchoBytes", js.FuncOf(func(this js.Value, args []js.Value) any {
		data := make([]byte, args[0].Get("length").Int())
		js.CopyBytesToGo(data, args[0])

		echoed := payload.EchoBytes(data)

		out := js.Global().Get("Uint8Array").New(len(echoed))
		js.CopyBytesToJS(out, echoed)

		return out
	}))

	raw.Set("SumFloats", js.FuncOf(func(this js.Value, args []js.Value) any {
		values := make([]float64, args[0].Length())
		for i := range values {
			values[i] = args[0].Index(i).Float()
		}

		return payload.SumFloats(values)
	}))

	raw.Set("MakeInts", js.FuncOf(func(this js.Value, args []js.Value) any {
		made := payload.MakeInts(int(args[0].Float()))

		out := make([]any, 0, len(made))
		for _, v := range made {
			out = append(out, float64(v))
		}

		return out
	}))

	raw.Set("CountKeys", js.FuncOf(func(this js.Value, args []js.Value) any {
		keys := js.Global().Get("Object").Call("keys", args[0])

		index := make(map[string]int, keys.Length())
		for i := range keys.Length() {
			key := keys.Index(i).String()
			index[key] = int(args[0].Get(key).Float())
		}

		return payload.CountKeys(index)
	}))

	raw.Set("MakeMap", js.FuncOf(func(this js.Value, args []js.Value) any {
		made := payload.MakeMap(int(args[0].Float()))

		out := make(map[string]any, len(made))
		for key, value := range made {
			out[key] = float64(value)
		}

		return out
	}))

	raw.Set("MakePoints", js.FuncOf(func(this js.Value, args []js.Value) any {
		made := payload.MakePoints(int(args[0].Float()))

		out := make([]any, 0, len(made))
		for _, point := range made {
			out = append(out, map[string]any{
				"X":      point.X,
				"Y":      point.Y,
				"Label":  point.Label,
				"Origin": map[string]any{"X": point.Origin.X, "Y": point.Origin.Y},
			})
		}

		return out
	}))

	raw.Set("Rounds", js.FuncOf(func(this js.Value, args []js.Value) any {
		n := int(args[0].Float())

		return js.Global().Get("Promise").Call("resolve", payload.Rounds(n))
	}))

	raw.Set("MayFail", js.FuncOf(func(this js.Value, args []js.Value) any {
		value, err := payload.MayFail(args[0].Bool())
		if err != nil {
			return map[string]any{"ok": false, "error": err.Error()}
		}

		return map[string]any{"ok": true, "value": value}
	}))

	raw.Set("MakeRecord", js.FuncOf(func(this js.Value, args []js.Value) any {
		made := payload.MakeRecord()

		floats := func(values []float64) []any {
			out := make([]any, 0, len(values))
			for _, value := range values {
				out = append(out, value)
			}

			return out
		}

		line := func(l payload.Line) map[string]any {
			return map[string]any{"Label": l.Label, "Values": floats(l.Values)}
		}

		tags := make([]any, 0, len(made.Tags))
		for _, tag := range made.Tags {
			tags = append(tags, tag)
		}

		lines := make([]any, 0, len(made.Lines))
		for _, l := range made.Lines {
			lines = append(lines, line(l))
		}

		grid := make([]any, 0, len(made.Grid))
		for _, row := range made.Grid {
			grid = append(grid, floats(row))
		}

		totals := make(map[string]any, len(made.Totals))
		for key, value := range made.Totals {
			totals[key] = value
		}

		return map[string]any{
			"ID":     made.ID,
			"Name":   made.Name,
			"Score":  made.Score,
			"Active": made.Active,
			"Tags":   tags,
			"Lines":  lines,
			"Grid":   grid,
			"Totals": totals,
			"Main":   line(made.Main),
			"Parent": line(*made.Parent),
		}
	}))

	// Named so the benchmark can say what it is comparing against.
	raw.Set("name", strconv.Quote("hand-written syscall/js"))
}
