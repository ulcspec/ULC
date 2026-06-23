package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// repoRoot resolves the repository root from this package directory
// (tools/validator/cmd/ulc -> up four levels).
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

// identityOnlyRecord is a schema-valid record with identity and zero source
// documents: the canonical floor case. It carries a stub index that build-index
// regenerates.
const identityOnlyRecord = `{
  "ulc_version": "0.8.0",
  "record_id": "example-incomplete-floor",
  "record_status": "announced",
  "index": { "x-ulc-generated": true },
  "product_family": {
    "family_id": "example-floor",
    "manufacturer": { "slug": "example", "display_name": "Example Manufacturer" },
    "catalog_model": "Floor Demo"
  },
  "configuration": { "photometric_scenario_id": "floor-demo-default" },
  "source_files": []
}`

// TestCLIFloorExitsZero pins the headline v0.8.0 contract at the CLI layer: an
// identity-only record (zero source documents) is a valid, expected outcome.
// build-index stamps conformance_level "incomplete" and exits 0; validate then
// exits 0 (the floor is never a data-completeness failure).
func TestCLIFloorExitsZero(t *testing.T) {
	dir := t.TempDir()
	recordPath := filepath.Join(dir, "floor.ulc")
	if err := os.WriteFile(recordPath, []byte(identityOnlyRecord), 0o644); err != nil {
		t.Fatalf("write record: %v", err)
	}

	// build-index writes the index in place and must succeed (exit 0).
	if rc := runBuildIndex([]string{recordPath}); rc != 0 {
		t.Fatalf("build-index exit = %d, want 0 (incomplete is not a failure)", rc)
	}

	// The stamped grade is the floor.
	raw, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("re-read record: %v", err)
	}
	var rec map[string]any
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("unmarshal record: %v", err)
	}
	idx, _ := rec["index"].(map[string]any)
	if got := idx["conformance_level"]; got != "incomplete" {
		t.Errorf("conformance_level = %v, want \"incomplete\"", got)
	}

	// validate must exit 0 for a schema-valid record at the floor.
	schemaDir := filepath.Join(repoRoot(t), "schema")
	if rc := runValidate([]string{"--schema-dir", schemaDir, recordPath}); rc != 0 {
		t.Errorf("validate exit = %d, want 0 (incomplete is a valid, expected outcome)", rc)
	}
}

// TestCLIFromSheetWritesRecord guards the from-sheet write path: a converted record
// is WRITTEN to --out and the run exits 0 (the converter no longer skips records on
// data completeness). Uses the canonical CSV bundle fixture, whose referenced files
// resolve against the bundle directory by default.
func TestCLIFromSheetWritesRecord(t *testing.T) {
	bundleDir := filepath.Join(repoRoot(t), "tools", "validator", "internal", "sheet", "testdata", "bundle")
	if _, err := os.Stat(bundleDir); err != nil {
		t.Skipf("bundle fixture not available: %v", err)
	}
	outDir := t.TempDir()

	if rc := runFromSheet([]string{"--out", outDir, bundleDir}); rc != 0 {
		t.Fatalf("from-sheet exit = %d, want 0", rc)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read out dir: %v", err)
	}
	wrote := 0
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			wrote++
		}
	}
	if wrote == 0 {
		t.Error("from-sheet wrote no records to --out; the converter should write, not skip")
	}
}
