package sheet

import (
	"fmt"
	"math"
	"strconv"
)

// inchToMM is the exact inch-to-millimeter factor used to expand an authored
// length in inches to the DualUnitLength SI leaf.
const inchToMM = 25.4

// ftPerInch converts a length in inches to feet for per-foot rate scaling.
const ftPerInch = 1.0 / 12.0

// linearRates carries the per-foot scaling rates the Pattern D
// declared_by_length table scales by. The reference (baseline) length itself is
// written directly onto the record from the records columns; the generator scales
// the covered length axis values, so it needs only the rates here.
type linearRates struct {
	lumensPerFoot float64
	hasLumens     bool
	wattsPerFoot  float64
	hasWatts      bool
}

// declaredByLengthParams bundles everything the declared_by_length assembler
// needs: the per-foot rates, the covered length axis values (in inches), the
// baseline LM-79 attestation id, and the tested-baseline length the table omits.
type declaredByLengthParams struct {
	rates        linearRates
	lengthValues []string // covered_axes length values, in inches, file order
	baseLM79     string
	baselineIn   string // the tested baseline length in inches (omitted from the generated table)
}

// assembleDeclaredByLength produces photometry.declared_by_length[]. When the
// declared_by_length sheet carries rows for the record, those rows are used
// verbatim (the authored sheet wins, DESIGN.md decision 3) and each row is
// checked against rate*length, emitting a warning when it diverges by more than
// 2%. When no sheet is present, the table is generated from the per-foot rates
// applied to every covered length value except the tested baseline. The shape
// mirrors the Vode example: each entry carries a DualUnitLength `length` plus
// scaled `lumens`, `input_power_w`, and computed `efficacy_lm_per_w`, all with
// per_foot_linear_scaling provenance.
//
// warnings receives any divergence advisories; it is the hasher's warnings slice
// so the converter surfaces them alongside missing-file notes.
func assembleDeclaredByLength(wb Workbook, id string, p declaredByLengthParams, warnings *[]string) ([]any, error) {
	rows := wb.RowsFor("declared_by_length", id)
	var out []any
	var err error
	if len(rows) > 0 {
		out, err = echoDeclaredByLength(rows, id, p, warnings)
	} else {
		out, err = generateDeclaredByLength(p, id)
	}
	if err != nil {
		return nil, err
	}
	// Every declared_by_length row is a length-scaled derivation, so it needs a
	// base attestation to anchor its provenance.base_attestation_ref. If the
	// table has any rows but no base, hard-error rather than emit derived values
	// with no traceable base measurement.
	if len(out) > 0 && p.baseLM79 == "" {
		return nil, fmt.Errorf("record %q: photometry.declared_by_length has %d length-scaled row(s) but no base attestation to anchor them; the record declares no single lm_79* attestation. Add an lm_79* attestations row, or set the length base override", id, len(out))
	}
	return out, nil
}

// echoDeclaredByLength builds the table from the authored declared_by_length
// sheet rows verbatim, warning when an authored lumens or input_power value
// diverges from the per-foot rate projection by more than 2%.
func echoDeclaredByLength(rows []Row, id string, p declaredByLengthParams, warnings *[]string) ([]any, error) {
	out := []any{}
	for i, row := range rows {
		raw := row["length_mm"]
		if raw == "" {
			return nil, fmt.Errorf("declared_by_length row %d for %q: missing length_mm", i+1, id)
		}
		lengthMM, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return nil, fmt.Errorf("declared_by_length row %d for %q: invalid length_mm %q: %w", i+1, id, raw, err)
		}

		entry := map[string]any{
			"length": lengthDualUnit(lengthMM),
		}

		lumens, hasLumens, err := authoredScaled(row, "lumens", "lm", p.baseLM79)
		if err != nil {
			return nil, fmt.Errorf("declared_by_length row %d for %q: %w", i+1, id, err)
		}
		if hasLumens {
			entry["lumens"] = lumens.obj
		}
		power, hasPower, err := authoredScaled(row, "input_power_w", "W", p.baseLM79)
		if err != nil {
			return nil, fmt.Errorf("declared_by_length row %d for %q: %w", i+1, id, err)
		}
		if hasPower {
			entry["input_power_w"] = power.obj
		}
		eff, hasEff, err := authoredEfficacy(row, p.baseLM79)
		if err != nil {
			return nil, fmt.Errorf("declared_by_length row %d for %q: %w", i+1, id, err)
		}
		if hasEff {
			entry["efficacy_lm_per_w"] = eff
		}

		// Divergence check against the per-foot projection (DESIGN.md decision 3).
		lengthFt := lengthMM / inchToMM * ftPerInch
		if hasLumens && p.rates.hasLumens {
			checkDivergence(warnings, id, lengthMM, "lumens", lumens.value, p.rates.lumensPerFoot*lengthFt)
		}
		if hasPower && p.rates.hasWatts {
			checkDivergence(warnings, id, lengthMM, "input_power_w", power.value, p.rates.wattsPerFoot*lengthFt)
		}

		out = append(out, entry)
	}
	return out, nil
}

// generateDeclaredByLength synthesizes the table from the per-foot rates applied
// to every covered length value except the tested baseline. Each generated entry
// carries scaled lumens and input power (rounded to whole lm and 1-dp W) and a
// computed efficacy, all with per_foot_linear_scaling provenance.
func generateDeclaredByLength(p declaredByLengthParams, id string) ([]any, error) {
	if !p.rates.hasLumens && !p.rates.hasWatts {
		return nil, fmt.Errorf("record %q: cannot generate declared_by_length without per-foot rates; author photometry.per_length_normalized.lumens_per_foot / watts_per_foot or provide a declared_by_length sheet", id)
	}
	out := []any{}
	for _, raw := range p.lengthValues {
		if raw == p.baselineIn {
			continue // the tested baseline length is the headline record, not a derived row
		}
		lengthIn, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			// A non-numeric covered length value (for example "any") is not a real
			// length and cannot be scaled; skip it.
			continue
		}
		// Round the computed SI leaf to kill the binary-float artifact (96 in *
		// 25.4 evaluates to 2438.3999999999996, not 2438.4); 4 dp is finer than any
		// real fixture length, so this only removes the trailing-noise digits.
		lengthMM := roundTo(lengthIn*inchToMM, 4)
		lengthFt := lengthIn * ftPerInch

		entry := map[string]any{
			"length": lengthDualUnit(lengthMM),
		}
		var lumens, power float64
		if p.rates.hasLumens {
			lumens = math.Round(p.rates.lumensPerFoot * lengthFt)
			entry["lumens"] = scaledProvNumber(numberLeaf(lumens), "lm", p.baseLM79)
		}
		if p.rates.hasWatts {
			power = roundTo(p.rates.wattsPerFoot*lengthFt, 1)
			entry["input_power_w"] = scaledProvNumber(numberLeaf(power), "W", p.baseLM79)
		}
		if p.rates.hasLumens && p.rates.hasWatts && power != 0 {
			eff := roundTo(lumens/power, 2)
			entry["efficacy_lm_per_w"] = computedProvNumber(numberLeaf(eff), "lm/W", p.baseLM79)
		}
		out = append(out, entry)
	}
	return out, nil
}

// authoredValue is a parsed authored ProvenancedNumber: the assembled object plus
// the numeric value used for the divergence check.
type authoredValue struct {
	obj   map[string]any
	value float64
}

// authoredScaled reads an authored declared_by_length quantity column
// (`<field>_value`) into a scaled ProvenancedNumber object. Returns hasValue=false
// when the column is blank.
func authoredScaled(row Row, field, unit, baseLM79 string) (authoredValue, bool, error) {
	raw, ok := row[field+"_value"]
	if !ok {
		return authoredValue{}, false, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return authoredValue{}, false, fmt.Errorf("invalid %s_value %q: %w", field, raw, err)
	}
	return authoredValue{obj: scaledProvNumber(numberLeaf(v), unit, baseLM79), value: v}, true, nil
}

// authoredEfficacy reads an authored efficacy column into a computed
// ProvenancedNumber object. Returns hasValue=false when the column is blank.
func authoredEfficacy(row Row, baseLM79 string) (map[string]any, bool, error) {
	raw, ok := row["efficacy_lm_per_w_value"]
	if !ok {
		return nil, false, nil
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, false, fmt.Errorf("invalid efficacy_lm_per_w_value %q: %w", raw, err)
	}
	return computedProvNumber(numberLeaf(v), "lm/W", baseLM79), true, nil
}

// scaledProvNumber builds a ProvenancedNumber for a per-foot-scaled quantity:
// value_type=rated, method=scaled, extension_method=per_foot_linear_scaling, with
// base_attestation_ref pointing at the LM-79 anchor (matching the Vode example).
func scaledProvNumber(value any, unit, baseLM79 string) map[string]any {
	prov := map[string]any{
		"source":           "datasheet_pdf",
		"method":           "scaled",
		"extension_method": "per_foot_linear_scaling",
	}
	if baseLM79 != "" {
		prov["base_attestation_ref"] = baseLM79
	}
	return map[string]any{
		"value":      value,
		"unit":       unit,
		"value_type": "rated",
		"provenance": prov,
	}
}

// computedProvNumber builds a ProvenancedNumber for a derived efficacy:
// value_type=rated, method=computed, extension_method=per_foot_linear_scaling.
func computedProvNumber(value any, unit, baseLM79 string) map[string]any {
	prov := map[string]any{
		"source":           "datasheet_pdf",
		"method":           "computed",
		"extension_method": "per_foot_linear_scaling",
	}
	if baseLM79 != "" {
		prov["base_attestation_ref"] = baseLM79
	}
	return map[string]any{
		"value":      value,
		"unit":       unit,
		"value_type": "rated",
		"provenance": prov,
	}
}

// lengthDualUnit builds a DualUnitLength for a fixture length authored as the SI
// (mm) leaf, with value_type=nominal and normalized provenance (the published
// nominal lengths in the Vode example carry method=normalized).
func lengthDualUnit(mm float64) map[string]any {
	return map[string]any{
		"mm":         numberLeaf(mm),
		"in":         numberLeaf(roundTo(mm/inchToMM, 4)),
		"value_type": "nominal",
		"provenance": map[string]any{
			"source": "datasheet_pdf",
			"method": "normalized",
		},
	}
}

// isLengthScaled reports whether the record is a Pattern D per-foot-linear case:
// a declared_by_length sheet exists, or the covered length axis declares a
// per_foot_linear_scaling derivation. This gates the declared_by_length
// generator so a Pattern B record (no length scaling) is never asked to scale.
func isLengthScaled(wb Workbook, id string, app map[string]any) bool {
	if len(wb.RowsFor("declared_by_length", id)) > 0 {
		return true
	}
	axis, ok := coveredAxis(app, "length")
	if !ok {
		return false
	}
	deriv, ok := axis["derivation"].(map[string]any)
	if !ok {
		return false
	}
	method, _ := deriv["method"].(string)
	return method == "per_foot_linear_scaling"
}

// ratesFromRecord reads the per-foot scaling rates and reference length off the
// assembled record's photometry.per_length_normalized block (written from the
// records columns). Absent rates leave the has* flags false so the generator can
// skip the corresponding quantity.
func ratesFromRecord(rec map[string]any) linearRates {
	var r linearRates
	if v, ok := provValue(rec, "photometry.per_length_normalized.lumens_per_foot.value"); ok {
		r.lumensPerFoot, r.hasLumens = v, true
	}
	if v, ok := provValue(rec, "photometry.per_length_normalized.watts_per_foot.value"); ok {
		r.wattsPerFoot, r.hasWatts = v, true
	}
	return r
}

// provValue reads a numeric leaf off the assembled record at a dotted path,
// coercing int64/int to float64. It reports false when the path is absent or not
// numeric.
func provValue(rec map[string]any, path string) (float64, bool) {
	v, ok := getPath(rec, path)
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

// lengthBaseAttestation resolves the base attestation id the generated
// declared_by_length rows reference: an explicit
// total_luminous_flux_lm__attestation_ref override on the master row wins,
// otherwise the record's single LM-79 anchor.
func lengthBaseAttestation(master Row, lm79ID string) string {
	if ref := master["total_luminous_flux_lm__attestation_ref"]; ref != "" {
		return ref
	}
	return lm79ID
}

// checkDivergence appends a warning when an authored value diverges from its
// per-foot projection by more than 2% (DESIGN.md decision 3). A zero projection
// is skipped to avoid a divide-by-zero ratio.
func checkDivergence(warnings *[]string, id string, lengthMM float64, field string, authored, projected float64) {
	if projected == 0 {
		return
	}
	diff := math.Abs(authored-projected) / math.Abs(projected)
	if diff > 0.02 {
		*warnings = append(*warnings, fmt.Sprintf(
			"record %q declared_by_length at %.1f mm: authored %s %.4g diverges from per-foot projection %.4g by %.1f%% (> 2%%)",
			id, lengthMM, field, authored, projected, diff*100))
	}
}
