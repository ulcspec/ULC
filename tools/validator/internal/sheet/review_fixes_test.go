package sheet

import (
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
