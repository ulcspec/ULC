package sheet

import "fmt"

// provenanceContext carries the per-record facts the provenance resolver needs:
// the single LM-79 attestation id used for the measured -> attestation_ref
// auto-link, and how many LM-79 rows the record declared (so the resolver can
// hard-error on the ambiguous 0-or-many case only when an auto-link is actually
// needed).
type provenanceContext struct {
	lm79AttestationID string
	lm79Count         int
}

// resolvedProvenance is the value_type plus provenance block the assembler
// stamps onto one ProvenancedNumber or DualUnit value.
type resolvedProvenance struct {
	valueType  string
	provenance map[string]any
}

// resolveProvenance computes the effective value_type and provenance block for a
// provenanced column, applying the per-column defaults and the optional
// companion-column overrides (`*__value_type`, `*__prov_source`,
// `*__prov_method`, `*__attestation_ref`). It enforces the load-bearing rule
// from DESIGN.md section 3.3: any value whose effective value_type is "measured"
// MUST carry an attestation_ref, auto-linked to the record's single LM-79
// attestation, and it hard-errors when there are zero or more than one LM-79
// rows unless the manufacturer supplies an explicit `*__attestation_ref`.
func resolveProvenance(col Column, row Row, ctx provenanceContext) (resolvedProvenance, error) {
	valueType := col.ProvValueType
	if v, ok := row[col.Header+"__value_type"]; ok {
		valueType = v
	}
	source := col.ProvSource
	if v, ok := row[col.Header+"__prov_source"]; ok {
		source = v
	}
	method := col.ProvMethod
	if v, ok := row[col.Header+"__prov_method"]; ok {
		method = v
	}

	prov := map[string]any{}
	if source != "" {
		prov["source"] = source
	}
	if method != "" {
		prov["method"] = method
	}

	// extension_method / base_attestation_ref companion overrides: these are the
	// extension points the C and D patterns lean on (extended_photometry,
	// optical_simulation, scaled). They are plumbed here so the provenance layer
	// is pattern-complete; the assembler restricts to Pattern A in increment 1.
	if v, ok := row[col.Header+"__extension_method"]; ok {
		prov["extension_method"] = v
	}
	if v, ok := row[col.Header+"__base_attestation_ref"]; ok {
		prov["base_attestation_ref"] = v
	}

	// attestation_ref: explicit override wins; otherwise auto-link when measured.
	if v, ok := row[col.Header+"__attestation_ref"]; ok {
		prov["attestation_ref"] = v
	} else if valueType == "measured" {
		ref, err := ctx.measuredAttestationRef(col.Header)
		if err != nil {
			return resolvedProvenance{}, err
		}
		prov["attestation_ref"] = ref
	}

	return resolvedProvenance{valueType: valueType, provenance: prov}, nil
}

// measuredAttestationRef returns the attestation id a measured value links to,
// hard-erroring when the record carries zero or more than one LM-79 attestation
// (the manufacturer must then disambiguate with an explicit `*__attestation_ref`
// column on the offending field).
func (ctx provenanceContext) measuredAttestationRef(header string) (string, error) {
	switch {
	case ctx.lm79Count == 1:
		return ctx.lm79AttestationID, nil
	case ctx.lm79Count == 0:
		return "", fmt.Errorf("column %q has effective value_type=measured but the record declares no lm_79* attestation row to link; add an lm_79* attestations row or set %s__value_type=rated", header, header)
	default:
		return "", fmt.Errorf("column %q has effective value_type=measured but the record declares %d lm_79* attestation rows; disambiguate with an explicit %s__attestation_ref column", header, ctx.lm79Count, header)
	}
}
