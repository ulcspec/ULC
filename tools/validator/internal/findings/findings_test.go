package findings

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestFinalizeSortsAndCounts(t *testing.T) {
	r := NewReport()
	r.AddInfo(CodeConformanceGradingDeferred, "/x", "info one")
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
