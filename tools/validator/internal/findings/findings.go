// Package findings is the structured diagnostic model the `ulc validate`
// command emits. Every check produces zero or more Finding values keyed by
// Level; callers aggregate them into a Report and render it as human-readable
// text or JSON.
//
// Severity semantics:
//
//	Error   a hard violation of the spec. The record is not ULC-conformant.
//	Warning a soft concern graded against the record's declared conformance
//	        level (e.g., "full"-level record missing alpha-opic metrics).
//	Info    an observation the user might want but which is not a defect
//	        (e.g., source file not on the local filesystem so hash cannot
//	        be verified here).
package findings

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// Level is the severity tier of a Finding.
type Level string

const (
	LevelError   Level = "ERROR"
	LevelWarning Level = "WARNING"
	LevelInfo    Level = "INFO"
)

// Code is an identifier for a class of finding. Stable across releases so
// consumers can suppress or act on specific codes.
type Code string

const (
	// Schema validation.
	CodeSchemaViolation Code = "schema/violation"

	// Index / builder parity.
	CodeIndexBuilderMissingRequired Code = "index/builder-missing-required"
	CodeIndexDrift                  Code = "index/drift"

	// Source-file hash verification.
	CodeSourceFileHashMismatch Code = "source-file/hash-mismatch"
	CodeSourceFileNotFound     Code = "source-file/not-found-locally"
	CodeSourceFileUnreadable   Code = "source-file/unreadable"

	// Conformance grading (deferred; currently emits a single info).
	CodeConformanceGradingDeferred Code = "conformance/grading-deferred"
)

// Finding is a single diagnostic.
type Finding struct {
	Level   Level  `json:"level"`
	Code    Code   `json:"code"`
	Message string `json:"message"`
	// Path is an optional JSON Pointer into the record that located the problem.
	Path string `json:"path,omitempty"`
}

// Report is the aggregate result of a validate run.
type Report struct {
	Findings []Finding `json:"findings"`
	// Summary counters; derived by Finalize but kept on the struct so JSON
	// consumers do not have to recompute.
	Summary Summary `json:"summary"`
}

// Summary is the counts rollup used by both the text renderer and JSON consumers.
type Summary struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Infos    int `json:"infos"`
}

// NewReport returns an empty Report.
func NewReport() *Report {
	return &Report{Findings: []Finding{}}
}

// Add appends a finding.
func (r *Report) Add(f Finding) {
	r.Findings = append(r.Findings, f)
}

// AddError, AddWarning, AddInfo are convenience wrappers.
func (r *Report) AddError(code Code, path, msg string) {
	r.Add(Finding{Level: LevelError, Code: code, Path: path, Message: msg})
}
func (r *Report) AddWarning(code Code, path, msg string) {
	r.Add(Finding{Level: LevelWarning, Code: code, Path: path, Message: msg})
}
func (r *Report) AddInfo(code Code, path, msg string) {
	r.Add(Finding{Level: LevelInfo, Code: code, Path: path, Message: msg})
}

// Finalize sorts findings into deterministic order (Error first, then Warning,
// then Info; within a level, by Code then Path then Message) and fills the
// Summary counts. Call before rendering.
func (r *Report) Finalize() {
	order := map[Level]int{LevelError: 0, LevelWarning: 1, LevelInfo: 2}
	sort.SliceStable(r.Findings, func(i, j int) bool {
		a, b := r.Findings[i], r.Findings[j]
		if order[a.Level] != order[b.Level] {
			return order[a.Level] < order[b.Level]
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Message < b.Message
	})
	r.Summary = Summary{}
	for _, f := range r.Findings {
		switch f.Level {
		case LevelError:
			r.Summary.Errors++
		case LevelWarning:
			r.Summary.Warnings++
		case LevelInfo:
			r.Summary.Infos++
		}
	}
}

// HasErrors reports whether the report contains any Error-level findings.
// Safe to call before Finalize; scans the findings list directly so callers
// do not have to remember the ordering.
func (r *Report) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Level == LevelError {
			return true
		}
	}
	return false
}

// WriteText renders the report as human-readable lines. Finalize should have
// been called first so findings are ordered and counts are accurate.
func (r *Report) WriteText(w io.Writer, recordPath string) error {
	if len(r.Findings) == 0 {
		_, err := fmt.Fprintf(w, "OK -- %s: 0 errors, 0 warnings, 0 infos.\n", recordPath)
		return err
	}
	for _, f := range r.Findings {
		loc := ""
		if f.Path != "" {
			loc = " at " + f.Path
		}
		if _, err := fmt.Fprintf(w, "%-7s %s%s: %s\n", f.Level, f.Code, loc, f.Message); err != nil {
			return err
		}
	}
	status := "FAIL"
	if r.Summary.Errors == 0 {
		status = "OK"
	}
	_, err := fmt.Fprintf(w, "\n%s -- %s: %d errors, %d warnings, %d infos.\n",
		status, recordPath, r.Summary.Errors, r.Summary.Warnings, r.Summary.Infos)
	return err
}

// WriteJSON emits the report as pretty-printed JSON on w.
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}
