package sheet

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestResolveProvenanceDerivedMethodRequiresBase locks the rule that a derived
// method (extended_photometry / optical_simulation / scaled) must resolve a
// non-empty provenance.base_attestation_ref: an explicit override wins, else the
// converter auto-links to the single LM-79 and hard-errors on the 0-or-many case.
func TestResolveProvenanceDerivedMethodRequiresBase(t *testing.T) {
	col := Column{Header: "total_luminous_flux_lm", ProvSource: "ies", ProvMethod: "extracted", ProvValueType: "rated"}

	// extended_photometry with no base override and no lm_79 anchor -> hard error.
	if _, err := resolveProvenance(col,
		Row{"total_luminous_flux_lm__prov_method": "extended_photometry"},
		provenanceContext{lm79Count: 0}); err == nil {
		t.Fatal("expected error for derived method with no base attestation and no lm_79 anchor")
	}

	// extended_photometry with a single lm_79 -> auto-links base_attestation_ref.
	rp, err := resolveProvenance(col,
		Row{"total_luminous_flux_lm__prov_method": "extended_photometry"},
		provenanceContext{lm79AttestationID: "att-lm79-1", lm79Count: 1})
	if err != nil {
		t.Fatalf("unexpected error with single lm_79 anchor: %v", err)
	}
	if got := rp.provenance["base_attestation_ref"]; got != "att-lm79-1" {
		t.Fatalf("base_attestation_ref = %v, want att-lm79-1", got)
	}

	// Explicit base override wins, no lm_79 anchor needed.
	rp, err = resolveProvenance(col,
		Row{"total_luminous_flux_lm__prov_method": "optical_simulation", "total_luminous_flux_lm__base_attestation_ref": "BASE-9"},
		provenanceContext{lm79Count: 0})
	if err != nil {
		t.Fatalf("unexpected error with explicit base override: %v", err)
	}
	if got := rp.provenance["base_attestation_ref"]; got != "BASE-9" {
		t.Fatalf("base_attestation_ref = %v, want BASE-9", got)
	}
}

// TestHashFileRejectsAbsoluteAndTraversal locks that referenced files must be
// relative paths under the assets root: absolute paths and ".." traversal are
// rejected (so no local path leaks into a FileReference and no file outside the
// assets dir is hashed), while a clean relative name hashes.
func TestHashFileRejectsAbsoluteAndTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.ies"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &fileHasher{assetsRoot: dir}

	if _, err := h.hashFile(filepath.Join(dir, "ok.ies")); err == nil {
		t.Error("expected error for an absolute path")
	}
	if _, err := h.hashFile("../escape.txt"); err == nil {
		t.Error("expected error for .. traversal")
	}
	sum, err := h.hashFile("ok.ies")
	if err != nil {
		t.Fatalf("a relative path under the assets root should hash: %v", err)
	}
	if len(sum) != 64 {
		t.Fatalf("sha256 hex length = %d, want 64", len(sum))
	}
}

// TestParseJSONObjectCellRejectsTrailing locks the trailing-content guard so a
// malformed workbook cell fails loudly instead of silently ignoring the suffix.
func TestParseJSONObjectCellRejectsTrailing(t *testing.T) {
	if _, err := parseJSONObjectCell("axes", `{"a":"b"} junk`); err == nil {
		t.Error("expected error for trailing garbage after the object")
	}
	if _, err := parseJSONObjectCell("axes", `{"a":"b"}{"c":"d"}`); err == nil {
		t.Error("expected error for a trailing second object")
	}
	obj, err := parseJSONObjectCell("axes", `{"a":"b"}`)
	if err != nil {
		t.Fatalf("a clean JSON object cell should parse: %v", err)
	}
	if obj["a"] != "b" {
		t.Fatalf("obj[\"a\"] = %v, want b", obj["a"])
	}
}

// TestMeasuredLumensDerivedRequiresBase locks that a zonal/LCS lumen overridden
// to a derived method must resolve a base_attestation_ref (auto-linked to the
// single LM-79, hard-erroring on 0-or-many).
func TestMeasuredLumensDerivedRequiresBase(t *testing.T) {
	row := Row{"lumens": "1200", "lumens__value_type": "rated", "lumens__prov_method": "scaled", "lumens__extension_method": "cct_multiplier"}

	if _, err := measuredLumens(row, "lumens", provenanceContext{lm79Count: 0}); err == nil {
		t.Error("expected error: derived zonal lumen with no base attestation and no lm_79 anchor")
	}

	pn, err := measuredLumens(row, "lumens", provenanceContext{lm79AttestationID: "L1", lm79Count: 1})
	if err != nil {
		t.Fatalf("unexpected error with single lm_79 anchor: %v", err)
	}
	prov, _ := pn["provenance"].(map[string]any)
	if prov["base_attestation_ref"] != "L1" {
		t.Fatalf("base_attestation_ref = %v, want L1", prov["base_attestation_ref"])
	}
	if prov["extension_method"] != "cct_multiplier" {
		t.Fatalf("extension_method = %v, want cct_multiplier (the override column must pass through)", prov["extension_method"])
	}
}

// TestRejectCaseByCaseMeasuredAttestation locks that a case-by-case attestation
// (requires_manufacturer_confirmation) cannot be promoted to value_type=measured.
func TestRejectCaseByCaseMeasuredAttestation(t *testing.T) {
	h := &fileHasher{}
	if _, err := buildAttestation(Row{"program": "baa_compliance", "value_type": "measured", "verification_type": "requires_manufacturer_confirmation"}, h); err == nil {
		t.Error("expected error: case-by-case attestation cannot be value_type=measured")
	}
	if _, err := buildAttestation(Row{"program": "baa_compliance", "value_type": "rated", "verification_type": "requires_manufacturer_confirmation"}, h); err != nil {
		t.Errorf("case-by-case + rated should be allowed: %v", err)
	}
	if _, err := buildSharedAttestation(Row{"program": "baa_compliance", "value_type": "measured", "verification_type": "requires_manufacturer_confirmation"}); err == nil {
		t.Error("expected error: shared case-by-case attestation cannot be value_type=measured")
	}
}

// TestCheckRelatedSheetIDs locks the preflight: consumed related-sheet rows must
// carry a record_id that exists in the records sheet; unrelated extra tabs are
// ignored.
func TestCheckRelatedSheetIDs(t *testing.T) {
	records := []Row{{"record_id": "r1"}}

	if err := checkRelatedSheetIDs(Workbook{"records": records, "source_files": {{"record_id": "r2", "filename": "x.ies"}}}, records); err == nil {
		t.Error("expected error: source_files record_id r2 not in records")
	}
	if err := checkRelatedSheetIDs(Workbook{"records": records, "attestations": {{"program": "lm_79"}}}, records); err == nil {
		t.Error("expected error: attestations row missing record_id")
	}
	if err := checkRelatedSheetIDs(Workbook{"records": records, "instructions": {{"note": "fill this in"}}, "source_files": {{"record_id": "r1", "filename": "x.ies"}}}, records); err != nil {
		t.Errorf("valid workbook with an ignored extra tab should pass: %v", err)
	}
}

// TestGenerateDeclaredByCCTBaselineMeasured locks that the baseline CCT row of a
// generated declared_by_cct table carries the measured baseline flux (not a
// multiplier-scaled value), even when the authored baseline multiplier is not 1.0.
func TestGenerateDeclaredByCCTBaselineMeasured(t *testing.T) {
	mult := map[string]float64{"3000": 0.95, "4000": 1.05}
	out := generateDeclaredByCCT(mult, []string{"3000", "4000"}, 4000, "3000", "L1")
	for _, e := range out {
		row := e.(map[string]any)
		if row["cct"] != "3000" {
			continue
		}
		lum := row["lumens"].(map[string]any)
		if lum["value_type"] != "measured" {
			t.Errorf("baseline row value_type = %v, want measured", lum["value_type"])
		}
		if got := fmt.Sprint(lum["value"]); got != "4000" {
			t.Errorf("baseline lumens = %s, want 4000 (the measured baseline, not round(0.95*4000))", got)
		}
	}
}

// TestBuildAttestationApplicability locks the option-conditional attestation
// block: required_order_code_options (;-list) + required_constraints_json (JSON
// object) become attestation.applicability; an unconditional attestation has none.
func TestBuildAttestationApplicability(t *testing.T) {
	h := &fileHasher{}
	att, err := buildAttestation(Row{
		"program":                     "chicago_plenum",
		"value_type":                  "rated",
		"required_order_code_options": "CPP;HD",
		"required_constraints_json":   `{"voltage":"277"}`,
	}, h)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	app, ok := att["applicability"].(map[string]any)
	if !ok {
		t.Fatal("expected an applicability block")
	}
	if opts, _ := app["required_order_code_options"].([]any); len(opts) != 2 {
		t.Fatalf("required_order_code_options = %v, want 2 entries", app["required_order_code_options"])
	}
	if rc, _ := app["required_constraints"].(map[string]any); rc["voltage"] != "277" {
		t.Fatalf("required_constraints = %v, want voltage 277", app["required_constraints"])
	}

	plain, _ := buildAttestation(Row{"program": "ul_listed", "value_type": "rated"}, h)
	if _, present := plain["applicability"]; present {
		t.Error("an unconditional attestation should carry no applicability block")
	}
}

// TestRatedOverrideSwitchesSourceOffIES locks that overriding a default-IES
// photometry anchor to rated (the IES-free path) switches provenance.source off
// "ies" so the record cannot claim IES provenance with no IES file; an explicit
// prov_source override is still honored, and the measured default keeps ies.
func TestRatedOverrideSwitchesSourceOffIES(t *testing.T) {
	col := Column{Header: "total_luminous_flux_lm", ProvSource: "ies", ProvMethod: "extracted", ProvValueType: "measured"}

	rp, err := resolveProvenance(col, Row{"total_luminous_flux_lm__value_type": "rated"}, provenanceContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rp.provenance["source"] == "ies" {
		t.Errorf("rated override should switch the default ies source off, got %v", rp.provenance["source"])
	}

	rp, err = resolveProvenance(col, Row{"total_luminous_flux_lm__value_type": "rated", "total_luminous_flux_lm__prov_source": "ies"}, provenanceContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rp.provenance["source"] != "ies" {
		t.Errorf("explicit prov_source=ies should be honored even when rated, got %v", rp.provenance["source"])
	}

	rp, err = resolveProvenance(col, Row{}, provenanceContext{lm79AttestationID: "L1", lm79Count: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rp.provenance["source"] != "ies" {
		t.Errorf("measured default should keep ies, got %v", rp.provenance["source"])
	}
}

// TestAnchorRequiresAttestationID locks that a single lm_79* attestation with no
// attestation_id cannot be used as an auto-link anchor (it must error, not emit
// an empty attestation_ref).
func TestAnchorRequiresAttestationID(t *testing.T) {
	col := Column{Header: "total_luminous_flux_lm", ProvSource: "ies", ProvMethod: "extracted", ProvValueType: "measured"}
	if _, err := resolveProvenance(col, Row{}, provenanceContext{lm79Count: 1, lm79AttestationID: ""}); err == nil {
		t.Error("expected error: single lm_79 anchor with no attestation_id")
	}
}

// TestAssembleSourceFilesCutsheetConflict locks that listing the cutsheet
// filename in source_files with a non-datasheet_pdf file_type errors (rather
// than silently dropping the datasheet from source_files[]).
func TestAssembleSourceFilesCutsheetConflict(t *testing.T) {
	h := &fileHasher{allowMissing: true} // no real files on disk
	cutRef := map[string]any{"filename": "cut.pdf", "sha256": zeroSHA256}

	conflict := Workbook{"source_files": {{"record_id": "r1", "filename": "cut.pdf", "file_type": "ies"}}}
	if _, err := assembleSourceFiles(conflict, "r1", "cut.pdf", cutRef, h); err == nil {
		t.Error("expected error: cutsheet filename listed with conflicting file_type=ies")
	}

	ok := Workbook{"source_files": {{"record_id": "r1", "filename": "cut.pdf", "file_type": "datasheet_pdf"}}}
	out, err := assembleSourceFiles(ok, "r1", "cut.pdf", cutRef, h)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 source_files entry (no duplicate), got %d", len(out))
	}
}

// TestVerifyIESReference locks that a record whose headline photometry is
// measured from an IES must reference an IES in source_files; rated photometry
// (no IES) and a present IES source both pass.
func TestVerifyIESReference(t *testing.T) {
	iesSF := []any{map[string]any{"file_type": "ies", "reference": map[string]any{"filename": "lumos-skyline.ies"}}}
	measuredFlux := map[string]any{"value_type": "measured", "provenance": map[string]any{"source": "ies"}}

	// measured-ies flux but no ies source_files row -> error
	if err := verifyIESReference(map[string]any{
		"source_files": []any{},
		"photometry":   map[string]any{"total_luminous_flux_lm": measuredFlux},
	}, "r1"); err == nil {
		t.Error("expected error: measured-ies photometry with no ies source_files row")
	}

	// source_ies_ref that names no ies source_files entry -> error (schema match)
	if err := verifyIESReference(map[string]any{
		"source_files":  iesSF,
		"configuration": map[string]any{"source_ies_ref": "some-other-name.ies"},
		"photometry":    map[string]any{"total_luminous_flux_lm": measuredFlux},
	}, "r1"); err == nil {
		t.Error("expected error: source_ies_ref matches no ies source_files filename")
	}

	// source_ies_ref equal to the ies source_files filename + measured flux -> ok
	if err := verifyIESReference(map[string]any{
		"source_files":  iesSF,
		"configuration": map[string]any{"source_ies_ref": "lumos-skyline.ies"},
		"photometry":    map[string]any{"total_luminous_flux_lm": measuredFlux},
	}, "r1"); err != nil {
		t.Errorf("matching source_ies_ref should pass: %v", err)
	}

	// rated flux (no IES) needs no ies source -> ok
	ratedFlux := map[string]any{"value_type": "rated", "provenance": map[string]any{"source": "datasheet_pdf"}}
	if err := verifyIESReference(map[string]any{
		"source_files": []any{},
		"photometry":   map[string]any{"total_luminous_flux_lm": ratedFlux},
	}, "r1"); err != nil {
		t.Errorf("rated photometry without an IES should pass: %v", err)
	}

	// flux overridden to rated, but another anchor (input power) is still
	// measured-ies, with no ies source -> error (whole-record scan, not just flux)
	if err := verifyIESReference(map[string]any{
		"source_files": []any{},
		"photometry":   map[string]any{"total_luminous_flux_lm": ratedFlux},
		"electrical":   map[string]any{"input_power_w": measuredFlux},
	}, "r1"); err == nil {
		t.Error("expected error: a non-flux measured-ies anchor with no ies source_files row")
	}
}

// TestGenerateDeclaredByLengthRequiresNumeric locks that Pattern D generation
// fails fast on a non-numeric baseline or covered length, rather than silently
// emitting the baseline as a derived row or dropping order-code length values.
func TestGenerateDeclaredByLengthRequiresNumeric(t *testing.T) {
	rates := linearRates{lumensPerFoot: 500, hasLumens: true}

	if _, err := generateDeclaredByLength(declaredByLengthParams{rates: rates, lengthValues: []string{"96"}, baseLM79: "L1", baselineIn: "any"}, "r1"); err == nil {
		t.Error("expected error: non-numeric baseline length")
	}
	if _, err := generateDeclaredByLength(declaredByLengthParams{rates: rates, lengthValues: []string{"L48", "96"}, baseLM79: "L1", baselineIn: "48"}, "r1"); err == nil {
		t.Error("expected error: non-numeric covered length value")
	}
	out, err := generateDeclaredByLength(declaredByLengthParams{rates: rates, lengthValues: []string{"48", "96"}, baseLM79: "L1", baselineIn: "48"}, "r1")
	if err != nil {
		t.Fatalf("unexpected error on all-numeric lengths: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 derived row (96; baseline 48 excluded), got %d", len(out))
	}
}
