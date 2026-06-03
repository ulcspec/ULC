package sheet

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
	"github.com/ulcspec/ULC/tools/validator/internal/grade"
	"github.com/ulcspec/ULC/tools/validator/internal/index"
	"github.com/ulcspec/ULC/tools/validator/internal/validate"
)

// schemaDir locates the repo schema directory from the test's working dir
// (tools/validator/internal/sheet) by walking up to the module root.
func schemaDir(t *testing.T) string {
	t.Helper()
	dir, err := validate.FindSchemaDir("", "")
	if err != nil {
		t.Fatalf("locate schema dir: %v", err)
	}
	return dir
}

// TestConvertPatternAStandard runs the full increment-1 happy path: convert a
// one-record CSV bundle, build the index (stamping conformance_level), assert no
// required index key is missing, validate against the live ULC schema with zero
// ERROR findings, and assert the fixture earns the conformance level its data
// honestly carries.
func TestConvertPatternAStandard(t *testing.T) {
	bundle := filepath.Join("testdata", "bundle")

	results, err := Convert(bundle, Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	res := results[0]
	if res.RecordID != "acme-orbit-1200-4000k" {
		t.Fatalf("unexpected record_id %q", res.RecordID)
	}
	if res.Pattern != PatternA {
		t.Fatalf("expected Pattern A, got %s", res.Pattern)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("expected no warnings (files present on disk), got %v", res.Warnings)
	}

	// Build the index: this stamps the index block and grades conformance_level.
	built := index.Build(res.Record)
	if missing := index.MissingRequiredKeys(built); len(missing) > 0 {
		t.Fatalf("MissingRequiredKeys is not empty: %v", missing)
	}

	// The fixture authors the full standard field set, so it must grade standard.
	if got := grade.AchievedLevel(res.Record); got != grade.LevelStandard {
		t.Fatalf("expected conformance level standard, got %s", got)
	}
	if level, _ := built["conformance_level"].(string); level != "standard" {
		t.Fatalf("expected index.conformance_level standard, got %q", level)
	}

	// Validate against the live schema. The validator wants the json.Number-typed
	// tree, so round-trip the assembled record (with index) through the strict
	// decoder the way the from-sheet command does on the written file.
	res.Record["index"] = built
	tree := numberTree(t, res.Record)

	v, err := validate.NewValidator(schemaDir(t))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	report := findings.NewReport()
	v.Validate(tree, report)
	validate.VerifyHashes(filepath.Join("testdata", "bundle"), res.Record, report)
	report.Finalize()

	if report.HasErrors() {
		buf := &bytes.Buffer{}
		_ = report.WriteText(buf, res.RecordID)
		t.Fatalf("schema validation produced ERROR findings:\n%s", buf.String())
	}

	// Spot-check load-bearing converter outputs.
	assertCutsheetDualWrite(t, res.Record)
	assertDualUnit(t, res.Record)
	assertMeasuredAttestationRef(t, res.Record)
}

// numberTree re-encodes a Go-native record and decodes it with UseNumber so the
// schema validator sees json.Number, matching its documented input contract.
func numberTree(t *testing.T, record map[string]any) any {
	t.Helper()
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		t.Fatalf("decode record: %v", err)
	}
	return tree
}

func assertCutsheetDualWrite(t *testing.T, record map[string]any) {
	t.Helper()
	pf, _ := record["product_family"].(map[string]any)
	cut, _ := pf["cutsheet"].(map[string]any)
	cutName, _ := cut["filename"].(string)
	if cutName != "acme-orbit-1200-specs.pdf" {
		t.Fatalf("cutsheet filename wrong: %q", cutName)
	}
	if sha, _ := cut["sha256"].(string); len(sha) != 64 || sha == zeroSHA256 {
		t.Fatalf("cutsheet sha256 not a real 64-hex hash: %q", sha)
	}

	files, _ := record["source_files"].([]any)
	var datasheetCount int
	for _, f := range files {
		m, _ := f.(map[string]any)
		if ft, _ := m["file_type"].(string); ft == "datasheet_pdf" {
			datasheetCount++
			ref, _ := m["reference"].(map[string]any)
			if fn, _ := ref["filename"].(string); fn != "acme-orbit-1200-specs.pdf" {
				t.Fatalf("synthesized datasheet_pdf filename wrong: %q", fn)
			}
		}
	}
	if datasheetCount != 1 {
		t.Fatalf("expected exactly one synthesized datasheet_pdf source_files entry, got %d", datasheetCount)
	}
}

func assertDualUnit(t *testing.T, record map[string]any) {
	t.Helper()
	// The fixture authors no physical_dimensions columns, so synthesize a unit
	// check directly through buildDualUnit to guard the math and rounding.
	got := buildDualUnit(dualLength, 113, "rated", nil)
	if in, ok := got["in"]; !ok {
		t.Fatalf("dual-unit length missing companion 'in'")
	} else if f, _ := in.(float64); f != 4.4488 {
		t.Fatalf("expected in=4.4488 for mm=113, got %v", in)
	}
	mass := buildDualUnit(dualMass, 0.7, "rated", nil)
	if lb, _ := mass["lb"].(float64); lb != 1.5 {
		t.Fatalf("expected lb=1.5 for kg=0.7, got %v", mass["lb"])
	}
}

func assertMeasuredAttestationRef(t *testing.T, record map[string]any) {
	t.Helper()
	phot, _ := record["photometry"].(map[string]any)
	flux, _ := phot["total_luminous_flux_lm"].(map[string]any)
	if vt, _ := flux["value_type"].(string); vt != "measured" {
		t.Fatalf("flux value_type expected measured, got %q", vt)
	}
	prov, _ := flux["provenance"].(map[string]any)
	if ref, _ := prov["attestation_ref"].(string); ref != "lm79_acme_orbit_1200_4000k" {
		t.Fatalf("measured flux did not auto-link to the lm_79 attestation: %q", ref)
	}
}

// TestConvertPatternDRejected confirms B/C/D are detected and rejected with a
// clean error rather than silently mis-handled. A declared_by_length row flips
// the record to Pattern D.
func TestConvertPatternDRejected(t *testing.T) {
	dir := t.TempDir()
	writeFixtureCopy(t, dir)
	// Add a declared_by_length sheet to trip the Pattern D classifier.
	writeFile(t, filepath.Join(dir, "declared_by_length.csv"),
		"record_id,length_mm\nacme-orbit-1200-4000k,1200\n")

	_, err := Convert(dir, Options{})
	if err == nil {
		t.Fatalf("expected Pattern D rejection error, got nil")
	}
	if !contains(err.Error(), "Pattern D") || !contains(err.Error(), "not yet implemented") {
		t.Fatalf("expected a Pattern D not-yet-implemented error, got: %v", err)
	}
}

// TestAllowMissingFilesSentinel confirms a missing referenced file errors by
// default and stamps the zero sentinel (with a warning) under the flag.
func TestAllowMissingFilesSentinel(t *testing.T) {
	dir := t.TempDir()
	// Copy only the CSVs and the IES, deliberately omitting the cutsheet PDF.
	writeFile(t, filepath.Join(dir, "records.csv"), readFixture(t, "records.csv"))
	writeFile(t, filepath.Join(dir, "source_files.csv"), readFixture(t, "source_files.csv"))
	writeFile(t, filepath.Join(dir, "attestations.csv"), readFixture(t, "attestations.csv"))
	writeFile(t, filepath.Join(dir, "shared_attestations.csv"), readFixture(t, "shared_attestations.csv"))
	writeFile(t, filepath.Join(dir, "acme-orbit-1200-4000k.ies"), readFixture(t, "acme-orbit-1200-4000k.ies"))

	if _, err := Convert(dir, Options{}); err == nil {
		t.Fatalf("expected a missing-cutsheet error without --allow-missing-files")
	}

	results, err := Convert(dir, Options{AllowMissingFiles: true})
	if err != nil {
		t.Fatalf("Convert with AllowMissingFiles: %v", err)
	}
	if len(results[0].Warnings) == 0 {
		t.Fatalf("expected a missing-file warning under AllowMissingFiles")
	}
	pf, _ := results[0].Record["product_family"].(map[string]any)
	cut, _ := pf["cutsheet"].(map[string]any)
	if sha, _ := cut["sha256"].(string); sha != zeroSHA256 {
		t.Fatalf("expected zero-sentinel sha256 for missing cutsheet, got %q", sha)
	}
}
