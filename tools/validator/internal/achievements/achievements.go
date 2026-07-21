// Package achievements computes the Product Achievements view over a ULC record's
// Master Ledger: per-theme third-party program qualifications (none / claimed /
// documented) and the restricted-substances compliance flag. It is the second
// grading axis, orthogonal to data completeness: it never touches the conformance
// ladder and no completeness rubric reads its output.
//
// The types here are COPIED from the completeness package's Result/Item shapes, not
// imported (the completeness.Result contract note). The two packages must never
// import each other; a package-level import-graph test enforces the boundary in both
// directions.
//
// Compute is a pure function of one record. It reads exactly three inputs and nothing
// else: the merged attestation ledger (top-level attestations[] then
// product_family.shared_attestations[]), sustainability_declaration.declaration_type,
// and the envelope field record_status_as_of (used only for the record-relative expiry
// comparison). Wall clock never enters the compute, so index.achievements is
// reproducible: build-index --check and the golden byte-compares stay stable as time
// passes. Compute is total on hostile input (nil record, wrong types, junk entries
// contribute nothing and never panic) and never mutates its input record.
package achievements

// State is a theme's achievement state. none < claimed < documented.
type State int

const (
	// StateNone: no qualifying attestation or declaration contributes to the theme.
	StateNone State = iota
	// StateClaimed: a qualifying contribution exists but carries no attached evidence
	// document (or its only evidence is disqualified by status or record-relative expiry).
	StateClaimed
	// StateDocumented: at least one qualifying, non-disqualified attestation carries an
	// attached, unexpired evidence document. A record fact, never a verification by ULC.
	StateDocumented
)

// String returns the token form used in the index and findings.
func (s State) String() string {
	switch s {
	case StateClaimed:
		return "claimed"
	case StateDocumented:
		return "documented"
	default:
		return "none"
	}
}

// Theme is one theme's computed picture. Programs and SourceAttestationIDs are sorted
// and deduplicated (both non-nil, even when empty). BestMetricRef names the
// attestation_id whose sustainability_metric best represents the theme; it is empty
// (and omitted by the builder) unless a qualifying embodied_carbon or circularity
// attestation carries a metric.
type Theme struct {
	State                State
	Programs             []string
	SourceAttestationIDs []string
	EvidencePresent      bool
	BestMetricRef        string
}

// Item is one claimed-to-documented roadmap row. Unlike completeness.Item it carries no
// NextLevel (an achievement roadmap unlocks no tier) and no Document/Standard (the
// guidance is generic: attach the evidence document), so it holds just a path and message.
type Item struct {
	Path    string
	Message string
}

// Result is the structured achievements picture Compute returns. Themes always carries
// all seven fixed theme keys. RestrictedSubstances is the sibling legal-floor flag (the
// restricted-substances programs declared in the ledger), computed here for the builder
// but never an achievement theme. Roadmap holds the claimed-to-documented items.
type Result struct {
	Themes               map[string]Theme
	DocumentedCount      int
	RestrictedSubstances []string
	Roadmap              []Item
}
