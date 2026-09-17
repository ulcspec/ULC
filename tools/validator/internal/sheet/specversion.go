package sheet

import (
	"fmt"
	"strconv"
	"strings"
)

// SpecVersion is the current released ULC specification version stamped into
// newly converted records. It is a compile-time converter policy, independent
// of the CLI build version, so source and packaged builds stamp the same value.
const SpecVersion = "1.10.0"

func specificationVersionGreater(candidate, bound string) (bool, error) {
	parse := func(value string) ([3]int, error) {
		var parsed [3]int
		parts := strings.Split(value, ".")
		if len(parts) != len(parsed) {
			return parsed, fmt.Errorf("version %q is not three components", value)
		}
		for i, part := range parts {
			if part == "" || strings.Trim(part, "0123456789") != "" {
				return parsed, fmt.Errorf("version %q has a non-integer component", value)
			}
			n, err := strconv.Atoi(part)
			if err != nil {
				return parsed, fmt.Errorf("version %q component %q: %w", value, part, err)
			}
			parsed[i] = n
		}
		return parsed, nil
	}

	left, err := parse(candidate)
	if err != nil {
		return false, err
	}
	right, err := parse(bound)
	if err != nil {
		return false, err
	}
	for i := range left {
		if left[i] != right[i] {
			return left[i] > right[i], nil
		}
	}
	return false, nil
}
