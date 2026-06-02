package grade

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// repoRoot resolves the repository root from this test package's directory
// (tools/validator/internal/grade -> four levels up).
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

// exampleRecord loads and normalizes a record under examples/.
func exampleRecord(t *testing.T, name string) map[string]any {
	t.Helper()
	path := filepath.Join(repoRoot(t), "examples", name)
	return loadRecord(t, path)
}

// findingsFor returns every finding carrying the given code.
func findingsFor(report *findings.Report, code findings.Code) []findings.Finding {
	out := []findings.Finding{}
	for _, f := range report.Findings {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

// TestAchievedLevel asserts the computed level on the four canonical example
// records. These are the values the builder stamps into index.conformance_level.
func TestAchievedLevel(t *testing.T) {
	cases := []struct {
		name string
		want Level
	}{
		{"erco-quintessence-30416-023.ulc", LevelFull},
		{"selux-aya-pole-sr-ho-3000k.ulc", LevelFull},
		{"vode-nexa-suspended-807-so-3500k-90cri-hl-black-48in.ulc", LevelFull},
		// lumenpulse: no colorimetry.nominal_cct_k (a hard standard requirement),
		// so it grades core. It is RGB so cri_ra is skipped, and it is outdoor with
		// no BUG, but BUG only matters at full and full is unreachable from core, so
		// neither affects the core grade.
		{"lumenpulse-lumenfacade-loi-12-rgb-30x60-ts0.ulc", LevelCore},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			rec := exampleRecord(t, c.name)
			if got := AchievedLevel(rec); got != c.want {
				t.Errorf("AchievedLevel(%s) = %s, want %s", c.name, got, c.want)
			}
		})
	}
}

// TestReportEmitsAchievedLevelOnly confirms that conformance grading produces
// only INFO findings (no WARNING, no ERROR): a record is whatever level its data
// achieves, with nothing to fall short of.
func TestReportEmitsInfoOnly(t *testing.T) {
	for _, name := range []string{
		"erco-quintessence-30416-023.ulc",
		"selux-aya-pole-sr-ho-3000k.ulc",
		"vode-nexa-suspended-807-so-3500k-90cri-hl-black-48in.ulc",
		"lumenpulse-lumenfacade-loi-12-rgb-30x60-ts0.ulc",
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			rec := exampleRecord(t, name)
			report := findings.NewReport()
			Report(rec, report)
			report.Finalize()
			if report.Summary.Errors != 0 {
				t.Errorf("%s: expected 0 ERROR findings, got %d", name, report.Summary.Errors)
			}
			if report.Summary.Warnings != 0 {
				t.Errorf("%s: expected 0 WARNING findings, got %d", name, report.Summary.Warnings)
			}
		})
	}
}

// TestReportSummaryFull checks the achieved-level INFO wording on a full record.
func TestReportSummaryFull(t *testing.T) {
	rec := exampleRecord(t, "erco-quintessence-30416-023.ulc")
	report := findings.NewReport()
	achieved := Report(rec, report)
	report.Finalize()
	if achieved != LevelFull {
		t.Fatalf("achieved = %s, want full", achieved)
	}
	summaries := findingsFor(report, findings.CodeConformanceLevel)
	if len(summaries) != 1 {
		t.Fatalf("expected exactly 1 conformance-level INFO, got %d", len(summaries))
	}
	if got := summaries[0].Message; !strings.Contains(got, `achieves conformance level "full"`) {
		t.Errorf("summary = %q, want it to name full", got)
	}
	// A full record emits no gap guidance.
	if gaps := findingsFor(report, findings.CodeConformanceGap); len(gaps) != 0 {
		t.Errorf("expected 0 gap INFO at full, got %d: %+v", len(gaps), gaps)
	}
}

// TestReportFullObservations confirms a full record's INFO observations fire for
// the comprehensive items it lacks. vode carries a rated efficacy value_type, so
// the provenance-quality observation must fire on the efficacy value.
func TestReportFullObservations(t *testing.T) {
	rec := exampleRecord(t, "vode-nexa-suspended-807-so-3500k-90cri-hl-black-48in.ulc")
	report := findings.NewReport()
	if achieved := Report(rec, report); achieved != LevelFull {
		t.Fatalf("achieved = %s, want full", achieved)
	}
	report.Finalize()
	if !hasObservationAt(report, "/photometry/luminaire_efficacy_lm_per_w/value_type") {
		t.Errorf("expected provenance INFO on efficacy value_type; observations: %+v",
			findingsFor(report, findings.CodeConformanceObservation))
	}
}

// TestReportGapGuidance confirms that a record below full is told which hard
// fields to add to reach the next level, and that the conditional predicates are
// applied (the RGB lumenpulse record is not told to add cri_ra; the bug_rating
// gap belongs to full, not the standard gap reported from core).
func TestReportGapGuidance(t *testing.T) {
	rec := exampleRecord(t, "lumenpulse-lumenfacade-loi-12-rgb-30x60-ts0.ulc")
	report := findings.NewReport()
	if achieved := Report(rec, report); achieved != LevelCore {
		t.Fatalf("achieved = %s, want core", achieved)
	}
	report.Finalize()

	gaps := findingsFor(report, findings.CodeConformanceGap)
	if len(gaps) != 1 {
		t.Fatalf("expected exactly 1 gap INFO, got %d: %+v", len(gaps), gaps)
	}
	msg := gaps[0].Message
	if !strings.Contains(msg, `to reach "standard"`) {
		t.Errorf("gap = %q, want it to target standard", msg)
	}
	// The single missing standard hard field on this record is nominal_cct_k.
	if !strings.Contains(msg, "/colorimetry/nominal_cct_k") {
		t.Errorf("gap = %q, want it to list /colorimetry/nominal_cct_k", msg)
	}
	// RGB record: cri_ra is conditional and skipped, so it must not appear.
	if strings.Contains(msg, "/colorimetry/cri_ra") {
		t.Errorf("gap = %q, must not list cri_ra on an RGB record", msg)
	}
	// bug_rating is a full-level field; the standard gap must not mention it.
	if strings.Contains(msg, "/outdoor_classification/bug_rating") {
		t.Errorf("gap = %q, must not list a full-level field in the standard gap", msg)
	}
}

// TestAchievedLevelCoreMissing covers the below-core case: a record with no
// photometric anchors is not a photometric record at all and grades LevelNone.
func TestAchievedLevelCoreMissing(t *testing.T) {
	rec := map[string]any{
		"product_family": map[string]any{"primary_category": "downlight"},
	}
	if got := AchievedLevel(rec); got != LevelNone {
		t.Errorf("AchievedLevel = %s, want none", got)
	}
	// Report stays silent on gap guidance below core (schema validation surfaces
	// the missing anchors). It still names the level.
	report := findings.NewReport()
	Report(rec, report)
	report.Finalize()
	if report.Summary.Errors != 0 || report.Summary.Warnings != 0 {
		t.Errorf("expected INFO-only report, got %d errors, %d warnings",
			report.Summary.Errors, report.Summary.Warnings)
	}
}

// standardBase returns a minimal synthetic record that meets every hard
// standard requirement (and therefore core). It is intentionally white-light,
// non-directional, and indoor, so the conditional predicates (beam_angle_deg,
// cri_ra, bug_rating) are all inert; individual tests flip one axis at a time to
// isolate a single gate. Numeric anchors use float64 to match the normalized
// shapes the grader sees from the CLI.
func standardBase() map[string]any {
	return map[string]any{
		"product_family": map[string]any{
			"primary_category": "flood_area_site", // non-directional
			"indoor_outdoor":   "indoor",
		},
		"configuration": map[string]any{
			"tested_axes": map[string]any{
				"color_tunability": "static_white", // not pure color-mixing
			},
		},
		"photometry": map[string]any{
			"total_luminous_flux_lm":        map[string]any{"value": float64(1000)},
			"luminaire_efficacy_lm_per_w":   map[string]any{"value": float64(100)},
			"maximum_intensity_cd":          map[string]any{"value": float64(500)},
			"distribution_type":             "direct",
			"photometric_coordinate_system": "type_c",
			"symmetry_type":                 "quadrilateral",
		},
		"electrical": map[string]any{
			"input_power_w":     map[string]any{"value": float64(10)},
			"control_gear_type": "internal_driver",
			"input_voltage_v":   map[string]any{"value": float64(120)},
		},
		"colorimetry": map[string]any{
			"nominal_cct_k": map[string]any{"value": float64(3000)},
			"cri_ra":        map[string]any{"value": float64(90)},
		},
		"test_conditions": map[string]any{
			"photometry_basis": "absolute",
		},
		"instrumentation": map[string]any{
			"measurement_regime": "luminaire_level",
		},
		"attestations": []any{
			map[string]any{"program": "lm_79_08"},
		},
		"lumen_maintenance_luminaire": map[string]any{
			"manufacturer_rated_claim": map[string]any{"rated_hours": float64(50000)},
		},
	}
}

// fullBase extends standardBase to also meet the hard full requirement
// (operating_point). It remains indoor by default so the conditional bug_rating
// gate is inert; the outdoor test flips indoor_outdoor to exercise it.
func fullBase() map[string]any {
	rec := standardBase()
	rec["operating_point"] = map[string]any{
		"drive_current_ma": map[string]any{"value": float64(350)},
	}
	return rec
}

// TestAchievedLevelDirectionalGate pins the isDirectional conditional: a
// directional primary_category makes photometry.beam_angle_deg a hard standard
// requirement, so a record missing it grades core; a non-directional category
// with the same data grades standard because beam_angle_deg is not required.
func TestAchievedLevelDirectionalGate(t *testing.T) {
	cases := []struct {
		name     string
		category string
		want     Level
	}{
		// downlight is directional: beam_angle_deg is required and absent here.
		{"directional missing beam angle grades core", "downlight", LevelCore},
		// flood_area_site is non-directional: beam_angle_deg not required.
		{"non-directional needs no beam angle grades standard", "flood_area_site", LevelStandard},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			rec := standardBase()
			rec["product_family"].(map[string]any)["primary_category"] = c.category
			if got := AchievedLevel(rec); got != c.want {
				t.Errorf("AchievedLevel = %s, want %s", got, c.want)
			}
		})
	}
}

// TestAchievedLevelOutdoorBugGate pins the isOutdoor conditional: an outdoor (or
// both) record makes outdoor_classification.bug_rating a hard full requirement,
// so a record otherwise meeting full but missing bug_rating grades standard; the
// same record marked indoor reaches full because bug_rating is not required.
func TestAchievedLevelOutdoorBugGate(t *testing.T) {
	cases := []struct {
		name          string
		indoorOutdoor string
		want          Level
	}{
		// outdoor: bug_rating is required and absent here.
		{"outdoor missing bug rating grades standard", "outdoor", LevelStandard},
		// both: same outdoor gate applies.
		{"both missing bug rating grades standard", "both", LevelStandard},
		// indoor: bug_rating not required, so the record reaches full.
		{"indoor needs no bug rating grades full", "indoor", LevelFull},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			rec := fullBase()
			rec["product_family"].(map[string]any)["indoor_outdoor"] = c.indoorOutdoor
			if got := AchievedLevel(rec); got != c.want {
				t.Errorf("AchievedLevel = %s, want %s", got, c.want)
			}
		})
	}
}

// --- helpers ---

func hasObservationAt(report *findings.Report, path string) bool {
	for _, f := range findingsFor(report, findings.CodeConformanceObservation) {
		if f.Path == path {
			return true
		}
	}
	return false
}

func loadRecord(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	normalized, err := normalizeForTest(raw)
	if err != nil {
		t.Fatalf("normalize %s: %v", path, err)
	}
	m, ok := normalized.(map[string]any)
	if !ok {
		t.Fatalf("%s: top-level JSON is not an object", path)
	}
	return m
}

// normalizeForTest mirrors main.normalizeNumbers: json.Number -> int64 when
// integral, float64 otherwise, so the grader sees the same shapes the CLI does.
func normalizeForTest(v any) (any, error) {
	switch n := v.(type) {
	case map[string]any:
		for k, child := range n {
			fixed, err := normalizeForTest(child)
			if err != nil {
				return nil, err
			}
			n[k] = fixed
		}
		return n, nil
	case []any:
		for i, child := range n {
			fixed, err := normalizeForTest(child)
			if err != nil {
				return nil, err
			}
			n[i] = fixed
		}
		return n, nil
	case json.Number:
		s := n.String()
		isInt := true
		for _, r := range s {
			if r == '.' || r == 'e' || r == 'E' {
				isInt = false
				break
			}
		}
		if isInt {
			if i, err := n.Int64(); err == nil {
				return i, nil
			}
		}
		f, err := n.Float64()
		if err != nil {
			return nil, fmt.Errorf("invalid number %q: %w", s, err)
		}
		return f, nil
	default:
		return v, nil
	}
}
