package sheet

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseJSONObjectCell parses a JSON-in-cell value into a string-keyed object,
// per DESIGN.md decision 1 (KV-map cells are JSON-in-cell, converter-validated).
// It is used for the fixed_axes, attestation required_constraints, and
// excluded_combinations axes cells. The cell must be a JSON object; a non-object
// or malformed value returns a wrapped error naming the offending column so the
// author can locate it.
//
// Values are decoded with UseNumber and left as their JSON types (string, an
// array of strings for the multi-value excluded-axes shape the schema allows, or
// json.Number): the caller decides which shapes are legal for its target field.
func parseJSONObjectCell(column, raw string) (map[string]any, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, fmt.Errorf("column %q: invalid JSON object cell %q: %w", column, raw, err)
	}
	if obj == nil {
		return nil, fmt.Errorf("column %q: JSON object cell %q decoded to null", column, raw)
	}
	return obj, nil
}
