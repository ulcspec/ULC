package validate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	embedded "github.com/ulcspec/ULC/schema"
	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// TestValidatorAcceptsExampleRecords is the load-bearing positive test: each
// canonical reference record must validate clean against the schema, with no
// ERROR-level findings. A failure here means either the schema or the
// validator wiring (including cross-file $ref resolution) has regressed.
func TestValidatorAcceptsExampleRecords(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	examples := filepath.Join(root, "examples")
	matches, err := filepath.Glob(filepath.Join(examples, "*.ulc"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no .ulc files under %s", examples)
	}
	for _, path := range matches {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			doc := loadOrFail(t, path)
			report := findings.NewReport()
			v.Validate(doc, report)
			if report.HasErrors() {
				for _, f := range report.Findings {
					t.Logf("%s: %s", f.Level, f.Message)
				}
				t.Fatalf("%s: expected zero schema errors, got %d", path, len(report.Findings))
			}
		})
	}
}

// TestValidatorRejectsBrokenRecord asserts the validator catches a violation
// introduced to one of the canonical records (e.g., wiping a required field).
// This guards against silently-accepting-everything bugs in the wiring.
func TestValidatorRejectsBrokenRecord(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	// Start from a known-good record, then delete a required top-level field
	// (ulc_version) to force a schema violation.
	doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
	m, ok := doc.(map[string]any)
	if !ok {
		t.Fatalf("record is not an object")
	}
	delete(m, "ulc_version")

	report := findings.NewReport()
	v.Validate(m, report)
	if !report.HasErrors() {
		t.Fatalf("expected at least one schema error after deleting ulc_version, got none")
	}
	// Every finding at this stage should be a schema-violation code.
	for _, f := range report.Findings {
		if f.Code != findings.CodeSchemaViolation {
			t.Errorf("unexpected finding code %q, want %q", f.Code, findings.CodeSchemaViolation)
		}
	}
}

// TestValidatorRejectsTopLevelConformanceLevel asserts the breaking change is
// enforced: the level is computed into index.conformance_level, so a hand-authored
// top-level conformance_level is a hard schema error, not silently accepted.
func TestValidatorRejectsTopLevelConformanceLevel(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
	m, ok := doc.(map[string]any)
	if !ok {
		t.Fatalf("record is not an object")
	}
	m["conformance_level"] = "full"

	report := findings.NewReport()
	v.Validate(m, report)
	if !report.HasErrors() {
		t.Fatalf("expected a schema error for a top-level conformance_level, got none")
	}
	for _, f := range report.Findings {
		if f.Code != findings.CodeSchemaViolation {
			t.Errorf("unexpected finding code %q, want %q", f.Code, findings.CodeSchemaViolation)
		}
	}
}

// TestValidatorConstrainsPhotometryFormatToPhotometricFiles asserts the v0.9.0
// SourceFile conditional: photometry_format is only valid on a photometric source
// file (ies / ldt / tm33). A photometry_format on a non-photometric entry (for example
// datasheet_pdf) is a schema error; on an ies entry it validates.
func TestValidatorConstrainsPhotometryFormatToPhotometricFiles(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	load := func() map[string]any {
		doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
		m, ok := doc.(map[string]any)
		if !ok {
			t.Fatalf("record is not an object")
		}
		return m
	}
	setFormatOn := func(m map[string]any, fileType string) {
		sf, _ := m["source_files"].([]any)
		for _, e := range sf {
			if entry, ok := e.(map[string]any); ok && entry["file_type"] == fileType {
				entry["photometry_format"] = "lm63_2019"
				return
			}
		}
		t.Fatalf("no source_files entry of type %q to mutate", fileType)
	}

	// Bad: photometry_format on a datasheet_pdf entry -> schema error.
	bad := load()
	setFormatOn(bad, "datasheet_pdf")
	rBad := findings.NewReport()
	v.Validate(bad, rBad)
	if !rBad.HasErrors() {
		t.Error("expected a schema error for photometry_format on a datasheet_pdf source file, got none")
	}

	// Good: photometry_format on an ies entry -> valid.
	good := load()
	setFormatOn(good, "ies")
	rGood := findings.NewReport()
	v.Validate(good, rGood)
	if rGood.HasErrors() {
		t.Errorf("photometry_format on an ies source file must validate; got: %+v", rGood.Findings)
	}
}

// TestValidatorConstrainsDirectionalIndicator asserts the v0.10.0 exit_sign
// directional_indicator constraint: "none" (no chevron) is mutually exclusive with any real
// direction, and the array carries no duplicate tokens. ["none"] and a direction-only array
// validate; the contradictory ["none","left"] and the duplicate ["left","left"] are errors.
func TestValidatorConstrainsDirectionalIndicator(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	load := func() map[string]any {
		doc := loadOrFail(t, filepath.Join(root, "examples", "cooper-sure-lites-es61src.ulc"))
		m, ok := doc.(map[string]any)
		if !ok {
			t.Fatalf("record is not an object")
		}
		return m
	}
	cases := []struct {
		name    string
		vals    []any
		wantErr bool
	}{
		{"none-only", []any{"none"}, false},
		{"directions-only", []any{"left", "right"}, false},
		{"none-with-direction", []any{"none", "left"}, true},
		{"duplicate-direction", []any{"left", "left"}, true},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			m := load()
			m["exit_sign"].(map[string]any)["directional_indicator"] = append([]any{}, c.vals...)
			r := findings.NewReport()
			v.Validate(m, r)
			if c.wantErr && !r.HasErrors() {
				t.Errorf("expected a schema error for directional_indicator %v, got none", c.vals)
			}
			if !c.wantErr && r.HasErrors() {
				t.Errorf("directional_indicator %v must validate; got errors: %+v", c.vals, r.Findings)
			}
		})
	}
}

// TestValidatorEnforcesEmergencyBlockContract asserts the emergency block's required
// members are schema-enforced (emergency_role and power_source): an empty block, or a
// block missing either member, is a schema violation whose message names the missing
// member; a block carrying both validates. The class contract in §2.4 depends on this:
// the power_source completeness gate keys on the leaf, so block-absent and
// block-present-but-invalid must both be catchable, and required-ness is what makes a
// present block always carry its two discriminators.
func TestValidatorEnforcesEmergencyBlockContract(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	load := func() map[string]any {
		doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
		m, ok := doc.(map[string]any)
		if !ok {
			t.Fatalf("record is not an object")
		}
		return m
	}
	messages := func(r *findings.Report) string {
		var b strings.Builder
		for _, f := range r.Findings {
			b.WriteString(f.Path)
			b.WriteByte(' ')
			b.WriteString(f.Message)
			b.WriteByte('\n')
		}
		return b.String()
	}

	t.Run("empty emergency block fails naming both members", func(t *testing.T) {
		m := load()
		m["emergency"] = map[string]any{}
		r := findings.NewReport()
		v.Validate(m, r)
		if !r.HasErrors() {
			t.Fatalf("expected a schema error for emergency: {}, got none")
		}
		msg := messages(r)
		for _, member := range []string{"emergency_role", "power_source"} {
			if !strings.Contains(msg, member) {
				t.Errorf("expected the violation to name %q; findings:\n%s", member, msg)
			}
		}
	})

	t.Run("emergency missing power_source fails naming it", func(t *testing.T) {
		m := load()
		m["emergency"] = map[string]any{"emergency_role": "exit_sign_only"}
		r := findings.NewReport()
		v.Validate(m, r)
		if !r.HasErrors() {
			t.Fatalf("expected a schema error for emergency missing power_source, got none")
		}
		if msg := messages(r); !strings.Contains(msg, "power_source") {
			t.Errorf("expected the violation to name power_source; findings:\n%s", msg)
		}
	})

	t.Run("emergency missing emergency_role fails naming it", func(t *testing.T) {
		m := load()
		m["emergency"] = map[string]any{"power_source": "ac_only"}
		r := findings.NewReport()
		v.Validate(m, r)
		if !r.HasErrors() {
			t.Fatalf("expected a schema error for emergency missing emergency_role, got none")
		}
		if msg := messages(r); !strings.Contains(msg, "emergency_role") {
			t.Errorf("expected the violation to name emergency_role; findings:\n%s", msg)
		}
	})

	t.Run("complete minimal emergency block validates", func(t *testing.T) {
		m := load()
		m["emergency"] = map[string]any{"emergency_role": "exit_sign_only", "power_source": "ac_only"}
		r := findings.NewReport()
		v.Validate(m, r)
		if r.HasErrors() {
			t.Errorf("a complete minimal emergency block must validate; got: %+v", r.Findings)
		}
	})

	t.Run("empty exit_sign block validates", func(t *testing.T) {
		// Identity-only sign records must remain schema-valid; grading, not schema,
		// drives completeness, so exit_sign has no required members.
		m := load()
		m["exit_sign"] = map[string]any{}
		r := findings.NewReport()
		v.Validate(m, r)
		if r.HasErrors() {
			t.Errorf("an empty exit_sign block must validate; got: %+v", r.Findings)
		}
	})
}

// TestValidatorConstrainsSustainabilityMetricCarbonScope asserts the sustainability_metric
// conditional (the SourceFile photometry_format precedent): a declared embodied_carbon_kgco2e
// requires both embodied_carbon_scope and embodied_carbon_functional_unit, so a bare kgCO2e
// figure can never be ambiguous about its life-cycle boundary or its declared unit. A metric
// with the kgCO2e alone is a schema error; a metric with all three, and a metric that carries
// no kgCO2e at all, validate.
func TestValidatorConstrainsSustainabilityMetricCarbonScope(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	load := func() map[string]any {
		doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
		m, ok := doc.(map[string]any)
		if !ok {
			t.Fatalf("record is not an object")
		}
		return m
	}
	setMetric := func(m map[string]any, metric map[string]any) {
		atts, _ := m["attestations"].([]any)
		if len(atts) == 0 {
			t.Fatalf("erco record has no attestations to mutate")
		}
		first, ok := atts[0].(map[string]any)
		if !ok {
			t.Fatalf("first attestation is not an object")
		}
		first["sustainability_metric"] = metric
	}

	// Bad: kgCO2e without scope and functional unit.
	bad := load()
	setMetric(bad, map[string]any{"embodied_carbon_kgco2e": 42.0})
	rBad := findings.NewReport()
	v.Validate(bad, rBad)
	if !rBad.HasErrors() {
		t.Error("expected a schema error for embodied_carbon_kgco2e without scope and functional unit, got none")
	}

	// Good: kgCO2e with both required companions.
	good := load()
	setMetric(good, map[string]any{
		"embodied_carbon_kgco2e":          42.0,
		"embodied_carbon_scope":           "a1_a3",
		"embodied_carbon_functional_unit": "one luminaire",
	})
	rGood := findings.NewReport()
	v.Validate(good, rGood)
	if rGood.HasErrors() {
		t.Errorf("a complete embodied-carbon metric must validate; got: %+v", rGood.Findings)
	}

	// Good: a metric with no kgCO2e (ceam_score plus C2C level) has no scope requirement.
	noCarbon := load()
	setMetric(noCarbon, map[string]any{"ceam_score": 3.5, "c2c_overall_level": "gold"})
	rNo := findings.NewReport()
	v.Validate(noCarbon, rNo)
	if rNo.HasErrors() {
		t.Errorf("a non-carbon sustainability_metric must validate; got: %+v", rNo.Findings)
	}
}

// TestValidatorAcceptsIssuingAuthority asserts the additive descriptive field on Attestation
// takes a string and never affects validity.
func TestValidatorAcceptsIssuingAuthority(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
	m, ok := doc.(map[string]any)
	if !ok {
		t.Fatalf("record is not an object")
	}
	atts, _ := m["attestations"].([]any)
	if len(atts) == 0 {
		t.Fatalf("erco record has no attestations to mutate")
	}
	first, ok := atts[0].(map[string]any)
	if !ok {
		t.Fatalf("first attestation is not an object")
	}
	first["issuing_authority"] = "EPD International"
	r := findings.NewReport()
	v.Validate(m, r)
	if r.HasErrors() {
		t.Errorf("issuing_authority on an attestation must validate; got: %+v", r.Findings)
	}
}

// TestValidatorConstrainsAchievementThemeShape asserts the index.achievements member shape:
// a well-formed achievements block (six themes, each a valid AchievementTheme, plus
// documented_count) validates when injected into a record's generated index, and an
// AchievementTheme missing its required `state` is a schema error. The builder emits this
// block from Phase C; this test pins the shape independently of the builder.
func TestValidatorConstrainsAchievementThemeShape(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	theme := func(state string) map[string]any {
		return map[string]any{
			"state":                  state,
			"programs":               []any{},
			"source_attestation_ids": []any{},
			"evidence_present":       false,
		}
	}
	allThemes := func() map[string]any {
		return map[string]any{
			"embodied_carbon": theme("none"),
			"circularity":     theme("none"),
			"material_health": theme("none"),
			"energy":          theme("none"),
			"dark_sky":        theme("none"),
			"emergency":       theme("none"),
		}
	}
	load := func() map[string]any {
		doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
		m, ok := doc.(map[string]any)
		if !ok {
			t.Fatalf("record is not an object")
		}
		return m
	}

	// Good: a well-formed achievements block plus the restricted-substances sibling validate.
	good := load()
	gidx, ok := good["index"].(map[string]any)
	if !ok {
		t.Fatalf("record has no index object")
	}
	gidx["achievements"] = map[string]any{"themes": allThemes(), "documented_count": float64(0)}
	gidx["restricted_substances_declared"] = []any{}
	rGood := findings.NewReport()
	v.Validate(good, rGood)
	if rGood.HasErrors() {
		t.Errorf("a well-formed achievements index block must validate; got: %+v", rGood.Findings)
	}

	// Bad: a theme missing its required state.
	bad := load()
	bidx, ok := bad["index"].(map[string]any)
	if !ok {
		t.Fatalf("record has no index object")
	}
	themes := allThemes()
	broken := theme("none")
	delete(broken, "state")
	themes["emergency"] = broken
	bidx["achievements"] = map[string]any{"themes": themes, "documented_count": float64(0)}
	rBad := findings.NewReport()
	v.Validate(bad, rBad)
	if !rBad.HasErrors() {
		t.Error("expected a schema error for an AchievementTheme missing its required state, got none")
	}
}

// T3 SCHEMA SHAPE (D3 Option A): the domestic_content property $ref is enforced even though
// domestic_content is deliberately not in `required`. A malformed domestic_content
// AchievementTheme is a schema error; a valid seven-theme index and a valid six-theme index
// (domestic_content absent, since it is optional) both validate, so the additive-minor
// property holds: adding the property rejects malformed shapes without invalidating older
// six-theme indices.
func TestValidatorConstrainsDomesticContentTheme(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	theme := func(state string) map[string]any {
		return map[string]any{
			"state":                  state,
			"programs":               []any{},
			"source_attestation_ids": []any{},
			"evidence_present":       false,
		}
	}
	sixThemes := func() map[string]any {
		return map[string]any{
			"embodied_carbon": theme("none"),
			"circularity":     theme("none"),
			"material_health": theme("none"),
			"energy":          theme("none"),
			"dark_sky":        theme("none"),
			"emergency":       theme("none"),
		}
	}
	validate := func(themes map[string]any) *findings.Report {
		doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
		m, ok := doc.(map[string]any)
		if !ok {
			t.Fatalf("record is not an object")
		}
		idx, ok := m["index"].(map[string]any)
		if !ok {
			t.Fatalf("record has no index object")
		}
		idx["achievements"] = map[string]any{"themes": themes, "documented_count": float64(0)}
		idx["restricted_substances_declared"] = []any{}
		r := findings.NewReport()
		v.Validate(m, r)
		return r
	}

	// A valid six-theme index still validates: domestic_content is optional (not in required).
	if r := validate(sixThemes()); r.HasErrors() {
		t.Errorf("a six-theme index must validate under Option A (domestic_content optional); got: %+v", r.Findings)
	}

	// A valid seven-theme index validates: the added property accepts a well-formed theme.
	seven := sixThemes()
	seven["domestic_content"] = theme("claimed")
	if r := validate(seven); r.HasErrors() {
		t.Errorf("a valid seven-theme index must validate; got: %+v", r.Findings)
	}

	// A malformed domestic_content theme (missing its required state) is rejected: the property
	// $ref is enforced even though domestic_content is not in `required`.
	broken := sixThemes()
	badTheme := theme("none")
	delete(badTheme, "state")
	broken["domestic_content"] = badTheme
	if r := validate(broken); !r.HasErrors() {
		t.Error("expected a schema error for a malformed domestic_content AchievementTheme, got none")
	}
}

// TestValidatorAssertsDeclaredFormats asserts the compiler runs the schema's
// declared `format` keywords as checks rather than annotations: a malformed
// date and a relative URI are each a schema violation at their own JSON
// Pointer, and well-formed values in both families still validate. This is the
// test that fails the day the AssertFormat call is dropped from compile.
func TestValidatorAssertsDeclaredFormats(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	load := func() map[string]any {
		doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
		m, ok := doc.(map[string]any)
		if !ok {
			t.Fatalf("record is not an object")
		}
		return m
	}
	setFirstSourceFileURL := func(m map[string]any, url string) {
		sf, _ := m["source_files"].([]any)
		if len(sf) == 0 {
			t.Fatalf("erco record has no source_files to mutate")
		}
		first, ok := sf[0].(map[string]any)
		if !ok {
			t.Fatalf("first source_files entry is not an object")
		}
		ref, ok := first["reference"].(map[string]any)
		if !ok {
			t.Fatalf("first source_files entry has no reference object")
		}
		ref["url"] = url
	}
	// Assert the pointer and the code only: the library owns the message text.
	wantViolationAt := func(t *testing.T, r *findings.Report, pointer string) {
		t.Helper()
		for _, f := range r.Findings {
			if f.Code == findings.CodeSchemaViolation && f.Path == pointer {
				return
			}
		}
		t.Errorf("expected a %q finding at %q; got: %+v", findings.CodeSchemaViolation, pointer, r.Findings)
	}

	// A spreadsheet date serial where an ISO date belongs.
	t.Run("date serial in record_status_as_of", func(t *testing.T) {
		m := load()
		m["record_status_as_of"] = "46082"
		r := findings.NewReport()
		v.Validate(m, r)
		wantViolationAt(t, r, "/record_status_as_of")
	})

	// format: uri requires an absolute URI, so a relative reference is rejected.
	t.Run("relative url on a file reference", func(t *testing.T) {
		m := load()
		setFirstSourceFileURL(m, "files/datasheet.pdf")
		r := findings.NewReport()
		v.Validate(m, r)
		wantViolationAt(t, r, "/source_files/0/reference/url")
	})

	// Positive floor: an absolute url beside the record's shipped ISO
	// record_status_as_of validates, so the assertion cannot pass by rejecting
	// every value.
	t.Run("well-formed values validate", func(t *testing.T) {
		m := load()
		setFirstSourceFileURL(m, "https://example.com/datasheet.pdf")
		r := findings.NewReport()
		v.Validate(m, r)
		if r.HasErrors() {
			t.Errorf("well-formed format values must validate; got: %+v", r.Findings)
		}
	})
}

// TestValidatorConstrainsSupersededBy pins the SupersessionReference
// constraints: at least one of the three naming members is required, the
// record_sha256 pin is only meaningful alongside record_id, and both patterned
// members reject malformed values.
func TestValidatorConstrainsSupersededBy(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	const hash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	validate := func(ref map[string]any) *findings.Report {
		doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
		m, ok := doc.(map[string]any)
		if !ok {
			t.Fatalf("record is not an object")
		}
		m["superseded_by"] = ref
		r := findings.NewReport()
		v.Validate(m, r)
		return r
	}

	accepted := map[string]map[string]any{
		"record_id alone":      {"record_id": "acme-orbit-2400-3000k"},
		"catalog_number alone": {"catalog_number": "ORB-2400-30K-90"},
		"catalog_model alone":  {"catalog_model": "ORB-2400"},
		"record_id with catalog_number and a hash pin": {
			"record_id":      "acme-orbit-2400-3000k",
			"catalog_number": "ORB-2400-30K-90",
			"record_sha256":  hash,
		},
	}
	for name, ref := range accepted {
		t.Run("accepts "+name, func(t *testing.T) {
			if r := validate(ref); r.HasErrors() {
				t.Errorf("expected a valid supersession reference; got: %+v", r.Findings)
			}
		})
	}

	rejected := map[string]map[string]any{
		"an empty object":              {},
		"a hash pin without record_id": {"record_sha256": hash},
		"a malformed record_id slug":   {"record_id": "Bad_Slug"},
		"a record_id under minLength":  {"record_id": "ab"},
		// An empty naming member would satisfy anyOf while naming nothing,
		// which is the case the constraint exists to reject.
		"an empty catalog_number":   {"catalog_number": ""},
		"an empty catalog_model":    {"catalog_model": ""},
		"a malformed record_sha256": {"record_id": "acme-orbit-2400-3000k", "record_sha256": "xyz"},
	}
	for name, ref := range rejected {
		t.Run("rejects "+name, func(t *testing.T) {
			if r := validate(ref); !r.HasErrors() {
				t.Errorf("expected a schema error for %s, got none", name)
			}
		})
	}
}

// TestValidatorConstrainsWarrantyTermBasis pins the closed WarrantyTermBasis
// token set at product_family.shared_warranty.term_basis.
func TestValidatorConstrainsWarrantyTermBasis(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	validate := func(basis string) *findings.Report {
		doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
		m, ok := doc.(map[string]any)
		if !ok {
			t.Fatalf("record is not an object")
		}
		fam, ok := m["product_family"].(map[string]any)
		if !ok {
			t.Fatalf("record has no product_family object")
		}
		warranty, ok := fam["shared_warranty"].(map[string]any)
		if !ok {
			warranty = map[string]any{}
			fam["shared_warranty"] = warranty
		}
		warranty["term_basis"] = basis
		r := findings.NewReport()
		v.Validate(m, r)
		return r
	}

	for _, token := range []string{"invoice", "shipment", "installation", "energization"} {
		t.Run("accepts "+token, func(t *testing.T) {
			if r := validate(token); r.HasErrors() {
				t.Errorf("expected %q to validate; got: %+v", token, r.Findings)
			}
		})
	}
	t.Run("rejects an unlisted basis", func(t *testing.T) {
		if r := validate("purchase"); !r.HasErrors() {
			t.Error("expected a schema error for an unlisted warranty term basis, got none")
		}
	})
}

// TestValidatorConstrainsWarrantyConditionsDocument asserts the new
// conditions_document site carries the FileReference shape, hash included.
func TestValidatorConstrainsWarrantyConditionsDocument(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	validate := func(ref map[string]any) *findings.Report {
		doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
		m, ok := doc.(map[string]any)
		if !ok {
			t.Fatalf("record is not an object")
		}
		fam, ok := m["product_family"].(map[string]any)
		if !ok {
			t.Fatalf("record has no product_family object")
		}
		warranty, ok := fam["shared_warranty"].(map[string]any)
		if !ok {
			warranty = map[string]any{}
			fam["shared_warranty"] = warranty
		}
		warranty["conditions_document"] = ref
		r := findings.NewReport()
		v.Validate(m, r)
		return r
	}

	full := map[string]any{
		"filename":      "warranty-conditions.pdf",
		"sha256":        "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		"revision_date": "2026-01-15",
	}
	if r := validate(full); r.HasErrors() {
		t.Errorf("expected a complete file reference to validate; got: %+v", r.Findings)
	}
	if r := validate(map[string]any{"filename": "warranty-conditions.pdf"}); !r.HasErrors() {
		t.Error("expected a schema error for a conditions_document with no sha256, got none")
	}
}

// TestValidatorConstrainsDiscontinuedAt asserts discontinued_at is checked as
// an ISO 8601 date rather than accepted as free text: the validator asserts
// declared string formats, so a spreadsheet serial, a slash date, and an
// out-of-range ISO value are each a violation at the field's own pointer.
func TestValidatorConstrainsDiscontinuedAt(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	validate := func(value string) *findings.Report {
		doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
		m, ok := doc.(map[string]any)
		if !ok {
			t.Fatalf("record is not an object")
		}
		m["discontinued_at"] = value
		r := findings.NewReport()
		v.Validate(m, r)
		return r
	}

	if r := validate("2027-03-31"); r.HasErrors() {
		t.Errorf("expected an ISO date to validate; got: %+v", r.Findings)
	}
	for _, bad := range []string{"46082", "3/1/26", "2026-13-45"} {
		t.Run("rejects "+bad, func(t *testing.T) {
			r := validate(bad)
			for _, f := range r.Findings {
				if f.Code == findings.CodeSchemaViolation && f.Path == "/discontinued_at" {
					return
				}
			}
			t.Errorf("expected a %q finding at %q; got: %+v", findings.CodeSchemaViolation, "/discontinued_at", r.Findings)
		})
	}
}

// TestValidatorConstrainsMedia pins the media manifest: the required trio, the
// two closed vocabularies (image formats only, so a document type is rejected
// outright), the FileReference contract each entry's reference carries, and the
// keyword conventions on the new descriptive members.
func TestValidatorConstrainsMedia(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	const hash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	photoRef := func() map[string]any {
		return map[string]any{"filename": "acme-orbit-2400.jpg", "sha256": hash}
	}
	validate := func(entry map[string]any) *findings.Report {
		doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
		m, ok := doc.(map[string]any)
		if !ok {
			t.Fatalf("record is not an object")
		}
		m["media"] = []any{entry}
		r := findings.NewReport()
		v.Validate(m, r)
		return r
	}

	accepted := map[string]map[string]any{
		"a minimal entry": {
			"role":       "product_photo",
			"media_type": "image/jpeg",
			"reference":  photoRef(),
		},
		"a dimensional drawing in a vector format": {
			"role":       "dimensional_drawing",
			"media_type": "image/svg+xml",
			"reference":  map[string]any{"filename": "acme-orbit-2400-dimensions.svg", "sha256": hash},
		},
		"an entry populating every optional member": {
			"role":               "application_photo",
			"media_type":         "image/png",
			"reference":          photoRef(),
			"primary":            true,
			"alt_text":           "The luminaire installed above a reception desk",
			"caption":            "Reception installation",
			"language":           "en-GB",
			"width_px":           2400,
			"height_px":          1600,
			"size_bytes":         812345,
			"rights":             "Reuse permitted with attribution",
			"credit":             "Photograph supplied by the manufacturer",
			"extracted_from_ref": "acme-orbit-datasheet.pdf",
			"configuration_refs": []any{"ORB-2400-30K-90"},
		},
	}
	for name, entry := range accepted {
		t.Run("accepts "+name, func(t *testing.T) {
			if r := validate(entry); r.HasErrors() {
				t.Errorf("expected a valid media entry; got: %+v", r.Findings)
			}
		})
	}

	rejected := map[string]map[string]any{
		"an entry with no role":       {"media_type": "image/jpeg", "reference": photoRef()},
		"an entry with no reference":  {"role": "product_photo", "media_type": "image/jpeg"},
		"an entry with no media_type": {"role": "product_photo", "reference": photoRef()},
		"an unlisted role":            {"role": "thumbnail", "media_type": "image/jpeg", "reference": photoRef()},
		"an unlisted image format":    {"role": "product_photo", "media_type": "image/gif", "reference": photoRef()},
		// The manifest admits image formats only, so a document type is invalid
		// wherever it appears: a PDF drawing belongs in source_files instead.
		"a document media type": {
			"role":       "dimensional_drawing",
			"media_type": "application/pdf",
			"reference":  map[string]any{"filename": "acme-orbit-2400-dimensions.pdf", "sha256": hash},
		},
		"a reference with no sha256": {
			"role":       "product_photo",
			"media_type": "image/jpeg",
			"reference":  map[string]any{"filename": "acme-orbit-2400.jpg"},
		},
		"a zero pixel width": {
			"role": "product_photo", "media_type": "image/jpeg", "reference": photoRef(),
			"width_px": 0,
		},
		"an empty caption": {
			"role": "product_photo", "media_type": "image/jpeg", "reference": photoRef(),
			"caption": "",
		},
	}
	for name, entry := range rejected {
		t.Run("rejects "+name, func(t *testing.T) {
			if r := validate(entry); !r.HasErrors() {
				t.Errorf("expected a schema error for %s, got none", name)
			}
		})
	}
}

// TestValidatorConstrainsOrdering pins the keywords the ordering block adds:
// the closed order-unit vocabulary, the dual-unit contract every length member
// inherits through its $ref, and the addressing keywords on a per-configuration
// row, which must name at least one configuration and carry at least one value
// beside the names. Fixture codes and lengths are synthetic.
func TestValidatorConstrainsOrdering(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatalf("NewValidator: %v", err)
	}
	cutModule := func() map[string]any { return map[string]any{"mm": 40, "in": 1.57} }
	runLimit := func() map[string]any { return map[string]any{"mm": 12000, "in": 472.44} }
	validate := func(block map[string]any) *findings.Report {
		doc := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc"))
		m, ok := doc.(map[string]any)
		if !ok {
			t.Fatalf("record is not an object")
		}
		m["ordering"] = block
		r := findings.NewReport()
		v.Validate(m, r)
		return r
	}
	// A row is only ever validated inside the block that carries it.
	validateRow := func(row map[string]any) *findings.Report {
		return validate(map[string]any{"declared_by_configuration": []any{row}})
	}
	// Assert the pointer and the code only: the library owns the message text.
	wantViolationAt := func(t *testing.T, r *findings.Report, pointer string) {
		t.Helper()
		for _, f := range r.Findings {
			if f.Code == findings.CodeSchemaViolation && f.Path == pointer {
				return
			}
		}
		t.Errorf("expected a %q finding at %q; got: %+v", findings.CodeSchemaViolation, pointer, r.Findings)
	}

	accepted := map[string]map[string]any{
		"a minimal cut-to-length block": {"order_unit": "cut_to_length_run"},
		// A sectional family chooses its lengths from the enumerated set its
		// applicability axis already publishes, so it carries no order_increment.
		"a minimal fixed-section block": {"order_unit": "fixed_length_section"},
		"an empty block":                {},
		// One member on the block and a different one in a row. The split is an
		// authoring rule the schema deliberately does not enforce, so this case
		// models the correct shape rather than pinning a rejection.
		"a block splitting an invariant member from a per-configuration one": {
			"order_unit":         "cut_to_length_run",
			"max_continuous_run": runLimit(),
			"declared_by_configuration": []any{
				map[string]any{"configuration_refs": []any{"A2"}, "cut_increment": cutModule()},
			},
		},
		"a block carrying every member": {
			"order_unit":         "cut_to_length_run",
			"order_increment":    cutModule(),
			"cut_increment":      cutModule(),
			"max_continuous_run": runLimit(),
		},
	}
	for name, block := range accepted {
		t.Run("accepts "+name, func(t *testing.T) {
			if r := validate(block); r.HasErrors() {
				t.Errorf("expected a valid ordering block; got: %+v", r.Findings)
			}
		})
	}

	rejected := map[string]struct {
		block   map[string]any
		pointer string
	}{
		"an unlisted order unit": {
			block:   map[string]any{"order_unit": "linear_foot"},
			pointer: "/ordering/order_unit",
		},
		"a length carrying only its SI leaf": {
			block:   map[string]any{"cut_increment": map[string]any{"mm": 40}},
			pointer: "/ordering/cut_increment",
		},
		"an order increment carrying only its SI leaf": {
			block:   map[string]any{"order_increment": map[string]any{"mm": 40}},
			pointer: "/ordering/order_increment",
		},
		"a row run limit carrying only its SI leaf": {
			block: map[string]any{"declared_by_configuration": []any{
				map[string]any{"configuration_refs": []any{"A1"}, "max_continuous_run": map[string]any{"mm": 12000}},
			}},
			pointer: "/ordering/declared_by_configuration/0/max_continuous_run",
		},
		"per-configuration rows carried as an object": {
			block:   map[string]any{"declared_by_configuration": map[string]any{}},
			pointer: "/ordering/declared_by_configuration",
		},
	}
	for name, tc := range rejected {
		t.Run("rejects "+name, func(t *testing.T) {
			r := validate(tc.block)
			if !r.HasErrors() {
				t.Fatalf("expected a schema error for %s, got none", name)
			}
			wantViolationAt(t, r, tc.pointer)
		})
	}

	rejectedRows := map[string]struct {
		row     map[string]any
		pointer string
	}{
		"a row naming no configurations": {
			row:     map[string]any{"order_increment": cutModule(), "cut_increment": cutModule()},
			pointer: "/ordering/declared_by_configuration/0",
		},
		"a row carrying no value": {
			row:     map[string]any{"configuration_refs": []any{"A1"}},
			pointer: "/ordering/declared_by_configuration/0",
		},
		"a row scoped to no order code": {
			row:     map[string]any{"configuration_refs": []any{}, "order_increment": cutModule()},
			pointer: "/ordering/declared_by_configuration/0/configuration_refs",
		},
		"a row scoped to an empty string": {
			row:     map[string]any{"configuration_refs": []any{""}, "order_increment": cutModule()},
			pointer: "/ordering/declared_by_configuration/0/configuration_refs/0",
		},
	}
	for name, tc := range rejectedRows {
		t.Run("rejects "+name, func(t *testing.T) {
			r := validateRow(tc.row)
			if !r.HasErrors() {
				t.Fatalf("expected a schema error for %s, got none", name)
			}
			wantViolationAt(t, r, tc.pointer)
		})
	}
}

// TestSchemaFormatDeclarationsAreAsserted guards the two properties that make
// a declared format load-bearing, at every site in both schema documents.
// First, the format name is one the compiler actually asserts: an unrecognized
// name is accepted silently and leaves the site unchecked, so a typo would
// ship a field that looks constrained and is not, with every other test still
// green. Adding a new format name to ULC means extending the pinned set here,
// at which point the author confirms the library asserts it. Second, the same
// subschema declares type string: the format checks return without complaint
// for non-string instances, so the pairing is what makes the check run.
func TestSchemaFormatDeclarationsAreAsserted(t *testing.T) {
	assertedFormats := map[string]bool{"date": true, "uri": true}

	var sites []string
	var walk func(node any, ptr string)
	walk = func(node any, ptr string) {
		switch n := node.(type) {
		case []any:
			for i, v := range n {
				walk(v, fmt.Sprintf("%s/%d", ptr, i))
			}
		case map[string]any:
			if raw, ok := n["format"]; ok {
				name, isString := raw.(string)
				if !isString {
					t.Errorf("%s: format is %T, want a string", ptr, raw)
				} else if !assertedFormats[name] {
					t.Errorf("%s: format %q is outside the asserted set {date, uri}; an unrecognized name leaves the site unchecked", ptr, name)
				}
				if n["type"] != "string" {
					t.Errorf("%s: a subschema declaring format must declare type string, got type %v", ptr, n["type"])
				}
				sites = append(sites, ptr)
			}
			for k, v := range n {
				walk(v, ptr+"/"+k)
			}
		}
	}

	for _, doc := range []struct {
		name string
		raw  []byte
	}{
		{"ulc.schema.json", embedded.ULCSchemaJSON},
		{"taxonomy.schema.json", embedded.TaxonomySchemaJSON},
	} {
		var parsed map[string]any
		if err := json.Unmarshal(doc.raw, &parsed); err != nil {
			t.Fatalf("parse %s: %v", doc.name, err)
		}
		walk(parsed, doc.name)
	}
	if len(sites) == 0 {
		t.Fatal("no format declarations found, so this guard checked nothing")
	}
}

// TestProvenanceSourceMirrorsSourceFileType enforces the synchronization the
// taxonomy states normatively: every SourceFileType token must also be a
// ProvenanceSource token, so any source file a record cites can also be named
// as the origin of an individual value. The two enums are edited by hand, and
// nothing else fails when a token lands in one and not the other.
func TestProvenanceSourceMirrorsSourceFileType(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(embedded.TaxonomySchemaJSON, &doc); err != nil {
		t.Fatalf("parse taxonomy.schema.json: %v", err)
	}
	defs, ok := doc["$defs"].(map[string]any)
	if !ok {
		t.Fatal("taxonomy.schema.json has no $defs object")
	}
	tokens := func(name string) map[string]bool {
		def, ok := defs[name].(map[string]any)
		if !ok {
			t.Fatalf("taxonomy.schema.json has no %s definition", name)
		}
		raw, ok := def["enum"].([]any)
		if !ok || len(raw) == 0 {
			t.Fatalf("%s has no non-empty enum", name)
		}
		out := make(map[string]bool, len(raw))
		for _, v := range raw {
			s, ok := v.(string)
			if !ok {
				t.Fatalf("%s has a non-string enum member %v", name, v)
			}
			out[s] = true
		}
		return out
	}

	provenance := tokens("ProvenanceSource")
	for token := range tokens("SourceFileType") {
		if !provenance[token] {
			t.Errorf("SourceFileType token %q is missing from ProvenanceSource", token)
		}
	}
}

func TestFindSchemaDirExplicit(t *testing.T) {
	root := repoRoot(t)
	schemaDir := filepath.Join(root, "schema")
	got, err := FindSchemaDir(schemaDir, "")
	if err != nil {
		t.Fatalf("FindSchemaDir explicit: %v", err)
	}
	if got != schemaDir {
		t.Fatalf("got %q, want %q", got, schemaDir)
	}
}

func TestFindSchemaDirWalksUp(t *testing.T) {
	root := repoRoot(t)
	recordPath := filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc")
	got, err := FindSchemaDir("", recordPath)
	if err != nil {
		t.Fatalf("FindSchemaDir walk-up: %v", err)
	}
	if !strings.HasSuffix(got, filepath.Join(root, "schema")) && got != filepath.Join(root, "schema") {
		t.Fatalf("got %q, want suffix schema", got)
	}
}

// --- helpers ---

func repoRoot(t *testing.T) string {
	t.Helper()
	// Tests run from tools/validator/internal/validate; four levels up is repo root.
	p, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return p
}

func loadOrFail(t *testing.T, path string) any {
	t.Helper()
	data, err := readFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return doc
}

// readFile wraps os.ReadFile so the import surface of this test file stays
// narrow (helpful for scanning in a code review).
func readFile(path string) ([]byte, error) {
	return osReadFile(path)
}
