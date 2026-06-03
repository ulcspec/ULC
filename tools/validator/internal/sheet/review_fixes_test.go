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
	row := Row{"lumens": "1200", "lumens__value_type": "rated", "lumens__prov_method": "scaled"}

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
