package crystalline

import "sort"

func SortedKeys[T any](data map[string]T) []string {
	result := make([]string, len(data))
	i := 0
	for s := range data {
		result[i] = s
		i++
	}
	sort.Strings(result)
	return result
}
