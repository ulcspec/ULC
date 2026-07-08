package findings

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// TestAddEnrichmentSetsStructuredFields mirrors TestAddRoadmapSetsStructuredFields
// for the enrichment carrier: AddEnrichment sets SourceDocument, Standard, and
// Message on an INFO finding, and leaves NextConformanceLevel empty (an enrichment
// suggestion unlocks no tier).
func TestAddEnrichmentSetsStructuredFields(t *testing.T) {
	r := NewReport()
	r.AddEnrichment(CodeConformanceEnrichment, "/thermal_derating", "test_report", "LM-82",
		"thermal derating not disclosed")
	if len(r.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(r.Findings))
	}
	f := r.Findings[0]
	if f.Level != LevelInfo {
		t.Errorf("enrichment finding level = %q, want INFO", f.Level)
	}
	if f.Code != CodeConformanceEnrichment {
		t.Errorf("code = %q, want %q", f.Code, CodeConformanceEnrichment)
	}
	if f.SourceDocument != "test_report" {
		t.Errorf("SourceDocument = %q, want test_report", f.SourceDocument)
	}
	if f.Standard != "LM-82" {
		t.Errorf("Standard = %q, want LM-82", f.Standard)
	}
	if f.NextConformanceLevel != "" {
		t.Errorf("NextConformanceLevel = %q, want empty (enrichment unlocks no tier)", f.NextConformanceLevel)
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

// TestVerboseGatesOptionalFindingsInText confirms the optional conformance findings
// (the enrichment roadmap AND the observation notes) are suppressed from WriteText
// unless Verbose, while the achieved-level summary and tier roadmap always render and
// WriteJSON always keeps everything. It exercises all three hidden states (enrichment
// only, observations only, both) and the merged hidden-count hint.
func TestVerboseGatesOptionalFindingsInText(t *testing.T) {
	base := func() *Report {
		r := NewReport()
		r.AddInfo(CodeConformanceLevel, "/index/conformance_level", `achieves conformance level "core"`)
		r.AddRoadmap(CodeConformanceGap, "/colorimetry/sdcm_step", "standard", "datasheet_pdf", "ANSI C78.377", "to reach standard, add sdcm_step")
		return r
	}
	withEnrichment := func(r *Report) {
		r.AddEnrichment(CodeConformanceEnrichment, "/thermal_derating", "test_report", "LM-82", "thermal derating not disclosed")
	}
	withObservation := func(r *Report) {
		r.AddInfo(CodeConformanceObservation, "/sustainability_declaration", "sustainability declaration not disclosed")
	}

	cases := []struct {
		name    string
		add     []func(*Report)
		hidden  int
		markers []string // paths that must be present verbose, absent quiet
	}{
		{"enrichment only", []func(*Report){withEnrichment}, 1, []string{"/thermal_derating"}},
		{"observations only", []func(*Report){withObservation}, 1, []string{"/sustainability_declaration"}},
		{"both", []func(*Report){withEnrichment, withObservation}, 2, []string{"/thermal_derating", "/sustainability_declaration"}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			build := func() *Report {
				r := base()
				for _, a := range c.add {
					a(r)
				}
				r.Finalize()
				return r
			}

			// Default (Verbose=false): optional findings suppressed; the summary and
			// tier roadmap always render; the merged hint names the hidden count.
			quiet := build()
			buf := &bytes.Buffer{}
			if err := quiet.WriteText(buf, "rec.ulc"); err != nil {
				t.Fatalf("WriteText: %v", err)
			}
			out := buf.String()
			for _, m := range c.markers {
				if strings.Contains(out, m) {
					t.Errorf("%s: %q should be suppressed without --verbose:\n%s", c.name, m, out)
				}
			}
			if !strings.Contains(out, "conformance level") || !strings.Contains(out, "/colorimetry/sdcm_step") {
				t.Errorf("%s: summary and tier roadmap must always render:\n%s", c.name, out)
			}
			wantHint := fmt.Sprintf("%d optional findings hidden (enrichment and observations); use --verbose or --json", c.hidden)
			if !strings.Contains(out, wantHint) {
				t.Errorf("%s: expected merged hint %q:\n%s", c.name, wantHint, out)
			}

			// Verbose=true: optional findings shown.
			loud := build()
			loud.Verbose = true
			buf.Reset()
			if err := loud.WriteText(buf, "rec.ulc"); err != nil {
				t.Fatalf("WriteText verbose: %v", err)
			}
			for _, m := range c.markers {
				if !strings.Contains(buf.String(), m) {
					t.Errorf("%s: %q must render with --verbose:\n%s", c.name, m, buf.String())
				}
			}

			// JSON always includes the optional findings regardless of Verbose.
			jbuf := &bytes.Buffer{}
			if err := build().WriteJSON(jbuf); err != nil {
				t.Fatalf("WriteJSON: %v", err)
			}
			for _, m := range c.markers {
				if !strings.Contains(jbuf.String(), m) {
					t.Errorf("%s: JSON must always include %q:\n%s", c.name, m, jbuf.String())
				}
			}
		})
	}
}

// TestOmitFlagHintDropsFlagAdvice confirms a caller without --verbose/--json (like
// `ulc from-sheet`) can suppress the flag-specific advice while still reporting the
// hidden-findings count.
func TestOmitFlagHintDropsFlagAdvice(t *testing.T) {
	build := func(omit bool) string {
		r := NewReport()
		r.AddInfo(CodeConformanceLevel, "/index/conformance_level", `achieves conformance level "core"`)
		r.AddEnrichment(CodeConformanceEnrichment, "/thermal_derating", "test_report", "LM-82", "thermal derating not disclosed")
		r.OmitFlagHint = omit
		r.Finalize()
		buf := &bytes.Buffer{}
		if err := r.WriteText(buf, "rec.ulc"); err != nil {
			t.Fatalf("WriteText: %v", err)
		}
		return buf.String()
	}
	// Default: the flag advice is present.
	def := build(false)
	if !strings.Contains(def, "1 optional findings hidden (enrichment and observations); use --verbose or --json") {
		t.Errorf("default hint should include the flag advice:\n%s", def)
	}
	// OmitFlagHint: the count remains, the flag advice is gone.
	omit := build(true)
	if !strings.Contains(omit, "1 optional findings hidden (enrichment and observations)") {
		t.Errorf("omit hint should still report the count:\n%s", omit)
	}
	if strings.Contains(omit, "--verbose") || strings.Contains(omit, "--json") {
		t.Errorf("omit hint must not mention --verbose/--json:\n%s", omit)
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
