package sheet

import (
	"fmt"
	"math"
	"strconv"
)

// assembleFullLevelBlocks attaches the optional comprehensive ("full-level")
// blocks that a manufacturer authors on dedicated long sheets, shared by all
// four patterns:
//
//	alpha_opic                 -> alpha_opic_metrics
//	flicker_metrics            -> flicker_measurements.metrics[]
//	lumen_maintenance_package  -> lumen_maintenance_package[]            (top-level array)
//	zonal_lumens               -> photometry.zonal_lumens[]
//	lcs_zonal_lumens           -> outdoor_classification.lcs_zonal_lumens[]
//	ingredient_list            -> sustainability_declaration.ingredient_list[]
//	cie97_lmf / cie97_llmf     -> lumen_maintenance_luminaire.cie_97_lmf_table
//
// Each block attaches only when its sheet carries rows for the record. None of
// these gates the conformance level (the rubric reads operating_point and the
// conditional bug_rating, not these blocks), so they are pure enrichment: a
// manufacturer who has the data adds the sheet and the record carries more
// depth, while one who does not simply omits it. The sustainability_declaration
// block-level scalars (declaration_type, dates, recyclable_percent, ...) ride on
// the records sheet via recordColumns; this assembler only adds its ingredient
// roster, which coexists with those scalars under the same parent.
//
// ctx carries the record's single LM-79 anchor so the measured zonal-lumen
// values can auto-link their attestation_ref, exactly like the headline
// photometry.
func assembleFullLevelBlocks(wb Workbook, id string, rec map[string]any, ctx provenanceContext) error {
	if err := assembleAlphaOpic(wb, id, rec); err != nil {
		return err
	}
	if err := assembleFlickerMeasurements(wb, id, rec); err != nil {
		return err
	}
	if err := assembleLumenMaintenancePackage(wb, id, rec); err != nil {
		return err
	}
	if err := assembleZonalLumens(wb, id, rec, ctx); err != nil {
		return err
	}
	if err := assembleLCSZonalLumens(wb, id, rec, ctx); err != nil {
		return err
	}
	if err := assembleIngredientList(wb, id, rec); err != nil {
		return err
	}
	return assembleCIE97Table(wb, id, rec)
}

// ratedRatioProvNumber builds the {value, unit, value_type:rated, provenance}
// shape the rated datasheet ratios use (alpha-opic efficacy, melanopic DER,
// flicker metrics). The provenance is the datasheet default {datasheet_pdf,
// extracted}; these are manufacturer-published values, not direct measurements,
// so no attestation_ref auto-link fires.
func ratedRatioProvNumber(value float64, unit string) map[string]any {
	obj := map[string]any{
		"value":      numberLeaf(value),
		"value_type": "rated",
		"provenance": map[string]any{"source": "datasheet_pdf", "method": "extracted"},
	}
	if unit != "" {
		obj["unit"] = unit
	}
	return obj
}

// transcribedProvNumber builds the {value, unit, value_type:rated, provenance}
// shape the lumen-maintenance package quantities use: a value transcribed from
// the manufacturer's published LM-80 / TM-21 figures ({manufacturer_direct,
// transcribed}), matching the erco example.
func transcribedProvNumber(value float64, unit string) map[string]any {
	obj := map[string]any{
		"value":      numberLeaf(value),
		"value_type": "rated",
		"provenance": map[string]any{"source": "manufacturer_direct", "method": "transcribed"},
	}
	if unit != "" {
		obj["unit"] = unit
	}
	return obj
}

// assembleAlphaOpic builds the alpha_opic_metrics block from the alpha_opic
// sheet. The block-level scalars (reference_illuminant, standard_observer,
// melanopic_der) are axis-level (first non-blank cell across the record's rows,
// like the covered_axes rationale); each row that names a channel contributes a
// per_channel entry. The block lives at the record root (a sibling of
// photometry / colorimetry), not under photometry.
func assembleAlphaOpic(wb Workbook, id string, rec map[string]any) error {
	rows := wb.RowsFor("alpha_opic", id)
	if len(rows) == 0 {
		return nil
	}
	block := map[string]any{}
	var refIllum, stdObs, melDER string
	perChannel := []any{}
	for i, row := range rows {
		firstNonBlank(&refIllum, row["reference_illuminant"])
		firstNonBlank(&stdObs, row["standard_observer"])
		firstNonBlank(&melDER, row["melanopic_der"])
		ch := row["channel"]
		if ch == "" {
			continue
		}
		raw := row["efficacy"]
		if raw == "" {
			return fmt.Errorf("alpha_opic row %d for %q: channel %q has no efficacy", i+1, id, ch)
		}
		eff, err := parseFloat(raw)
		if err != nil {
			return fmt.Errorf("alpha_opic row %d for %q: invalid efficacy %q: %w", i+1, id, raw, err)
		}
		perChannel = append(perChannel, map[string]any{
			"channel":  ch,
			"efficacy": ratedRatioProvNumber(eff, "ratio"),
		})
	}
	if refIllum != "" {
		block["reference_illuminant"] = refIllum
	}
	if stdObs != "" {
		block["standard_observer"] = stdObs
	}
	if melDER != "" {
		v, err := parseFloat(melDER)
		if err != nil {
			return fmt.Errorf("alpha_opic for %q: invalid melanopic_der %q: %w", id, melDER, err)
		}
		block["melanopic_der"] = ratedRatioProvNumber(v, "ratio")
	}
	if len(perChannel) > 0 {
		block["per_channel"] = perChannel
	}
	if len(block) > 0 {
		rec["alpha_opic_metrics"] = block
	}
	return nil
}

// assembleFlickerMeasurements builds flicker_measurements.metrics[] from the
// flicker_metrics sheet (one row per metric). Each metric carries a rated
// ProvenancedNumber value (default unit ratio, overridable per row) and an
// optional bound_operator. The block name is flicker_measurements; the closed
// metric enum (svm, pst_lm, percent_flicker, ...) is validated by the schema.
func assembleFlickerMeasurements(wb Workbook, id string, rec map[string]any) error {
	rows := wb.RowsFor("flicker_metrics", id)
	if len(rows) == 0 {
		return nil
	}
	metrics := []any{}
	for i, row := range rows {
		metric := row["metric"]
		if metric == "" {
			return fmt.Errorf("flicker_metrics row %d for %q: missing metric", i+1, id)
		}
		entry := map[string]any{"metric": metric}
		if raw := row["value"]; raw != "" {
			v, err := parseFloat(raw)
			if err != nil {
				return fmt.Errorf("flicker_metrics row %d for %q: invalid value %q: %w", i+1, id, raw, err)
			}
			unit := row["unit"]
			if unit == "" {
				unit = "ratio"
			}
			entry["value"] = ratedRatioProvNumber(v, unit)
		}
		if bo := row["bound_operator"]; bo != "" {
			entry["bound_operator"] = bo
		}
		metrics = append(metrics, entry)
	}
	rec["flicker_measurements"] = map[string]any{"metrics": metrics}
	return nil
}

// assembleLumenMaintenancePackage builds the top-level lumen_maintenance_package[]
// array from the lumen_maintenance_package sheet (one row per LED package). The
// numeric quantities are transcribed manufacturer figures (rated). This is the
// method-backed projection the full-level conformance observation looks for (a
// tm_21_projection_hours present means the record carries more than a bare
// manufacturer claim).
func assembleLumenMaintenancePackage(wb Workbook, id string, rec map[string]any) error {
	rows := wb.RowsFor("lumen_maintenance_package", id)
	if len(rows) == 0 {
		return nil
	}
	out := []any{}
	for _, row := range rows {
		entry := map[string]any{}
		copyIf(entry, row, "package_identifier", "package_identifier")
		copyIf(entry, row, "tested_product_type", "tested_product_type")
		copyIf(entry, row, "flux_maintenance_quantity", "flux_maintenance_quantity")
		copyIf(entry, row, "flux_maintenance_threshold", "flux_maintenance_threshold")
		copyIf(entry, row, "projection_reliability", "projection_reliability")
		copyIf(entry, row, "tm_21_interpolation_type", "tm_21_interpolation_type")
		for _, q := range []struct{ col, key, unit string }{
			{"tm_21_projection_hours", "tm_21_projection_hours", "h"},
			{"test_hours", "test_hours", "h"},
			{"test_temperature_c", "test_temperature_c", "C"},
			{"drive_current_ma", "drive_current_ma", "mA"},
		} {
			raw := row[q.col]
			if raw == "" {
				continue
			}
			v, err := parseFloat(raw)
			if err != nil {
				return fmt.Errorf("lumen_maintenance_package for %q: invalid %s %q: %w", id, q.col, raw, err)
			}
			entry[q.key] = transcribedProvNumber(v, q.unit)
		}
		out = append(out, entry)
	}
	rec["lumen_maintenance_package"] = out
	return nil
}

// assembleZonalLumens builds photometry.zonal_lumens[] from the zonal_lumens
// sheet. Each row is one angle band (zone_label, for example "0-30") whose
// lumens are measured photometric data (auto-linked to the record's LM-79
// attestation). The optional conflict_notes cell rides into the lumens
// provenance, matching the selux example.
func assembleZonalLumens(wb Workbook, id string, rec map[string]any, ctx provenanceContext) error {
	rows := wb.RowsFor("zonal_lumens", id)
	if len(rows) == 0 {
		return nil
	}
	out := []any{}
	for i, row := range rows {
		zone := row["zone_label"]
		if zone == "" {
			return fmt.Errorf("zonal_lumens row %d for %q: missing zone_label", i+1, id)
		}
		lumens, err := measuredLumens(row, "lumens", ctx)
		if err != nil {
			return fmt.Errorf("zonal_lumens row %d for %q (%s): %w", i+1, id, zone, err)
		}
		out = append(out, map[string]any{"zone_label": zone, "lumens": lumens})
	}
	return setPath(rec, "photometry.zonal_lumens", out)
}

// assembleLCSZonalLumens builds outdoor_classification.lcs_zonal_lumens[] from
// the lcs_zonal_lumens sheet: the TM-15 Luminaire Classification System
// secondary solid-angle zones (fl, fm, fh, fvh, bl, bm, bh, bvh, ul, uh). Each
// zone's lumens are measured (auto-linked) like the zonal_lumens block.
func assembleLCSZonalLumens(wb Workbook, id string, rec map[string]any, ctx provenanceContext) error {
	rows := wb.RowsFor("lcs_zonal_lumens", id)
	if len(rows) == 0 {
		return nil
	}
	out := []any{}
	for i, row := range rows {
		zone := row["zone"]
		if zone == "" {
			return fmt.Errorf("lcs_zonal_lumens row %d for %q: missing zone", i+1, id)
		}
		lumens, err := measuredLumens(row, "lumens", ctx)
		if err != nil {
			return fmt.Errorf("lcs_zonal_lumens row %d for %q (%s): %w", i+1, id, zone, err)
		}
		out = append(out, map[string]any{"zone": zone, "lumens": lumens})
	}
	return setPath(rec, "outdoor_classification.lcs_zonal_lumens", out)
}

// measuredLumens builds a measured-lumen ProvenancedNumber for a zonal cell. The
// default provenance is {ies, extracted, measured}, which triggers the same
// LM-79 attestation_ref auto-link the headline photometry uses; the per-row
// override columns (`<field>__value_type`, `__prov_source`, `__prov_method`,
// `__attestation_ref`) and an optional `conflict_notes` cell let the author
// override the defaults or record a reconstruction note.
func measuredLumens(row Row, field string, ctx provenanceContext) (map[string]any, error) {
	raw := row[field]
	if raw == "" {
		return nil, fmt.Errorf("missing %s", field)
	}
	v, err := parseFloat(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid %s %q: %w", field, raw, err)
	}
	valueType := "measured"
	if vt := row[field+"__value_type"]; vt != "" {
		valueType = vt
	}
	source := "ies"
	if s := row[field+"__prov_source"]; s != "" {
		source = s
	}
	method := "extracted"
	if m := row[field+"__prov_method"]; m != "" {
		method = m
	}
	prov := map[string]any{"source": source, "method": method}
	if ref := row[field+"__attestation_ref"]; ref != "" {
		prov["attestation_ref"] = ref
	} else if valueType == "measured" {
		ref, err := ctx.measuredAttestationRef(field)
		if err != nil {
			return nil, err
		}
		prov["attestation_ref"] = ref
	}
	// A zonal lumen overridden to a derived method (scaled / optical_simulation /
	// extended_photometry) must name its base attestation, same as every other
	// derived value: explicit override wins, else auto-link to the single LM-79.
	if ref := row[field+"__base_attestation_ref"]; ref != "" {
		prov["base_attestation_ref"] = ref
	}
	if derivedBaseMethods[method] {
		if base, _ := prov["base_attestation_ref"].(string); base == "" {
			ref, err := ctx.baseAttestationRef(field, method)
			if err != nil {
				return nil, err
			}
			prov["base_attestation_ref"] = ref
		}
	}
	if cn := row["conflict_notes"]; cn != "" {
		prov["conflict_notes"] = cn
	}
	return map[string]any{
		"value":      numberLeaf(v),
		"unit":       "lm",
		"value_type": valueType,
		"provenance": prov,
	}, nil
}

// assembleIngredientList builds sustainability_declaration.ingredient_list[]
// from the ingredient_list sheet (one row per material). Items are plain objects
// (material_name required; lbc_red_list_status and notes optional) with no
// provenance. The block-level sustainability scalars arrive separately through
// the records-sheet columns and share the same parent.
func assembleIngredientList(wb Workbook, id string, rec map[string]any) error {
	rows := wb.RowsFor("ingredient_list", id)
	if len(rows) == 0 {
		return nil
	}
	out := []any{}
	for i, row := range rows {
		name := row["material_name"]
		if name == "" {
			return fmt.Errorf("ingredient_list row %d for %q: missing material_name", i+1, id)
		}
		item := map[string]any{"material_name": name}
		copyIf(item, row, "lbc_red_list_status", "lbc_red_list_status")
		copyIf(item, row, "notes", "notes")
		out = append(out, item)
	}
	return setPath(rec, "sustainability_declaration.ingredient_list", out)
}

// assembleCIE97Table builds lumen_maintenance_luminaire.cie_97_lmf_table from
// the cie97_lmf and cie97_llmf sheets. The lmf / llmf / lsf leaves are bare JSON
// numbers (NOT ProvenancedNumber): the CIE 97 maintenance factors are
// manufacturer-declared design-stage reference values, so they carry no unit,
// value_type, or provenance. The table coexists with the declaration_framework
// and manufacturer_rated_claim written from the records columns under the same
// lumen_maintenance_luminaire parent (multiple frameworks may co-occur).
func assembleCIE97Table(wb Workbook, id string, rec map[string]any) error {
	lmfRows := wb.RowsFor("cie97_lmf", id)
	llmfRows := wb.RowsFor("cie97_llmf", id)
	if len(lmfRows) == 0 && len(llmfRows) == 0 {
		return nil
	}
	table := map[string]any{}

	if len(lmfRows) > 0 {
		arr := []any{}
		for i, row := range lmfRows {
			years, err := parseIntCell(row["cleaning_interval_years"])
			if err != nil {
				return fmt.Errorf("cie97_lmf row %d for %q: cleaning_interval_years: %w", i+1, id, err)
			}
			clean := row["ambient_cleanliness"]
			if clean == "" {
				return fmt.Errorf("cie97_lmf row %d for %q: missing ambient_cleanliness", i+1, id)
			}
			raw := row["lmf"]
			if raw == "" {
				return fmt.Errorf("cie97_lmf row %d for %q: missing lmf", i+1, id)
			}
			lmf, err := parseFloat(raw)
			if err != nil {
				return fmt.Errorf("cie97_lmf row %d for %q: invalid lmf %q: %w", i+1, id, raw, err)
			}
			arr = append(arr, map[string]any{
				"cleaning_interval_years": years,
				"ambient_cleanliness":     clean,
				"lmf":                     numberLeaf(lmf),
			})
		}
		table["lmf_by_cleanliness_and_interval"] = arr
	}

	if len(llmfRows) > 0 {
		arr := []any{}
		for i, row := range llmfRows {
			hours, err := parseIntCell(row["hours"])
			if err != nil {
				return fmt.Errorf("cie97_llmf row %d for %q: hours: %w", i+1, id, err)
			}
			item := map[string]any{"hours": hours}
			if raw := row["llmf"]; raw != "" {
				v, err := parseFloat(raw)
				if err != nil {
					return fmt.Errorf("cie97_llmf row %d for %q: invalid llmf %q: %w", i+1, id, raw, err)
				}
				item["llmf"] = numberLeaf(v)
			}
			if raw := row["lsf"]; raw != "" {
				v, err := parseFloat(raw)
				if err != nil {
					return fmt.Errorf("cie97_llmf row %d for %q: invalid lsf %q: %w", i+1, id, raw, err)
				}
				item["lsf"] = numberLeaf(v)
			}
			arr = append(arr, item)
		}
		table["llmf_by_hours"] = arr
	}

	return setPath(rec, "lumen_maintenance_luminaire.cie_97_lmf_table", table)
}

// parseIntCell parses an integer cell (cleaning_interval_years, hours) into an
// int64, so the emitted JSON renders a bare integer the schema's integer type
// requires. It tolerates the integral-float spellings a spreadsheet exporter may
// emit for an integer cell ("1.0", "1e1"): an .xlsx number cell carries its raw
// stored value, which Excel may format with a trailing decimal, so a strict
// base-10 parse alone would reject otherwise-valid data. A genuine non-integer
// (1.5) is still rejected with a clear message.
func parseIntCell(raw string) (int64, error) {
	if raw == "" {
		return 0, fmt.Errorf("missing integer value")
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return n, nil
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid integer %q", raw)
	}
	if f != math.Trunc(f) || math.IsInf(f, 0) {
		return 0, fmt.Errorf("expected an integer, got %q", raw)
	}
	return int64(f), nil
}
