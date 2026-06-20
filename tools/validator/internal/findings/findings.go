// Package findings is the structured diagnostic model the `ulc validate`
// command emits. Every check produces zero or more Finding values keyed by
// Level; callers aggregate them into a Report and render it as human-readable
// text or JSON.
//
// Severity semantics:
//
//	Error   a hard violation of the spec. The record is not ULC-conformant.
//	Warning a soft concern about a recoverable defect (e.g., a source file not
//	        reachable locally so its hash cannot be verified here).
//	Info    an observation the user might want but which is not a defect (e.g.,
//	        the computed conformance level and guidance toward the next level).
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

	// Conformance grading. The conformance level a record achieves is computed by
	// the builder and stored in index.conformance_level (guarded by the build-
	// parity check). Conformance grading therefore produces only INFO findings: a
	// human-facing report of what was computed, never a defect.
	//
	// CodeConformanceLevel is the INFO summary naming the achieved level.
	CodeConformanceLevel Code = "conformance/level"
	// CodeConformanceGap is the INFO roadmap guidance: one finding per hard field a
	// record must add to reach the next level up (conditional predicates applied),
	// each carrying the structured source-document and standard detail.
	CodeConformanceGap Code = "conformance/gap"
	// CodeConformanceObservation is an INFO surfaced at core and above: depth the
	// rubric does not gate on (thermal, flicker, circadian, sustainability, and
	// similar comprehensive data) or a provenance-quality note. Suppressed from
	// text output unless --verbose; always present in JSON.
	CodeConformanceObservation Code = "conformance/observation"
)

// Finding is a single diagnostic.
type Finding struct {
	Level   Level  `json:"level"`
	Code    Code   `json:"code"`
	Message string `json:"message"`
	// Path is an optional JSON Pointer into the record that located the problem.
	Path string `json:"path,omitempty"`
	// NextConformanceLevel, SourceDocument, and Standard are the structured
	// roadmap detail set on conformance/gap findings only (via AddRoadmap). They
	// are the complete, capped machine-readable roadmap contract a future website
	// consumes: for a missing item, which conformance level it unlocks, which
	// source document supplies it, and which standard governs it. All three are
	// static rule-table strings, never echoed record input, so there is no
	// disclosure or injection surface.
	NextConformanceLevel string `json:"next_conformance_level,omitempty"`
	SourceDocument       string `json:"source_document,omitempty"` // a SourceFileType token
	Standard             string `json:"standard,omitempty"`
}

// Report is the aggregate result of a validate run.
type Report struct {
	Findings []Finding `json:"findings"`
	// Summary counters; derived by Finalize but kept on the struct so JSON
	// consumers do not have to recompute.
	Summary Summary `json:"summary"`
	// Verbose controls text rendering only. When false (the default), WriteText
	// omits conformance observation findings (the comprehensive-depth nudges) so
	// the human report stays focused on errors, warnings, the achieved level, and
	// the roadmap to the next level. WriteJSON always emits every finding
	// regardless of this flag. Not serialized.
	Verbose bool `json:"-"`
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

// AddRoadmap appends an INFO finding carrying the structured roadmap detail
// (the conformance level it unlocks, the source document that supplies it, and
// the governing standard) in addition to the human-readable message. Used by the
// grader to surface, for each missing item, how a manufacturer climbs to the
// next conformance level.
func (r *Report) AddRoadmap(code Code, path, nextLevel, document, standard, msg string) {
	r.Add(Finding{Level: LevelInfo, Code: code, Path: path, Message: msg,
		NextConformanceLevel: nextLevel, SourceDocument: document, Standard: standard})
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
		// Conformance observations are the comprehensive-depth nudges; suppress
		// them in text unless Verbose is set. The achieved-level summary and the
		// roadmap (other conformance codes) always render. WriteJSON keeps them.
		if !r.Verbose && f.Code == CodeConformanceObservation {
			continue
		}
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
