package sheet

import (
	"fmt"
	"math"
	"strconv"
)

// assembleApplicability builds the applicability block (applicable_catalog_pattern,
// fixed_axes, covered_axes, excluded_combinations, applicable_sku_count_estimate)
// for the multi-value patterns B and D, joining the covered_axes,
// cct_multipliers, and excluded_combinations long sheets against the record. The
// applicable_catalog_pattern, fixed_axes (JSON-in-cell), and
// applicable_sku_count_estimate come from the master row.
//
// covered_axes rows are grouped by (record_id, axis_key) into one CoveredAxis per
// axis: values[] collects each row's value in file order; the axis-level rationale
// is the first non-blank rationale and a conflicting non-identical rationale for
// the same axis is a hard error (DESIGN.md decision 2); the derivation block is
// assembled from the derivation_* columns and, for a CCT axis with a
// cct_multipliers table, gains a multiplier_table.
func assembleApplicability(wb Workbook, id string, master Row) (map[string]any, error) {
	app := map[string]any{}
	if v, ok := master["applicable_catalog_pattern"]; ok {
		app["applicable_catalog_pattern"] = v
	}
	if v, ok := master["fixed_axes"]; ok {
		fixed, err := parseJSONObjectCell("fixed_axes", v)
		if err != nil {
			return nil, err
		}
		app["fixed_axes"] = fixed
	}
	if v, ok := master["applicable_sku_count_estimate"]; ok {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("column %q: invalid integer %q: %w", "applicable_sku_count_estimate", v, err)
		}
		app["applicable_sku_count_estimate"] = n
	}

	multipliers, multOrder, err := readMultiplierTable(wb, id)
	if err != nil {
		return nil, err
	}

	coveredAxes, err := assembleCoveredAxes(wb, id, multipliers, multOrder)
	if err != nil {
		return nil, err
	}
	if len(coveredAxes) > 0 {
		app["covered_axes"] = coveredAxes
	}

	excluded, err := assembleExcludedCombinations(wb, id)
	if err != nil {
		return nil, err
	}
	if len(excluded) > 0 {
		app["excluded_combinations"] = excluded
	}

	return app, nil
}

// coveredAxisAccumulator collects the rows of one covered axis across the
// covered_axes sheet, preserving value file order and tracking the single
// axis-level rationale and derivation columns.
type coveredAxisAccumulator struct {
	axisKey   string
	values    []any
	rationale string
	// derivation columns (first non-blank wins; the design treats them as
	// axis-level, like rationale).
	derivMethod   string
	derivBaseline string
	derivRef      string
	derivNote     string
	rateValue     string
	rateUnit      string
	excludes      []any
}

// assembleCoveredAxes groups the covered_axes sheet rows by axis_key into a map
// of CoveredAxis objects. For the axis whose derivation method is the CCT
// multiplier, the multiplier table (from cct_multipliers) is attached as
// derivation.multiplier_table. axisOrder preserves first-seen order so the output
// is deterministic.
func assembleCoveredAxes(wb Workbook, id string, multipliers map[string]float64, multOrder []string) (map[string]any, error) {
	rows := wb.RowsFor("covered_axes", id)
	if len(rows) == 0 {
		return nil, nil
	}

	accs := map[string]*coveredAxisAccumulator{}
	order := []string{}
	for i, row := range rows {
		axisKey := row["axis_key"]
		if axisKey == "" {
			return nil, fmt.Errorf("covered_axes row %d for %q: missing axis_key", i+1, id)
		}
		acc, ok := accs[axisKey]
		if !ok {
			acc = &coveredAxisAccumulator{axisKey: axisKey}
			accs[axisKey] = acc
			order = append(order, axisKey)
		}
		if v, ok := row["value"]; ok {
			acc.values = append(acc.values, v)
		}
		if r, ok := row["rationale"]; ok {
			if acc.rationale == "" {
				acc.rationale = r
			} else if acc.rationale != r {
				return nil, fmt.Errorf("covered_axes axis %q for %q: conflicting non-identical rationales (%q vs %q); the rationale is axis-level and must be identical across the axis's rows", axisKey, id, acc.rationale, r)
			}
		}
		firstNonBlank(&acc.derivMethod, row["derivation_method"])
		firstNonBlank(&acc.derivBaseline, row["baseline_axis_value"])
		firstNonBlank(&acc.derivRef, row["reference"])
		firstNonBlank(&acc.derivNote, row["note"])
		firstNonBlank(&acc.rateValue, row["linear_rate_value"])
		firstNonBlank(&acc.rateUnit, row["linear_rate_unit"])
		if ex, ok := row["excludes"]; ok {
			acc.excludes = append(acc.excludes, ex)
		}
	}

	out := map[string]any{}
	for _, axisKey := range order {
		acc := accs[axisKey]
		if acc.rationale == "" {
			return nil, fmt.Errorf("covered_axes axis %q for %q: missing rationale (required on every CoveredAxis)", axisKey, id)
		}
		axis := map[string]any{
			"values":    acc.values,
			"rationale": acc.rationale,
		}
		if len(acc.excludes) > 0 {
			axis["excludes"] = acc.excludes
		}

		deriv, err := acc.derivation(axisKey, id, multipliers, multOrder)
		if err != nil {
			return nil, err
		}
		if deriv != nil {
			axis["derivation"] = deriv
		}
		out[axisKey] = axis
	}
	return out, nil
}

// derivation builds the CoveredAxis.derivation block from the accumulator's
// derivation columns, or returns nil when no derivation method is declared. When
// the method is the CCT multiplier method and a cct_multipliers table exists, the
// table is attached as multiplier_table.
func (acc *coveredAxisAccumulator) derivation(axisKey, id string, multipliers map[string]float64, multOrder []string) (map[string]any, error) {
	if acc.derivMethod == "" {
		if len(multipliers) > 0 && axisKey == "cct" {
			return nil, fmt.Errorf("covered_axes axis %q for %q: a cct_multipliers table is present but the axis declares no derivation_method", axisKey, id)
		}
		return nil, nil
	}
	deriv := map[string]any{"method": acc.derivMethod}
	if acc.derivBaseline != "" {
		deriv["baseline_axis_value"] = acc.derivBaseline
	}
	if acc.derivRef != "" {
		deriv["reference"] = acc.derivRef
	}
	if acc.derivNote != "" {
		deriv["note"] = acc.derivNote
	}
	if acc.rateValue != "" {
		rv, err := parseNumber(acc.rateValue)
		if err != nil {
			return nil, fmt.Errorf("covered_axes axis %q for %q: invalid linear_rate_value: %w", axisKey, id, err)
		}
		rate := map[string]any{"value": rv}
		if acc.rateUnit != "" {
			rate["unit"] = acc.rateUnit
		}
		deriv["linear_rate"] = rate
	}
	// Attach the CCT multiplier table to its axis. The table key/value order is
	// deterministic via multOrder.
	if axisKey == "cct" && len(multipliers) > 0 {
		table := map[string]any{}
		for _, cct := range multOrder {
			table[cct] = numberLeaf(multipliers[cct])
		}
		deriv["multiplier_table"] = table
	}
	return deriv, nil
}

// readMultiplierTable reads the cct_multipliers sheet into a {cct -> multiplier}
// map plus the file-order CCT key list. A missing sheet yields an empty table.
func readMultiplierTable(wb Workbook, id string) (map[string]float64, []string, error) {
	rows := wb.RowsFor("cct_multipliers", id)
	if len(rows) == 0 {
		return nil, nil, nil
	}
	table := map[string]float64{}
	order := []string{}
	for i, row := range rows {
		cct := row["cct"]
		if cct == "" {
			return nil, nil, fmt.Errorf("cct_multipliers row %d for %q: missing cct", i+1, id)
		}
		raw := row["multiplier"]
		if raw == "" {
			return nil, nil, fmt.Errorf("cct_multipliers row %q for %q: missing multiplier", cct, id)
		}
		m, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, nil, fmt.Errorf("cct_multipliers row %q for %q: invalid multiplier %q: %w", cct, id, raw, err)
		}
		if _, dup := table[cct]; dup {
			return nil, nil, fmt.Errorf("cct_multipliers for %q: duplicate cct %q", id, cct)
		}
		table[cct] = m
		order = append(order, cct)
	}
	return table, order, nil
}

// assembleExcludedCombinations builds applicability.excluded_combinations[] from
// the excluded_combinations sheet. Each row carries a JSON-in-cell `axes` object
// (values may be a string or an array of strings, both allowed by the schema) and
// a `reason` string.
func assembleExcludedCombinations(wb Workbook, id string) ([]any, error) {
	rows := wb.RowsFor("excluded_combinations", id)
	if len(rows) == 0 {
		return nil, nil
	}
	out := []any{}
	for i, row := range rows {
		axesCell := row["axes"]
		if axesCell == "" {
			return nil, fmt.Errorf("excluded_combinations row %d for %q: missing axes", i+1, id)
		}
		axes, err := parseJSONObjectCell("excluded_combinations.axes", axesCell)
		if err != nil {
			return nil, err
		}
		reason := row["reason"]
		if reason == "" {
			return nil, fmt.Errorf("excluded_combinations row %d for %q: missing reason", i+1, id)
		}
		out = append(out, map[string]any{
			"axes":   axes,
			"reason": reason,
		})
	}
	return out, nil
}

// generateDeclaredByCCT generates photometry.declared_by_cct[] from the CCT
// multiplier table and the measured baseline total luminous flux. For each CCT,
// the lumens value is round(multiplier * baseline). The baseline CCT row (the one
// whose value equals the derivation's baseline_axis_value) carries the measured
// provenance and its attestation_ref; every other CCT carries scaled provenance
// with method=scaled, extension_method=cct_multiplier, and base_attestation_ref
// pointing at the same LM-79 attestation. The shape mirrors the Selux example.
//
// multOrder preserves the cct_multipliers file order so the generated array is
// deterministic. baseLM79 is the record's single LM-79 attestation id (the
// measured anchor). baselineCCT names the tested baseline CCT.
func generateDeclaredByCCT(multipliers map[string]float64, multOrder []string, baseline float64, baselineCCT, baseLM79 string) []any {
	out := []any{}
	for _, cct := range multOrder {
		mult := multipliers[cct]
		lumens := math.Round(mult * baseline)
		entry := map[string]any{"cct": cct}
		if cct == baselineCCT {
			// The baseline CCT row carries the actual measured flux, not a
			// multiplier-scaled value: the baseline multiplier should be 1.0, but
			// trusting the measured baseline keeps a mis-authored non-1.0 baseline
			// multiplier from turning a measured value into a derived one.
			entry["lumens"] = map[string]any{
				"value":      numberLeaf(baseline),
				"unit":       "lm",
				"value_type": "measured",
				"provenance": map[string]any{
					"source":          "ies",
					"method":          "extracted",
					"attestation_ref": baseLM79,
				},
			}
		} else {
			entry["lumens"] = map[string]any{
				"value":      numberLeaf(lumens),
				"unit":       "lm",
				"value_type": "rated",
				"provenance": map[string]any{
					"source":               "datasheet_pdf",
					"method":               "scaled",
					"base_attestation_ref": baseLM79,
					"extension_method":     "cct_multiplier",
				},
			}
		}
		out = append(out, entry)
	}
	return out
}

// firstNonBlank assigns v to *dst when *dst is still empty and v is non-blank, so
// the first non-blank cell across an axis's rows wins (axis-level derivation
// columns, matching the rationale rule).
func firstNonBlank(dst *string, v string) {
	if *dst == "" && v != "" {
		*dst = v
	}
}

// baselineFlux reads the measured baseline total luminous flux off the assembled
// record (photometry.total_luminous_flux_lm.value), the value the CCT multiplier
// table scales. It accepts both int64 and float64 (parseNumber emits int64 for
// integral cells) and reports false when absent.
func baselineFlux(rec map[string]any) (float64, bool) {
	v, ok := getPath(rec, "photometry.total_luminous_flux_lm.value")
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	default:
		return 0, false
	}
}

// coveredAxisBaseline returns the baseline_axis_value of a covered axis's
// derivation block, or "" when the axis or its derivation is absent. It is used
// to mark the tested baseline CCT (which keeps measured provenance) and the
// tested baseline length (which the generated declared_by_length table omits).
func coveredAxisBaseline(app map[string]any, axisKey string) string {
	axis, ok := coveredAxis(app, axisKey)
	if !ok {
		return ""
	}
	deriv, ok := axis["derivation"].(map[string]any)
	if !ok {
		return ""
	}
	b, _ := deriv["baseline_axis_value"].(string)
	return b
}

// coveredAxisValues returns the values[] of a covered axis as a string slice, or
// nil when the axis is absent.
func coveredAxisValues(app map[string]any, axisKey string) []string {
	axis, ok := coveredAxis(app, axisKey)
	if !ok {
		return nil
	}
	raw, ok := axis["values"].([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// coveredAxis returns the named CoveredAxis object from the applicability block.
func coveredAxis(app map[string]any, axisKey string) (map[string]any, bool) {
	covered, ok := app["covered_axes"].(map[string]any)
	if !ok {
		return nil, false
	}
	axis, ok := covered[axisKey].(map[string]any)
	return axis, ok
}
