package sheet

import "fmt"

// derivedBaseMethods are the ProvenanceMethod values that produce a value from a
// base measurement; the taxonomy ProvenanceMethod description says each names
// that base in provenance.base_attestation_ref. The JSON Schema does not require
// it structurally (Provenance.required is only {source, method}), so the
// converter enforces it.
var derivedBaseMethods = map[string]bool{
	"extended_photometry": true,
	"optical_simulation":  true,
	"scaled":              true,
}

// provenanceContext carries the per-record anchor candidates grouped by the
// authored AttestationProgram families.
type provenanceContext struct {
	anchors map[attestationFamily]attestationAnchor
}

type provenanceDefaults struct {
	valueType string
	source    string
	method    string
	family    attestationFamily
}

func (ctx provenanceContext) singleAnchorID(family attestationFamily) string {
	anchor := ctx.anchors[family]
	if anchor.count == 1 && len(anchor.ids) == 1 {
		return anchor.ids[0]
	}
	return ""
}

// resolvedProvenance is the value_type plus provenance block the assembler
// stamps onto one ProvenancedNumber or DualUnit value.
type resolvedProvenance struct {
	valueType  string
	provenance map[string]any
}

// resolveProvenance is the records-sheet adapter. Records-sheet columns retain
// the photometric family the original LM-79 resolver used, which keeps current
// records byte-identical. A measured non-photometric records-sheet quantity can
// therefore still select a photometric anchor; that residue remains explicit
// for the batch close-out rather than being hidden in a prefix rule.
func resolveProvenance(col Column, row Row, ctx provenanceContext) (resolvedProvenance, error) {
	return resolveProvenanceForField(col.Header, provenanceDefaults{valueType: col.ProvValueType, source: col.ProvSource, method: col.ProvMethod, family: attestationFamilyPhotometric}, row, ctx)
}

// resolveProvenanceForField applies a field's defaults and companion overrides,
// then resolves measured and derived references against its declared program
// family. An explicit reference always wins.
func resolveProvenanceForField(field string, defaults provenanceDefaults, row Row, ctx provenanceContext) (resolvedProvenance, error) {
	valueType := defaults.valueType
	if v, ok := row[field+"__value_type"]; ok {
		valueType = v
	}
	source := defaults.source
	sourceOverridden := false
	if v, ok := row[field+"__prov_source"]; ok {
		source = v
		sourceOverridden = true
	}
	method := defaults.method
	if v, ok := row[field+"__prov_method"]; ok {
		method = v
	}

	// A non-measured value did not come from an IES measurement. When the author
	// overrides the default value_type to rated/nominal (the IES-free path) without
	// also setting the provenance source, switch the default "ies" to the rated
	// datasheet source so the record cannot claim IES provenance with no IES file.
	if valueType != "measured" && source == "ies" && !sourceOverridden {
		source = "datasheet_pdf"
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
	// optical_simulation, scaled). Pattern C supplies them on the headline
	// photometry columns; the generated B/D derivation tables set them directly.
	if v, ok := row[field+"__extension_method"]; ok {
		prov["extension_method"] = v
	}
	if v, ok := row[field+"__base_attestation_ref"]; ok {
		prov["base_attestation_ref"] = v
	}

	// Derived methods (extended_photometry, optical_simulation, scaled) name the
	// base measurement they derive from in base_attestation_ref. The schema's
	// Provenance.required is only {source, method}, so the converter enforces it:
	// an explicit override wins, otherwise auto-link to the record's single LM-79,
	// hard-erroring on the 0-or-many case exactly like measured -> attestation_ref.
	if derivedBaseMethods[method] {
		if base, _ := prov["base_attestation_ref"].(string); base == "" {
			ref, err := ctx.baseAttestationRefForFamily(field, method, defaults.family)
			if err != nil {
				return resolvedProvenance{}, err
			}
			prov["base_attestation_ref"] = ref
		}
	}

	// attestation_ref: explicit override wins; otherwise auto-link when measured.
	if v, ok := row[field+"__attestation_ref"]; ok {
		prov["attestation_ref"] = v
	} else if valueType == "measured" {
		ref, err := ctx.measuredAttestationRefForFamily(field, defaults.family)
		if err != nil {
			return resolvedProvenance{}, err
		}
		prov["attestation_ref"] = ref
	}

	return resolvedProvenance{valueType: valueType, provenance: prov}, nil
}

// measuredAttestationRef retains the photometric adapter for the zonal resolver
// until item 3 routes zonal values through resolveProvenanceForField.

func familyDescription(family attestationFamily) string {
	switch family {
	case attestationFamilyPhotometric:
		return "photometric lm_79*"
	case attestationFamilyMaintenance:
		return "maintenance lm_80* or tm_21*"
	case attestationFamilyFlicker:
		return "flicker lm_90_20, ieee_1789_2015, or nema_77_2017"
	case attestationFamilyMelanopic:
		return "melanopic rp_46 or rp_46_23"
	default:
		return string(family)
	}
}

func (ctx provenanceContext) measuredAttestationRef(header string) (string, error) {
	return ctx.measuredAttestationRefForFamily(header, attestationFamilyPhotometric)
}

func (ctx provenanceContext) measuredAttestationRefForFamily(header string, family attestationFamily) (string, error) {
	anchor := ctx.anchors[family]
	description := familyDescription(family)
	switch {
	case anchor.count == 1 && len(anchor.ids) == 0:
		return "", fmt.Errorf("column %q would auto-link to the record's single %s attestation, but that row has no attestation_id to reference; add an attestation_id to that attestations row", header, description)
	case anchor.count == 1 && len(anchor.ids) == 1:
		return anchor.ids[0], nil
	case anchor.count == 0:
		return "", fmt.Errorf("column %q has effective value_type=measured but the record declares no %s attestation row to link; add that attestation row or set %s__value_type=rated", header, description, header)
	default:
		return "", fmt.Errorf("column %q has effective value_type=measured but the record declares %d %s attestation rows; disambiguate with an explicit %s__attestation_ref column", header, anchor.count, description, header)
	}
}

// baseAttestationRef retains the photometric adapter for the zonal resolver
// until item 3 routes zonal values through resolveProvenanceForField.
func (ctx provenanceContext) baseAttestationRef(header, method string) (string, error) {
	return ctx.baseAttestationRefForFamily(header, method, attestationFamilyPhotometric)
}

func (ctx provenanceContext) baseAttestationRefForFamily(header, method string, family attestationFamily) (string, error) {
	anchor := ctx.anchors[family]
	description := familyDescription(family)
	switch {
	case anchor.count == 1 && len(anchor.ids) == 0:
		return "", fmt.Errorf("column %q (derived method %q) would anchor base_attestation_ref to the record's single %s attestation, but that row has no attestation_id; add an attestation_id to that attestations row", header, method, description)
	case anchor.count == 1 && len(anchor.ids) == 1:
		return anchor.ids[0], nil
	case anchor.count == 0:
		return "", fmt.Errorf("column %q uses derived method %q but the record declares no %s attestation to anchor provenance.base_attestation_ref; add that attestation row or set %s__base_attestation_ref explicitly", header, method, description, header)
	default:
		return "", fmt.Errorf("column %q uses derived method %q but the record declares %d %s attestation rows; disambiguate with an explicit %s__base_attestation_ref column", header, method, anchor.count, description, header)
	}
}
