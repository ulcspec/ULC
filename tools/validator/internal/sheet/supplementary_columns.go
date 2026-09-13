package sheet

import "fmt"

type supplementaryValueKey struct {
	sheet string
	field string
}

type supplementaryValueColumn struct {
	unit       string
	unitColumn string
	defaults   provenanceDefaults
}

// supplementaryValueColumns declares every provenanced value authored by a
// supplementary sheet in this release. declared_by_length remains on its
// existing hardcoded path because section 4 item 5 was cut.
var supplementaryValueColumns = map[supplementaryValueKey]supplementaryValueColumn{
	{sheet: "alpha_opic", field: "melanopic_der"}: {
		unit:     "ratio",
		defaults: provenanceDefaults{valueType: "rated", source: "datasheet_pdf", method: "extracted", family: attestationFamilyMelanopic},
	},
	{sheet: "alpha_opic", field: "efficacy"}: {
		unit:     "ratio",
		defaults: provenanceDefaults{valueType: "rated", source: "datasheet_pdf", method: "extracted", family: attestationFamilyMelanopic},
	},
	{sheet: "flicker_metrics", field: "value"}: {
		unit:       "ratio",
		unitColumn: "unit",
		defaults:   provenanceDefaults{valueType: "rated", source: "datasheet_pdf", method: "extracted", family: attestationFamilyFlicker},
	},
	{sheet: "lumen_maintenance_package", field: "tm_21_projection_hours"}: {
		unit:     "h",
		defaults: provenanceDefaults{valueType: "rated", source: "manufacturer_direct", method: "transcribed", family: attestationFamilyMaintenance},
	},
	{sheet: "lumen_maintenance_package", field: "test_hours"}: {
		unit:     "h",
		defaults: provenanceDefaults{valueType: "rated", source: "manufacturer_direct", method: "transcribed", family: attestationFamilyMaintenance},
	},
	{sheet: "lumen_maintenance_package", field: "drive_current_ma"}: {
		unit:     "mA",
		defaults: provenanceDefaults{valueType: "rated", source: "manufacturer_direct", method: "transcribed", family: attestationFamilyMaintenance},
	},
	{sheet: "zonal_lumens", field: "lumens"}: {
		unit:     "lm",
		defaults: provenanceDefaults{valueType: "measured", source: "ies", method: "extracted", family: attestationFamilyPhotometric},
	},
	{sheet: "lcs_zonal_lumens", field: "lumens"}: {
		unit:     "lm",
		defaults: provenanceDefaults{valueType: "measured", source: "ies", method: "extracted", family: attestationFamilyPhotometric},
	},
}

func supplementaryProvenancedNumber(sheet, field string, row Row, ctx provenanceContext) (map[string]any, error) {
	column, ok := supplementaryValueColumns[supplementaryValueKey{sheet: sheet, field: field}]
	if !ok {
		return nil, fmt.Errorf("supplementary sheet %q has no declared provenanced value column %q", sheet, field)
	}
	raw := row[field]
	if raw == "" {
		return nil, fmt.Errorf("missing %s", field)
	}
	value, err := parseFloat(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s %q: %w", field, raw, err)
	}
	resolved, err := resolveProvenanceForField(field, column.defaults, row, ctx)
	if err != nil {
		return nil, err
	}
	obj := map[string]any{
		"value":      numberLeaf(value),
		"value_type": resolved.valueType,
		"provenance": resolved.provenance,
	}
	unit := column.unit
	if column.unitColumn != "" && row[column.unitColumn] != "" {
		unit = row[column.unitColumn]
	}
	if unit != "" {
		obj["unit"] = unit
	}
	if note := row["conflict_notes"]; note != "" {
		resolved.provenance["conflict_notes"] = note
	}
	return obj, nil
}
