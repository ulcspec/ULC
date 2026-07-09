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
	// CodeConformanceLevel is the INFO summary naming the achieved grade.
	CodeConformanceLevel Code = "conformance/level"
	// CodeConformanceGradeSatisfied is the INFO finding emitted for each grade at or
	// below the achieved grade: that grade is genuinely met. Carries the grade name in
	// its message.
	CodeConformanceGradeSatisfied Code = "conformance/grade-satisfied"
	// CodeConformanceGradeGated is the INFO finding emitted for an outstanding grade
	// (above the achieved grade) whose own requirements are nonetheless all met: it is
	// gated only by an unmet lower grade. This is the cascade signal (close the lower
	// grade and this one unlocks). A code-keyed consumer MUST treat this as "NOT
	// achieved": the achieved grade is index.conformance_level, never this finding.
	CodeConformanceGradeGated Code = "conformance/grade-gated"
	// CodeConformanceGap is the INFO roadmap guidance: one finding per hard field a
	// record must add to reach a given grade (conditional predicates applied), each
	// carrying the structured source-document and standard detail.
	CodeConformanceGap Code = "conformance/gap"
	// CodeConformanceEnrichment is an INFO surfaced at core and above: the
	// enrichment roadmap. Each finding names an optional dimension a record could
	// disclose to deepen the datasheet (thermal, flicker, circadian, chromaticity
	// shift, outdoor, and similar depth), carrying its source-document and standard.
	// Non-gating: it never affects the achieved grade. Suppressed from text output
	// unless --verbose; always present in JSON.
	CodeConformanceEnrichment Code = "conformance/enrichment"
	// CodeConformanceObservation is an INFO surfaced at core and above: a data-quality
	// note (a non-measured headline value, the attestation-coverage summary) or a
	// tracked-but-not-nudged disclosure (a sustainability declaration, a deprecated
	// legacy-cutoff classification). Distinct from the enrichment roadmap, which
	// suggests new disclosures. Suppressed from text output unless --verbose; always
	// present in JSON.
	CodeConformanceObservation Code = "conformance/observation"

	// Product achievements. The achievements axis is computed by the builder into
	// index.achievements (guarded by the build-parity check) and surfaced here as
	// INFO findings, orthogonal to conformance grading and never a defect. It reports
	// per-theme third-party program qualifications and the evidence attached to them.
	//
	// CodeAchievementsSummary is the one default-visible headline per record: how many
	// themes are documented and how many are claimed.
	CodeAchievementsSummary Code = "achievements/summary"
	// CodeAchievementsState is emitted once per non-none theme, carrying the theme, its
	// state, and the qualifying programs. Suppressed from text output unless --verbose;
	// always present in JSON.
	CodeAchievementsState Code = "achievements/state"
	// CodeAchievementsRoadmap is the claimed-to-documented roadmap: for each claimed theme
	// that names at least one program, how to raise it to documented (attach the
	// certificate). Suppressed from text output unless --verbose; always present in JSON.
	CodeAchievementsRoadmap Code = "achievements/roadmap"
)

// Finding is a single diagnostic.
type Finding struct {
	Level   Level  `json:"level"`
	Code    Code   `json:"code"`
	Message string `json:"message"`
	// Path is an optional JSON Pointer into the record that located the problem.
	Path string `json:"path,omitempty"`
	// NextConformanceLevel, SourceDocument, and Standard are the structured
	// roadmap detail. NextConformanceLevel is set on conformance/gap findings only
	// (via AddRoadmap): it names the tier a missing item unlocks. SourceDocument and
	// Standard are set on both conformance/gap (via AddRoadmap) and
	// conformance/enrichment (via AddEnrichment) findings: the source document that
	// supplies the item and the standard that governs it. Together they form the
	// capped machine-readable roadmap contract a future website consumes. All three
	// are static rule-table strings, never echoed record input, so there is no
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
	// omits the optional conformance findings (the enrichment roadmap and the
	// observation notes) so the human report stays focused on errors, warnings, the
	// achieved level, and the roadmap to the next level. WriteJSON always emits every
	// finding regardless of this flag. Not serialized.
	Verbose bool `json:"-"`
	// OmitFlagHint drops the "use --verbose or --json" advice from the hidden-findings
	// hint in WriteText, for callers that do not expose those flags (for example
	// `ulc from-sheet`). The count of hidden optional findings is still reported. Not
	// serialized.
	OmitFlagHint bool `json:"-"`
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

// AddEnrichment appends an INFO enrichment finding carrying the structured
// source-document and standard detail alongside the human-readable message. It
// mirrors AddRoadmap without NextConformanceLevel: an enrichment suggestion
// unlocks no tier (it is non-gating depth), so that field stays zero.
func (r *Report) AddEnrichment(code Code, path, document, standard, msg string) {
	r.Add(Finding{Level: LevelInfo, Code: code, Path: path, Message: msg,
		SourceDocument: document, Standard: standard})
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
	hidden := 0
	for _, f := range r.Findings {
		// The optional findings are suppressed in text unless Verbose is set: the
		// enrichment roadmap and observation notes, plus the verbose-only achievements
		// detail (per-theme states and the claimed-to-documented roadmap). The
		// achieved-level summary, the tier roadmap, and the achievements headline (other
		// conformance and achievements codes) always render. WriteJSON keeps them all.
		if !r.Verbose && (f.Code == CodeConformanceEnrichment || f.Code == CodeConformanceObservation ||
			f.Code == CodeAchievementsState || f.Code == CodeAchievementsRoadmap) {
			hidden++
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
	hint := ""
	if hidden > 0 {
		hint = fmt.Sprintf(" (%d optional findings hidden (enrichment, observations, and achievements)", hidden)
		if r.OmitFlagHint {
			hint += ")"
		} else {
			hint += "; use --verbose or --json)"
		}
	}
	_, err := fmt.Fprintf(w, "\n%s -- %s: %d errors, %d warnings, %d infos%s.\n",
		status, recordPath, r.Summary.Errors, r.Summary.Warnings, r.Summary.Infos, hint)
	return err
}

// WriteJSON emits the report as pretty-printed JSON on w.
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}
