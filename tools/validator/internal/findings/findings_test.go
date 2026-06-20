package findings

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestFinalizeSortsAndCounts(t *testing.T) {
	r := NewReport()
	r.AddInfo(CodeConformanceLevel, "/x", "info one")
	r.AddError(CodeIndexDrift, "/index/nominal_total_lumens", "lumens drift")
	r.AddWarning(CodeSourceFileNotFound, "/source_files/0", "pdf missing")
	r.AddError(CodeSchemaViolation, "/product_family", "schema issue")
	r.Finalize()

	if r.Summary.Errors != 2 {
		t.Errorf("errors=%d want 2", r.Summary.Errors)
	}
	if r.Summary.Warnings != 1 {
		t.Errorf("warnings=%d want 1", r.Summary.Warnings)
	}
	if r.Summary.Infos != 1 {
		t.Errorf("infos=%d want 1", r.Summary.Infos)
	}
	// Errors first, warnings second, infos last.
	if r.Findings[0].Level != LevelError || r.Findings[1].Level != LevelError {
		t.Errorf("expected first two findings to be ERROR: %+v", r.Findings[:2])
	}
	if r.Findings[2].Level != LevelWarning {
		t.Errorf("expected third finding to be WARNING: %+v", r.Findings[2])
	}
	if r.Findings[3].Level != LevelInfo {
		t.Errorf("expected fourth finding to be INFO: %+v", r.Findings[3])
	}
}

func TestWriteTextEmptyReport(t *testing.T) {
	r := NewReport()
	r.Finalize()
	buf := &bytes.Buffer{}
	if err := r.WriteText(buf, "record.ulc"); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	s := buf.String()
	if !strings.Contains(s, "OK") || !strings.Contains(s, "0 errors") {
		t.Errorf("expected OK with 0 errors, got %q", s)
	}
}

func TestAddRoadmapSetsStructuredFields(t *testing.T) {
	r := NewReport()
	r.AddRoadmap(CodeConformanceGap, "/colorimetry/sdcm_step", "standard", "datasheet_pdf", "ANSI C78.377",
		`to reach "standard", add /colorimetry/sdcm_step`)
	if len(r.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(r.Findings))
	}
	f := r.Findings[0]
	if f.Level != LevelInfo {
		t.Errorf("roadmap finding level = %q, want INFO", f.Level)
	}
	if f.NextConformanceLevel != "standard" {
		t.Errorf("NextConformanceLevel = %q, want standard", f.NextConformanceLevel)
	}
	if f.SourceDocument != "datasheet_pdf" {
		t.Errorf("SourceDocument = %q, want datasheet_pdf", f.SourceDocument)
	}
	if f.Standard != "ANSI C78.377" {
		t.Errorf("Standard = %q, want ANSI C78.377", f.Standard)
	}
}

// TestFinalizeStableWithRoadmapFields confirms the deterministic sort is
// unaffected by (and stable across) the structured roadmap fields: two roadmap
// findings sharing code and path but differing only in the structured fields keep
// their insertion order after Finalize.
func TestFinalizeStableWithRoadmapFields(t *testing.T) {
	r := NewReport()
	r.AddRoadmap(CodeConformanceGap, "/p", "standard", "ies", "LM-79", "msg A")
	r.AddRoadmap(CodeConformanceGap, "/p", "full", "test_report", "TM-30", "msg B")
	r.Finalize()
	if r.Summary.Infos != 2 {
		t.Fatalf("infos = %d, want 2", r.Summary.Infos)
	}
	// Both share code+path; the tiebreak is Message, so "msg A" sorts before "msg B".
	if r.Findings[0].Message != "msg A" || r.Findings[1].Message != "msg B" {
		t.Errorf("unexpected order: %q then %q", r.Findings[0].Message, r.Findings[1].Message)
	}
}

// TestVerboseGatesObservationsInText confirms observation findings are suppressed
// from WriteText unless Verbose, while the achieved-level summary and roadmap
// always render and WriteJSON always keeps everything.
func TestVerboseGatesObservationsInText(t *testing.T) {
	build := func() *Report {
		r := NewReport()
		r.AddInfo(CodeConformanceLevel, "/index/conformance_level", `achieves conformance level "core"`)
		r.AddRoadmap(CodeConformanceGap, "/colorimetry/sdcm_step", "standard", "datasheet_pdf", "ANSI C78.377", "to reach standard, add sdcm_step")
		r.AddInfo(CodeConformanceObservation, "/thermal_derating", "thermal derating not disclosed")
		r.Finalize()
		return r
	}

	// Default (Verbose=false): observation suppressed, summary + roadmap shown.
	quiet := build()
	buf := &bytes.Buffer{}
	if err := quiet.WriteText(buf, "rec.ulc"); err != nil {
		t.Fatalf("WriteText: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "/thermal_derating") {
		t.Errorf("observation should be suppressed without --verbose:\n%s", out)
	}
	if !strings.Contains(out, "conformance level") || !strings.Contains(out, "/colorimetry/sdcm_step") {
		t.Errorf("summary and roadmap must always render:\n%s", out)
	}

	// Verbose=true: observation shown.
	loud := build()
	loud.Verbose = true
	buf.Reset()
	if err := loud.WriteText(buf, "rec.ulc"); err != nil {
		t.Fatalf("WriteText verbose: %v", err)
	}
	if !strings.Contains(buf.String(), "/thermal_derating") {
		t.Errorf("observation must render with --verbose:\n%s", buf.String())
	}

	// JSON always includes the observation regardless of Verbose.
	jbuf := &bytes.Buffer{}
	if err := build().WriteJSON(jbuf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if !strings.Contains(jbuf.String(), "/thermal_derating") {
		t.Errorf("JSON must always include observations:\n%s", jbuf.String())
	}
}

func TestWriteJSONRoundTrip(t *testing.T) {
	r := NewReport()
	r.AddError(CodeIndexDrift, "/index/nominal_total_lumens", "drift msg")
	r.Finalize()
	buf := &bytes.Buffer{}
	if err := r.WriteJSON(buf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	var round struct {
		Findings []Finding `json:"findings"`
		Summary  Summary   `json:"summary"`
	}
	if err := json.Unmarshal(buf.Bytes(), &round); err != nil {
		t.Fatalf("unmarshal round-trip: %v", err)
	}
	if round.Summary.Errors != 1 || len(round.Findings) != 1 {
		t.Errorf("unexpected round-tripped report: %+v", round)
	}
	if round.Findings[0].Code != CodeIndexDrift {
		t.Errorf("code lost in round-trip: got %q", round.Findings[0].Code)
	}
}
