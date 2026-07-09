package sheet

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/completeness"
	"github.com/ulcspec/ULC/tools/validator/internal/findings"
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

// TestConvertPatternA runs the full increment-1 happy path: convert a
// one-record CSV bundle, build the index (stamping conformance_level), assert no
// required index key is missing, validate against the live ULC schema with zero
// ERROR findings, and assert the fixture earns the conformance level its data
// honestly carries.
func TestConvertPatternA(t *testing.T) {
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
	// A converted record with no ulc_version column stamps the current-spec default.
	if got, _ := res.Record["ulc_version"].(string); got != "1.0.0" {
		t.Fatalf("converted record ulc_version = %q, want 1.0.0 (from-sheet default)", got)
	}

	// Build the index: this stamps the index block and grades conformance_level.
	built := index.Build(res.Record)
	if missing := index.MissingRequiredKeys(built); len(missing) > 0 {
		t.Fatalf("MissingRequiredKeys is not empty: %v", missing)
	}

	// This minimal Pattern A fixture grades core under the redesigned rubric: it
	// authors identity, headline numbers, one-line colorimetry, and a safety
	// listing (core), but a complete standard record additionally needs the SDCM
	// step, lens material, and the analog dimming method and range. This bundle
	// authors none of those, so core is the honest grade for its data.
	if got := completeness.AchievedLevel(res.Record); got != completeness.LevelCore {
		t.Fatalf("expected conformance level core, got %s", got)
	}
	if level, _ := built["conformance_level"].(string); level != "core" {
		t.Fatalf("expected index.conformance_level core, got %q", level)
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
// tests, mirroring TestConvertPatternA.
func convertOne(t *testing.T, bundle string, wantPattern Pattern, wantLevel completeness.Level) map[string]any {
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
	if got := completeness.AchievedLevel(res.Record); got != wantLevel {
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
// round(multiplier * measured_baseline_flux). Under the redesigned rubric the
// fixture grades standard: it authors the full standard surface for an analog
// fixture, including the SDCM step and the dimming method and range (the records
// sheet now carries the dimming_range_min/max_percent and dimming_curve columns).
// Full would additionally need test-report depth the converter does not synthesize.
// The multiplier-table and dimming assertions below are the point of this test.
func TestConvertPatternB(t *testing.T) {
	record := convertOne(t, filepath.Join("testdata", "bundle-b"), PatternB, completeness.LevelStandard)

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

	// Structural fact 3: the analog dimming detail authored on the records sheet
	// lands in the electrical block, which is what lifts this analog fixture to
	// standard. The dimming_range_percent object carries both halves, and the new
	// dimming_curve enum column writes through.
	rng, ok := getPath(record, "electrical.dimming_range_percent")
	if !ok {
		t.Fatalf("Pattern B: electrical.dimming_range_percent missing")
	}
	rngMap, _ := rng.(map[string]any)
	if _, has := rngMap["min"]; !has {
		t.Fatalf("Pattern B: electrical.dimming_range_percent.min missing")
	}
	if _, has := rngMap["max"]; !has {
		t.Fatalf("Pattern B: electrical.dimming_range_percent.max missing")
	}
	if got, _ := getPath(record, "electrical.dimming_curve"); got != "logarithmic" {
		t.Fatalf("Pattern B: electrical.dimming_curve = %v, want logarithmic", got)
	}
}

// TestDimmingRangeRequiresBothHalves proves the schema rejects a one-sided
// dimming_range_percent. The converter authors min and max from two independent
// records-sheet columns (dimming_range_min_percent / dimming_range_max_percent, the
// bug_rating b/u/g pattern), so a blank companion cell must not slip a half-filled
// object past validation. It starts from the bundle-b record, which carries a complete
// {min,max} range and validates clean, then drops one half and confirms the schema
// now errors. The baseline-clean assertion makes the failure attributable to the range.
func TestDimmingRangeRequiresBothHalves(t *testing.T) {
	results, err := Convert(filepath.Join("testdata", "bundle-b"), Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	record := results[0].Record
	record["index"] = index.Build(record)

	v, err := validate.NewValidator(schemaDir(t))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}

	base := findings.NewReport()
	v.Validate(numberTree(t, record), base)
	base.Finalize()
	if base.HasErrors() {
		t.Fatalf("baseline bundle-b record should validate clean before mutation")
	}

	el, _ := record["electrical"].(map[string]any)
	for _, half := range []map[string]any{
		{"min": float64(1)},   // max omitted
		{"max": float64(100)}, // min omitted
	} {
		el["dimming_range_percent"] = half
		report := findings.NewReport()
		v.Validate(numberTree(t, record), report)
		report.Finalize()
		if !report.HasErrors() {
			t.Errorf("half-filled dimming_range_percent %v validated clean; schema must require both min and max", half)
		}
	}
}

// TestConvertPatternC runs the per-IES-with-provenance path: structurally a
// fixed-axes pin like A, but the headline photometry carries derived provenance
// (method=extended_photometry + base_attestation_ref) supplied via the per-column
// provenance override columns. The extensions_json overflow lands at
// extensions.manufacturer_specific.<slug>. This bundle is the DMX/RDM color-mixing
// Lumenfacade SKU, so it grades standard: a digital protocol is exempt from the
// analog/phase dimming detail, and pure RGB waives the white-light color gates.
func TestConvertPatternC(t *testing.T) {
	record := convertOne(t, filepath.Join("testdata", "bundle-c"), PatternC, completeness.LevelStandard)

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
	// Grades core under the redesigned rubric: a white-light linear fixture whose
	// standard gaps are the SDCM step and the dimming range, neither authored on
	// this bundle's sheet. The declared_by_length derivation below is the point of
	// the test.
	record := convertOne(t, filepath.Join("testdata", "bundle-d"), PatternD, completeness.LevelCore)

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

// TestConvertFullLevelSheets exercises the optional comprehensive long sheets on
// the bundle-b fixture (an outdoor area light): the alpha_opic, flicker_metrics,
// lumen_maintenance_package, zonal_lumens, lcs_zonal_lumens, ingredient_list, and
// cie97_lmf / cie97_llmf sheets each assemble into their ULC block, and convertOne
// re-validates the enriched record against the live schema with zero ERROR
// findings. The "full-level" name refers to these comprehensive sheets, not the
// conformance grade: under the redesigned rubric the converted record grades
// standard (it authors the analog dimming method and range), and full additionally
// needs test-report depth (uncertainty, corrections, instrumentation, TM-30) the
// converter does not synthesize. Several of these blocks (zonal_lumens, the
// method-backed lumen-maintenance package) feed full-tier rules, but they cannot
// lift the grade alone.
func TestConvertFullLevelSheets(t *testing.T) {
	record := convertOne(t, filepath.Join("testdata", "bundle-b"), PatternB, completeness.LevelStandard)

	// alpha_opic_metrics: block scalars + a single melanopic per_channel efficacy,
	// both rated ProvenancedNumbers (no attestation link).
	if v, _ := getPath(record, "alpha_opic_metrics.reference_illuminant"); v != "d65" {
		t.Fatalf("alpha_opic_metrics.reference_illuminant = %v, want d65", v)
	}
	if v, ok := getPath(record, "alpha_opic_metrics.melanopic_der.value"); !ok || asFloat(t, v) != 0.54 {
		t.Fatalf("alpha_opic_metrics.melanopic_der.value = %v, want 0.54", v)
	}
	if arr := arrayAt(t, record, "alpha_opic_metrics.per_channel"); len(arr) != 1 {
		t.Fatalf("alpha_opic_metrics.per_channel len = %d, want 1", len(arr))
	} else if ch, _ := arr[0].(map[string]any); ch["channel"] != "melanopic" {
		t.Fatalf("alpha_opic per_channel[0].channel = %v, want melanopic", ch["channel"])
	}

	// flicker_measurements.metrics: two rated metrics with bound operators.
	if arr := arrayAt(t, record, "flicker_measurements.metrics"); len(arr) != 2 {
		t.Fatalf("flicker_measurements.metrics len = %d, want 2", len(arr))
	}

	// lumen_maintenance_package: a TOP-LEVEL array with a transcribed TM-21
	// projection (the method-backed projection, not a bare claim).
	pkg, _ := record["lumen_maintenance_package"].([]any)
	if len(pkg) != 1 {
		t.Fatalf("lumen_maintenance_package len = %d, want 1", len(pkg))
	}
	pkg0, _ := pkg[0].(map[string]any)
	tm21, _ := pkg0["tm_21_projection_hours"].(map[string]any)
	if asInt64(t, tm21["value"]) != 60000 {
		t.Fatalf("lumen_maintenance_package[0].tm_21_projection_hours.value = %v, want 60000", tm21["value"])
	}

	// photometry.zonal_lumens: measured, auto-linked to the single LM-79 anchor.
	zonal := arrayAt(t, record, "photometry.zonal_lumens")
	if len(zonal) != 4 {
		t.Fatalf("photometry.zonal_lumens len = %d, want 4", len(zonal))
	}
	assertMeasuredLumens(t, zonal[0], "lm79_lumos_skyline_sr_ho")

	// outdoor_classification.lcs_zonal_lumens: the 10 TM-15 LCS zones, measured.
	lcs := arrayAt(t, record, "outdoor_classification.lcs_zonal_lumens")
	if len(lcs) != 10 {
		t.Fatalf("outdoor_classification.lcs_zonal_lumens len = %d, want 10", len(lcs))
	}
	assertMeasuredLumens(t, lcs[0], "lm79_lumos_skyline_sr_ho")

	// sustainability_declaration: ingredient roster from the long sheet plus the
	// block scalars from the records columns (bool + int coercion).
	if arr := arrayAt(t, record, "sustainability_declaration.ingredient_list"); len(arr) != 4 {
		t.Fatalf("sustainability_declaration.ingredient_list len = %d, want 4", len(arr))
	}
	if v, _ := getPath(record, "sustainability_declaration.declaration_type"); v != "red_list_approved" {
		t.Fatalf("sustainability_declaration.declaration_type = %v, want red_list_approved", v)
	}
	if v, _ := getPath(record, "sustainability_declaration.lbc_criteria_compliance"); v != true {
		t.Fatalf("sustainability_declaration.lbc_criteria_compliance = %v (%T), want bool true", v, v)
	}
	if v, _ := getPath(record, "sustainability_declaration.recyclable_percent"); asInt64(t, v) != 88 {
		t.Fatalf("sustainability_declaration.recyclable_percent = %v, want 88", v)
	}

	// cie_97_lmf_table: 12 LMF rows + 4 LLMF rows; the lmf/llmf leaves are BARE
	// numbers (not ProvenancedNumber), and cleaning_interval_years is an integer.
	lmf := arrayAt(t, record, "lumen_maintenance_luminaire.cie_97_lmf_table.lmf_by_cleanliness_and_interval")
	if len(lmf) != 12 {
		t.Fatalf("cie_97_lmf_table.lmf_by_cleanliness_and_interval len = %d, want 12", len(lmf))
	}
	lmf0, _ := lmf[0].(map[string]any)
	if asInt64(t, lmf0["cleaning_interval_years"]) != 1 || lmf0["ambient_cleanliness"] != "pure" || asFloat(t, lmf0["lmf"]) != 0.96 {
		t.Fatalf("cie_97 lmf[0] = %v, want {1, pure, 0.96}", lmf0)
	}
	if _, isMap := lmf0["lmf"].(map[string]any); isMap {
		t.Fatalf("cie_97 lmf must be a bare number, not a ProvenancedNumber object")
	}
	if llmf := arrayAt(t, record, "lumen_maintenance_luminaire.cie_97_lmf_table.llmf_by_hours"); len(llmf) != 4 {
		t.Fatalf("cie_97_lmf_table.llmf_by_hours len = %d, want 4", len(llmf))
	}
}

// arrayAt returns the []any at a dotted path, failing the test if absent or not
// an array.
func arrayAt(t *testing.T, record map[string]any, path string) []any {
	t.Helper()
	v, ok := getPath(record, path)
	if !ok {
		t.Fatalf("%s missing", path)
	}
	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("%s is %T, want []any", path, v)
	}
	return arr
}

// assertMeasuredLumens asserts a zonal entry carries a measured lumens
// ProvenancedNumber whose provenance auto-linked to the expected LM-79 id.
func assertMeasuredLumens(t *testing.T, entry any, wantRef string) {
	t.Helper()
	m, _ := entry.(map[string]any)
	lum, _ := m["lumens"].(map[string]any)
	if vt, _ := lum["value_type"].(string); vt != "measured" {
		t.Fatalf("zonal lumens value_type = %q, want measured", vt)
	}
	prov, _ := lum["provenance"].(map[string]any)
	if ref, _ := prov["attestation_ref"].(string); ref != wantRef {
		t.Fatalf("zonal lumens attestation_ref = %q, want %q", ref, wantRef)
	}
}

// asFloat coerces a JSON-decoded numeric (int64 or float64) to float64.
func asFloat(t *testing.T, v any) float64 {
	t.Helper()
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		t.Fatalf("value %v (%T) is not numeric", v, v)
		return 0
	}
}

// TestParseIntCell guards the integral-float tolerance: a spreadsheet exporter
// may serialize an integer cell as "1.0" or "1e1", which must parse to the same
// int64 as "1"; a genuine non-integer must still be rejected.
func TestParseIntCell(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		ok   bool
	}{
		{"3", 3, true}, {"1", 1, true}, {"60000", 60000, true},
		{"1.0", 1, true}, {"2.0", 2, true}, {"1e1", 10, true},
		{"1.5", 0, false}, {"abc", 0, false}, {"", 0, false},
	}
	for _, c := range cases {
		got, err := parseIntCell(c.in)
		if c.ok {
			if err != nil || got != c.want {
				t.Errorf("parseIntCell(%q) = (%d, %v), want (%d, nil)", c.in, got, err, c.want)
			}
		} else if err == nil {
			t.Errorf("parseIntCell(%q) = (%d, nil), want error", c.in, got)
		}
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

// TestConvertCutsheetOptionalGradesIncomplete confirms the cutsheet is a graded
// core requirement, not a converter requirement: a workbook record with an empty
// cutsheet_file converts successfully (no hard fail), omits product_family.cutsheet
// and the synthesized datasheet_pdf source-file entry, and grades incomplete.
func TestConvertCutsheetOptionalGradesIncomplete(t *testing.T) {
	dir := t.TempDir()
	writeFixtureCopy(t, dir)
	// Blank the cutsheet_file value so the record carries no cutsheet.
	recordsCSV := strings.Replace(readFixture(t, "records.csv"), "acme-orbit-1200-specs.pdf", "", 1)
	writeFile(t, filepath.Join(dir, "records.csv"), recordsCSV)

	results, err := Convert(dir, Options{})
	if err != nil {
		t.Fatalf("Convert without a cutsheet should succeed (cutsheet is graded, not required): %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no records converted")
	}
	rec := results[0].Record

	pf, _ := rec["product_family"].(map[string]any)
	if _, present := pf["cutsheet"]; present {
		t.Error("product_family.cutsheet should be absent when cutsheet_file is empty")
	}
	for _, sf := range rec["source_files"].([]any) {
		if m, ok := sf.(map[string]any); ok && m["file_type"] == "datasheet_pdf" {
			t.Error("no datasheet_pdf source_files entry should be synthesized without a cutsheet")
		}
	}

	// End to end: build the index and confirm the cutsheet-less record grades the floor.
	rec["index"] = index.Build(rec)
	if got := completeness.AchievedLevel(rec); got != completeness.LevelIncomplete {
		t.Errorf("cutsheet-less record grades %s, want incomplete", got)
	}
}
