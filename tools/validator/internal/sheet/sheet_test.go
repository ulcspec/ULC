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

// convertOne runs Convert on a bundle, asserts exactly one result with the
// expected pattern, builds the index (asserting no missing required keys),
// validates against the live schema with zero ERROR findings, and asserts the
// achieved conformance level. It returns the assembled record (with index) for
// pattern-specific structural assertions. It is the shared spine of the B/C/D
// tests, mirroring TestConvertPatternAStandard.
func convertOne(t *testing.T, bundle string, wantPattern Pattern, wantLevel grade.Level) map[string]any {
	t.Helper()
	results, err := Convert(bundle, Options{})
	if err != nil {
		t.Fatalf("Convert(%s): %v", bundle, err)
	}
	if len(results) != 1 {
		t.Fatalf("%s: expected 1 result, got %d", bundle, len(results))
	}
	res := results[0]
	if res.Pattern != wantPattern {
		t.Fatalf("%s: expected pattern %s, got %s", bundle, wantPattern, res.Pattern)
	}

	built := index.Build(res.Record)
	if missing := index.MissingRequiredKeys(built); len(missing) > 0 {
		t.Fatalf("%s: MissingRequiredKeys is not empty: %v", bundle, missing)
	}
	if got := grade.AchievedLevel(res.Record); got != wantLevel {
		t.Fatalf("%s: expected conformance level %s, got %s", bundle, wantLevel, got)
	}
	if level, _ := built["conformance_level"].(string); level != wantLevel.String() {
		t.Fatalf("%s: expected index.conformance_level %s, got %q", bundle, wantLevel.String(), level)
	}

	res.Record["index"] = built
	tree := numberTree(t, res.Record)
	v, err := validate.NewValidator(schemaDir(t))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	report := findings.NewReport()
	v.Validate(tree, report)
	validate.VerifyHashes(bundle, res.Record, report)
	report.Finalize()
	if report.HasErrors() {
		buf := &bytes.Buffer{}
		_ = report.WriteText(buf, res.RecordID)
		t.Fatalf("%s: schema validation produced ERROR findings:\n%s", bundle, buf.String())
	}
	return res.Record
}

// TestConvertPatternB runs the multiplier-table path end to end: covered_axes +
// cct_multipliers + excluded_combinations join into an applicability block, and a
// generated photometry.declared_by_cct[] derives each CCT's lumens from
// round(multiplier * measured_baseline_flux). The fixture grades full (outdoor
// with bug_rating and an operating_point).
func TestConvertPatternB(t *testing.T) {
	record := convertOne(t, filepath.Join("testdata", "bundle-b"), PatternB, grade.LevelFull)

	// Structural fact 1: the CCT multiplier table is present on the covered axis.
	table, ok := getPath(record, "applicability.covered_axes.cct.derivation.multiplier_table")
	if !ok {
		t.Fatalf("Pattern B: applicability.covered_axes.cct.derivation.multiplier_table missing")
	}
	tableMap, _ := table.(map[string]any)
	if len(tableMap) != 4 {
		t.Fatalf("Pattern B: expected 4 multiplier-table entries, got %d", len(tableMap))
	}

	// Structural fact 2: a generated declared_by_cct entry equals
	// round(multiplier * baseline). Baseline measured flux is 4200 lm.
	// 4000K multiplier is 1.07 -> round(1.07 * 4200) = round(4494) = 4494.
	declared, ok := getPath(record, "photometry.declared_by_cct")
	if !ok {
		t.Fatalf("Pattern B: photometry.declared_by_cct missing")
	}
	got4000 := declaredByCCTValue(t, declared, "4000")
	if got4000 != 4494 {
		t.Fatalf("Pattern B: declared_by_cct[4000].lumens.value = %v, want round(1.07*4200)=4494", got4000)
	}
	// The baseline 3000K entry equals round(1.00 * 4200) == 4200 and carries
	// measured provenance (it is the tested baseline, not a scaled derivative).
	got3000 := declaredByCCTValue(t, declared, "3000")
	if got3000 != 4200 {
		t.Fatalf("Pattern B: declared_by_cct[3000].lumens.value = %v, want round(1.00*4200)=4200", got3000)
	}
	assertDeclaredCCTBaselineMeasured(t, declared, "3000")
}

// TestConvertPatternC runs the per-IES-with-provenance path: structurally a
// fixed-axes pin like A, but the headline photometry carries derived provenance
// (method=extended_photometry + base_attestation_ref) supplied via the per-column
// provenance override columns. The extensions_json overflow lands at
// extensions.manufacturer_specific.<slug>.
func TestConvertPatternC(t *testing.T) {
	record := convertOne(t, filepath.Join("testdata", "bundle-c"), PatternC, grade.LevelCore)

	// Structural fact: a headline photometric value carries
	// provenance.method=extended_photometry + base_attestation_ref.
	flux, ok := getPath(record, "photometry.total_luminous_flux_lm.provenance")
	if !ok {
		t.Fatalf("Pattern C: photometry.total_luminous_flux_lm.provenance missing")
	}
	prov, _ := flux.(map[string]any)
	if m, _ := prov["method"].(string); m != "extended_photometry" {
		t.Fatalf("Pattern C: flux provenance method = %q, want extended_photometry", m)
	}
	if base, _ := prov["base_attestation_ref"].(string); base == "" {
		t.Fatalf("Pattern C: flux provenance base_attestation_ref missing")
	}
	// The value_type must be rated (not measured), so no attestation_ref auto-link
	// fires (extended photometry is a derivative, not a direct measurement).
	if vt, _ := getPath(record, "photometry.total_luminous_flux_lm.value_type"); vt != "rated" {
		t.Fatalf("Pattern C: flux value_type = %v, want rated", vt)
	}
	// The vendor-data overflow landed under the manufacturer slug.
	if _, ok := getPath(record, "extensions.manufacturer_specific.lumenpulse.ies_candela_multiplier"); !ok {
		t.Fatalf("Pattern C: extensions.manufacturer_specific.lumenpulse overflow missing")
	}
}

// TestConvertPatternD runs the per-foot-linear path: the covered length axis
// carries a per_foot_linear_scaling derivation and a generated
// photometry.declared_by_length[] derives each non-baseline length's lumens and
// input power from the per-foot rates. The fixture omits the declared_by_length
// sheet, so the table is generated.
func TestConvertPatternD(t *testing.T) {
	record := convertOne(t, filepath.Join("testdata", "bundle-d"), PatternD, grade.LevelStandard)

	declared, ok := getPath(record, "photometry.declared_by_length")
	if !ok {
		t.Fatalf("Pattern D: photometry.declared_by_length missing")
	}
	arr, _ := declared.([]any)
	if len(arr) == 0 {
		t.Fatalf("Pattern D: declared_by_length is empty (expected generated rows)")
	}
	// Rates: 300 lm/ft, 6 W/ft. The 60 in (5 ft) row scales to 1500 lm, 30 W.
	got := declaredByLengthLumens(t, declared, 1524) // 60 in = 1524 mm
	if got != 1500 {
		t.Fatalf("Pattern D: declared_by_length[60in].lumens.value = %v, want round(300*5)=1500", got)
	}
	// The covered length axis carries the per_foot_linear_scaling derivation.
	if m, _ := getPath(record, "applicability.covered_axes.length.derivation.method"); m != "per_foot_linear_scaling" {
		t.Fatalf("Pattern D: covered_axes.length.derivation.method = %v, want per_foot_linear_scaling", m)
	}
}

// TestConvertPatternDAuthoredLengthSheet confirms the authored declared_by_length
// sheet wins over generation (DESIGN.md decision 3): its rows are echoed verbatim
// and a row that diverges from the per-foot projection by more than 2% raises a
// warning. The injected 60 in row claims 2000 lm against a 1500 lm projection
// (33% high), so it must be echoed as 2000 and produce a divergence warning.
func TestConvertPatternDAuthoredLengthSheet(t *testing.T) {
	dir := t.TempDir()
	copyBundle(t, filepath.Join("testdata", "bundle-d"), dir)
	writeFile(t, filepath.Join(dir, "declared_by_length.csv"),
		"record_id,length_mm,lumens_value,input_power_w_value\n"+
			"vode-nexa-807-so-3500k-90cri-hl-black-48in,1524,2000,30\n")

	results, err := Convert(dir, Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	res := results[0]
	if res.Pattern != PatternD {
		t.Fatalf("expected pattern D, got %s", res.Pattern)
	}

	// The authored lumens (2000) are echoed verbatim, not regenerated to 1500.
	declared, ok := getPath(res.Record, "photometry.declared_by_length")
	if !ok {
		t.Fatalf("declared_by_length missing")
	}
	if got := declaredByLengthLumens(t, declared, 1524); got != 2000 {
		t.Fatalf("authored declared_by_length[60in].lumens.value = %v, want echoed 2000", got)
	}

	// The >2% divergence raised a warning.
	var warned bool
	for _, w := range res.Warnings {
		if contains(w, "diverges") && contains(w, "lumens") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("expected a declared_by_length divergence warning, got %v", res.Warnings)
	}
}

// declaredByCCTValue returns the lumens.value of the declared_by_cct entry for
// the given cct, as an int64.
func declaredByCCTValue(t *testing.T, declared any, cct string) int64 {
	t.Helper()
	arr, _ := declared.([]any)
	for _, e := range arr {
		m, _ := e.(map[string]any)
		if c, _ := m["cct"].(string); c == cct {
			lum, _ := m["lumens"].(map[string]any)
			return asInt64(t, lum["value"])
		}
	}
	t.Fatalf("declared_by_cct has no entry for cct %q", cct)
	return 0
}

// assertDeclaredCCTBaselineMeasured asserts the baseline CCT entry carries
// measured provenance (the tested anchor), not the scaled derivative provenance.
func assertDeclaredCCTBaselineMeasured(t *testing.T, declared any, cct string) {
	t.Helper()
	arr, _ := declared.([]any)
	for _, e := range arr {
		m, _ := e.(map[string]any)
		if c, _ := m["cct"].(string); c == cct {
			lum, _ := m["lumens"].(map[string]any)
			if vt, _ := lum["value_type"].(string); vt != "measured" {
				t.Fatalf("declared_by_cct[%s] value_type = %q, want measured (baseline)", cct, vt)
			}
			prov, _ := lum["provenance"].(map[string]any)
			if ref, _ := prov["attestation_ref"].(string); ref == "" {
				t.Fatalf("declared_by_cct[%s] baseline missing attestation_ref", cct)
			}
			return
		}
	}
	t.Fatalf("declared_by_cct has no entry for cct %q", cct)
}

// declaredByLengthLumens returns the lumens.value of the declared_by_length entry
// whose length.mm equals lengthMM, as an int64.
func declaredByLengthLumens(t *testing.T, declared any, lengthMM int64) int64 {
	t.Helper()
	arr, _ := declared.([]any)
	for _, e := range arr {
		m, _ := e.(map[string]any)
		length, _ := m["length"].(map[string]any)
		if asInt64(t, length["mm"]) == lengthMM {
			lum, _ := m["lumens"].(map[string]any)
			return asInt64(t, lum["value"])
		}
	}
	t.Fatalf("declared_by_length has no entry for length %d mm", lengthMM)
	return 0
}

// asInt64 coerces a JSON-decoded numeric (int64 or float64) to int64 for exact
// integer comparison in assertions.
func asInt64(t *testing.T, v any) int64 {
	t.Helper()
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		t.Fatalf("value %v (%T) is not numeric", v, v)
		return 0
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
