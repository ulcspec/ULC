package validate

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

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
