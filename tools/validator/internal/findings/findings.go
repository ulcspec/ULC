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
	"strings"
	"unicode/utf8"
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

	// Attestation expiry advisory. Emitted ONLY on `ulc validate --expiry`, an opt-in,
	// report-only preview of attestation and declaration expiry against a caller-chosen
	// as-of date. Advisory: it never changes the exit code (its WARNINGs do not trip
	// HasErrors), never mutates the stamped index.achievements, and is absent from default
	// runs, so goldens and default output stay byte-identical. None of these codes joins the
	// WriteText verbose-hidden set: they are add-time gated on the flag, not render-time
	// suppressed, so an added expiry finding never shifts a default run's footer.
	//
	// CodeExpirySummary is the one always-emitted INFO headline per --expiry run: how many
	// dated surfaces are lapsed and how many expire within the window, as of the check date.
	// The word "advisory" in its message marks the whole surface non-normative in output.
	CodeExpirySummary Code = "expiry/summary"
	// CodeExpiryLapsed is a WARNING per dated surface (an attestation or the sustainability
	// declaration) whose date is already past the check date. Its Path is the entry's JSON
	// Pointer.
	CodeExpiryLapsed Code = "expiry/lapsed"
	// CodeExpiryDowngrade is a WARNING per theme that is documented record-relatively but
	// would drop to claimed if the record were re-stamped on or after the check date (an
	// evidence-bearing attestation lapses and no other documented entry survives). Its Path
	// is the theme's index pointer.
	CodeExpiryDowngrade Code = "expiry/downgrade"
	// CodeExpiryUpcoming is an INFO per dated surface expiring within the window, carrying the
	// actual day count from the check date. Its Path is the entry's JSON Pointer.
	CodeExpiryUpcoming Code = "expiry/upcoming"
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

// SanitizeText renders control characters as visible \xNN escapes, so text
// that a record supplies cannot forge report lines or drive the terminal.
// Filenames, record ids, and the messages that echo them are schema-valid with
// any content at all, and the text report prints them raw.
//
// It covers C0 (below U+0020), DEL, and C1 (U+0080 to U+009F). JSON output
// needs none of this for C0, which encoding/json always escapes, but it passes
// DEL and C1 through literally, so the two ranges are handled together here
// rather than split by what another encoder happens to cover.
//
// This is a display contract, not a parsing one: a filename containing the
// literal characters backslash-x-1-b renders the same as an escaped ESC.
// Consumers that need exact bytes use --json.
func SanitizeText(s string) string {
	if !needsEscaping(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			// A byte that is not valid UTF-8. Escape the raw byte rather than
			// the replacement rune it decodes to: a bare 0x9B is CSI on a
			// terminal that is not reading UTF-8, and the C1 range would
			// otherwise slip through as U+FFFD.
			fmt.Fprintf(&b, "\\x%02X", s[i])
		case isControl(r):
			fmt.Fprintf(&b, "\\x%02X", r)
		default:
			b.WriteString(s[i : i+size])
		}
		i += size
	}
	return b.String()
}

// needsEscaping reports whether the string carries anything the writer must
// not emit verbatim. It decodes the same way SanitizeText does, so a string
// holding raw non-UTF-8 bytes cannot take the untouched fast path.
func needsEscaping(s string) bool {
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if (r == utf8.RuneError && size == 1) || isControl(r) {
			return true
		}
		i += size
	}
	return false
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7F || (r >= 0x80 && r <= 0x9F)
}

// WriteText renders the report as human-readable lines. Finalize should have
// been called first so findings are ordered and counts are accurate.
//
// Every record-controlled string is sanitized at this boundary rather than
// where the message is built, so the messages themselves stay byte-identical
// and the escaping is applied in exactly one place.
func (r *Report) WriteText(w io.Writer, recordPath string) error {
	recordPath = SanitizeText(recordPath)
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
			loc = " at " + SanitizeText(f.Path)
		}
		if _, err := fmt.Fprintf(w, "%-7s %s%s: %s\n", f.Level, f.Code, loc, SanitizeText(f.Message)); err != nil {
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

// WriteJSON emits the report as pretty-printed JSON on w. Angle brackets and
// ampersands are escaped, matching `ulc scope`: the report echoes
// record-controlled strings that a consumer may inline into a page, and one
// escaping contract across the machine-readable surfaces is easier to rely on
// than a per-subcommand rule. JSON semantics are unchanged after parsing.
func (r *Report) WriteJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(true)
	return enc.Encode(r)
}
