package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/index"
)

// copyFlatDir copies a flat directory of files (the sheet bundle fixtures are flat)
// from src into dst, which must already exist.
func copyFlatDir(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), b, 0o644); err != nil {
			t.Fatalf("write %s: %v", e.Name(), err)
		}
	}
}

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

// missingDesignationRecord drops catalog_model: identity (the manufacturer name and
// the catalog designation) is the ONLY non-optional thing, so a record that cannot be
// identified is malformed, distinct from the incomplete floor.
const missingDesignationRecord = `{
  "ulc_version": "0.8.0",
  "record_id": "example-no-designation",
  "record_status": "announced",
  "index": { "x-ulc-generated": true },
  "product_family": {
    "family_id": "example-floor",
    "manufacturer": { "slug": "example", "display_name": "Example Manufacturer" }
  },
  "configuration": { "photometric_scenario_id": "floor-demo-default" },
  "source_files": []
}`

// TestCLIMissingIdentityIsRejected pins the floor's one hard requirement: a record
// missing its designation (catalog_model) is rejected, NOT graded incomplete. This is
// the boundary between "incomplete is a valid floor" and "the record cannot be
// identified," so build-index reports the missing required key and exits nonzero.
func TestCLIMissingIdentityIsRejected(t *testing.T) {
	dir := t.TempDir()
	recordPath := filepath.Join(dir, "no-designation.ulc")
	if err := os.WriteFile(recordPath, []byte(missingDesignationRecord), 0o644); err != nil {
		t.Fatalf("write record: %v", err)
	}
	if rc := runBuildIndex([]string{recordPath}); rc == 0 {
		t.Error("build-index exit = 0 for a record missing catalog_model; identity (name + designation) is required, so this is malformed, not incomplete")
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

// TestCLIFromSheetWritesIncompleteRecord pins the headline from-sheet promise at the
// CLI seam: a workbook record with no cutsheet is WRITTEN to --out (not skipped),
// exits 0, and lands with conformance_level "incomplete".
func TestCLIFromSheetWritesIncompleteRecord(t *testing.T) {
	bundleSrc := filepath.Join(repoRoot(t), "tools", "validator", "internal", "sheet", "testdata", "bundle")
	if _, err := os.Stat(bundleSrc); err != nil {
		t.Skipf("bundle fixture not available: %v", err)
	}
	bundleDir := filepath.Join(t.TempDir(), "bundle")
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	copyFlatDir(t, bundleSrc, bundleDir)
	// Blank the cutsheet so the converted record grades incomplete.
	recordsPath := filepath.Join(bundleDir, "records.csv")
	b, err := os.ReadFile(recordsPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recordsPath, []byte(strings.Replace(string(b), "acme-orbit-1200-specs.pdf", "", 1)), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := t.TempDir()
	if rc := runFromSheet([]string{"--out", outDir, bundleDir}); rc != 0 {
		t.Fatalf("from-sheet exit = %d, want 0 (an incomplete record is written, not skipped)", rc)
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read out dir: %v", err)
	}
	written := ""
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".json" {
			written = filepath.Join(outDir, e.Name())
		}
	}
	if written == "" {
		t.Fatal("from-sheet did not write the incomplete record to --out")
	}
	raw, err := os.ReadFile(written)
	if err != nil {
		t.Fatalf("read written record: %v", err)
	}
	var rec map[string]any
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("unmarshal written record: %v", err)
	}
	idx, _ := rec["index"].(map[string]any)
	if got := idx["conformance_level"]; got != "incomplete" {
		t.Errorf("written record conformance_level = %v, want \"incomplete\"", got)
	}
}

// TestAchievementsIndexRoundTrip is the P1 guard for the nested achievements subtree.
// A freshly built index, marshaled and re-read through the exact validate pipeline
// (decodeStrict + normalizeNumbers), must diff against the fresh build with ZERO drift.
// This exercises the stored(int64 / []any / map[string]any) vs built comparison that
// index.Diff actually performs (a nested map value falls to type-strict reflect.DeepEqual),
// and confirms every always-present empty array serializes as [] not null. valuesEqual is
// deliberately untouched: any drift here is a builder-side emission-type bug, not a
// comparator gap.
func TestAchievementsIndexRoundTrip(t *testing.T) {
	root := repoRoot(t)
	matches, err := filepath.Glob(filepath.Join(root, "examples", "*.ulc"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no examples/*.ulc found")
	}
	for _, path := range matches {
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			raw, err := decodeStrict(data)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			normalized, err := normalizeNumbers(raw)
			if err != nil {
				t.Fatalf("normalize: %v", err)
			}
			record := normalized.(map[string]any)
			freshBuild := index.Build(record)

			buf, err := json.Marshal(freshBuild)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			// Every always-present array must serialize as [] not null (a nil slice from
			// dedupeSortedStrings would fail here and fail the Diff below).
			for _, key := range []string{`"programs":null`, `"source_attestation_ids":null`, `"restricted_substances_declared":null`} {
				if strings.Contains(string(buf), key) {
					t.Fatalf("%s: an always-present array serialized as null (%s):\n%s", name, key, string(buf))
				}
			}

			reRaw, err := decodeStrict(buf)
			if err != nil {
				t.Fatalf("re-decode: %v", err)
			}
			reNorm, err := normalizeNumbers(reRaw)
			if err != nil {
				t.Fatalf("re-normalize: %v", err)
			}
			reread := reNorm.(map[string]any)

			if diffs := index.Diff(reread, freshBuild); len(diffs) > 0 {
				t.Errorf("%s: nested-index Diff round-trip drift (fix builder emission types, never valuesEqual):\n%s",
					name, strings.Join(diffs, "\n"))
			}
		})
	}
}

// TestManufacturerRecycleProgramIndexValidates pins the 5.7 sub-point the corpus cannot:
// a record whose only sustainability signal is a manufacturer_recycle_program declaration
// builds an index with circularity claimed and an EMPTY programs array, and that built
// index schema-validates (an empty AttestationProgram array, never a non-enum string).
func TestManufacturerRecycleProgramIndexValidates(t *testing.T) {
	var rec map[string]any
	if err := json.Unmarshal([]byte(identityOnlyRecord), &rec); err != nil {
		t.Fatalf("parse base record: %v", err)
	}
	rec["sustainability_declaration"] = map[string]any{"declaration_type": "manufacturer_recycle_program"}
	rec["index"] = index.Build(rec)

	ach, _ := rec["index"].(map[string]any)["achievements"].(map[string]any)
	themes, _ := ach["themes"].(map[string]any)
	circ, _ := themes["circularity"].(map[string]any)
	if circ["state"] != "claimed" {
		t.Errorf("circularity state = %v, want claimed", circ["state"])
	}
	programs, ok := circ["programs"].([]any)
	if !ok {
		t.Fatalf("circularity programs is %T, want []any", circ["programs"])
	}
	if len(programs) != 0 {
		t.Errorf("circularity programs = %v, want empty (no AttestationProgram token)", programs)
	}

	// The built record must schema-validate: an empty enum array is valid; a non-enum
	// string (the bug this guards against) would be a schema error.
	dir := t.TempDir()
	recordPath := filepath.Join(dir, "recycle.ulc")
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	if err := os.WriteFile(recordPath, data, 0o644); err != nil {
		t.Fatalf("write record: %v", err)
	}
	schemaDir := filepath.Join(repoRoot(t), "schema")
	if rc := runValidate([]string{"--schema-dir", schemaDir, recordPath}); rc != 0 {
		t.Errorf("validate exit = %d, want 0 (a manufacturer_recycle_program built index must schema-validate)", rc)
	}
}
