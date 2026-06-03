package sheet

// Pattern is one of the four ULC authoring patterns. Classification is purely a
// function of sheet/column presence for a given record_id (no LLM), per
// DESIGN.md section 2.
type Pattern int

const (
	// PatternUnknown is the zero value, used before classification.
	PatternUnknown Pattern = iota
	// PatternA is a single-SKU cutsheet: one order code, fixed axes, a populated
	// catalog_number, and no derivation/length/multiplier sheets.
	PatternA
	// PatternB is a multiplier-table family: covered_axes plus a cct_multipliers
	// table.
	PatternB
	// PatternC is per-IES-with-provenance: fixed axes like A, but the headline
	// photometry derives from extended_photometry or optical_simulation with a
	// base_attestation_ref rather than a direct measurement.
	PatternC
	// PatternD is per-foot linear scaling: covered_axes plus declared_by_length /
	// per_foot_linear_scaling derivation.
	PatternD
)

// String returns the human label for a pattern (used in error messages).
func (p Pattern) String() string {
	switch p {
	case PatternA:
		return "A (single-SKU)"
	case PatternB:
		return "B (multiplier-table)"
	case PatternC:
		return "C (per-IES-with-provenance)"
	case PatternD:
		return "D (per-foot-linear)"
	default:
		return "unknown"
	}
}

// detectPattern classifies one record by the presence of its related-sheet rows
// and signal columns, applying the precedence rule from DESIGN.md section 2: the
// derivation/length/multiplier signals (B and D) win over the catalog_number
// signal, which resolves the Pattern D catalog_number collision. A vs C is not
// an applicability fork; C is distinguished only by a derived-provenance signal
// on the headline photometry.
//
// wb is the full workbook; id is the record_id; master is the record's row on
// the records sheet.
func detectPattern(wb Workbook, id string, master Row) Pattern {
	hasCoveredAxes := len(wb.RowsFor("covered_axes", id)) > 0
	hasMultipliers := len(wb.RowsFor("cct_multipliers", id)) > 0
	hasLengths := len(wb.RowsFor("declared_by_length", id)) > 0

	// Precedence: B/D derivation signals win over catalog_number.
	switch {
	case hasLengths:
		return PatternD
	case hasMultipliers:
		return PatternB
	case hasCoveredAxes:
		// covered_axes without a multiplier or length table still indicates a
		// multi-value applicability fork. Length signals were already handled
		// above, so a bare covered-axes record is a Pattern B family scenario.
		return PatternB
	}

	// A vs C: a headline photometry provenance method of extended_photometry or
	// optical_simulation (with a base attestation) marks Pattern C.
	if isDerivedHeadline(master) {
		return PatternC
	}
	return PatternA
}

// isDerivedHeadline reports whether the record's headline photometry carries a
// derived-provenance signal (the C marker): a flux or power prov_method override
// of extended_photometry or optical_simulation paired with a base_attestation_ref.
func isDerivedHeadline(master Row) bool {
	for _, header := range []string{"total_luminous_flux_lm", "input_power_w"} {
		method := master[header+"__prov_method"]
		_, hasBase := master[header+"__base_attestation_ref"]
		if (method == "extended_photometry" || method == "optical_simulation") && hasBase {
			return true
		}
	}
	return false
}
