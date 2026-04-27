package codinglane

import "slices"

func hasDuplicates(values []string) bool {
	seen := make([]string, 0, len(values))
	for _, value := range values {
		if slices.Contains(seen, value) {
			return true
		}
		seen = append(seen, value)
	}
	return false
}
