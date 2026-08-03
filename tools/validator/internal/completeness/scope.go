package completeness

import (
	"sort"
	"strings"
)

// This file exports the grading-scope manifest: which graded items the rubric
// holds in scope for a given record, in result form. It states results only and
// never the predicate logic that decided them, so the rubric table, the rule
// struct and every predicate stay unexported.
//
// Scope covers the GATING tiers (core, standard, full) only. The two non-gating
// sentinels (enrichment, observation) are excluded by definition: they are not
// graded items, and their applicability moves as a record fills in. If non-gating
// guidance is ever exported it arrives in its own slice, never inside Scope.

// ScopeKind classifies the FORM of a rubric row's path string, and nothing else.
type ScopeKind string

const (
	// ScopeKindPointer is a single RFC 6901 pointer naming the graded location.
	// The location may be a leaf, an object or an array; the kind says nothing
	// about what constitutes a satisfying value there.
	ScopeKindPointer ScopeKind = "pointer"
	// ScopeKindChoice is a pointer carrying a parenthetical alternative, where
	// either of two locations satisfies the row.
	ScopeKindChoice ScopeKind = "choice"
	// ScopeKindRequirement is a prose label for a predicate-backed requirement
	// that no single pointer names.
	ScopeKindRequirement ScopeKind = "requirement"
)

// ScopeItem is one graded item the rubric holds in scope for a record. Path,
// Document and Standard are the rubric row's own strings, shared verbatim with
// conformance/gap findings so the two surfaces can never disagree on identity.
// Blocks names the top-level record blocks the item's evidence lives in. An item
// carries no presence or satisfaction state: scope is scope, and validate reports
// the gaps.
type ScopeItem struct {
	Level    Level
	Kind     ScopeKind
	Path     string
	Document string
	Standard string
	Blocks   []string
}

// choiceBlocks maps each ScopeKindChoice gating path to the blocks it can be
// satisfied in. The mapping is explicit rather than parsed, because splitting the
// parenthetical of the input-voltage row yields input_voltage_class, which is a
// leaf inside electrical and not a block of its own.
var choiceBlocks = map[string][]string{
	"/electrical/input_voltage_v (or input_voltage_class)":         {"electrical"},
	"/lumen_maintenance_luminaire (or /lumen_maintenance_package)": {"lumen_maintenance_luminaire", "lumen_maintenance_package"},
}

// requirementBlocks maps each ScopeKindRequirement gating path to the blocks its
// present-closure reads. The three attestation-program rows route through
// attestationPrograms, which reads both the top-level attestations array and
// product_family.shared_attestations, so both blocks are named.
var requirementBlocks = map[string][]string{
	"safety listing (UL/cUL/ETL/CSA for NA; CE/ENEC/IEC 60598 otherwise)": {"attestations", "product_family"},
	"UL 924 listing":                                         {"attestations", "product_family"},
	"LM-79 attestation":                                      {"attestations", "product_family"},
	"instrumentation depth (goniometer/lab)":                 {"instrumentation"},
	"method-backed lumen maintenance (TM-21 hours or TM-28)": {"lumen_maintenance_luminaire", "lumen_maintenance_package"},
	"test-report-backed sign-face luminance":                 {"exit_sign"},
	"test-report-backed face illuminance":                    {"exit_sign"},
}

// isGating reports whether a level is one of the three gating tiers. The two
// sentinel levels and the incomplete floor are not.
func isGating(l Level) bool {
	return l == LevelCore || l == LevelStandard || l == LevelFull
}

// scopeKindOf classifies a rubric path by its form. Stated positively and
// deliberately NOT delegated to the drift test's shapeTestable, which consults a
// test-only allowlist and would reclassify seven pointer rows as requirements.
func scopeKindOf(path string) ScopeKind {
	if strings.Contains(path, " (or ") {
		return ScopeKindChoice
	}
	if strings.HasPrefix(path, "/") {
		return ScopeKindPointer
	}
	return ScopeKindRequirement
}

// scopeBlocksFor returns the blocks a gating row contributes: a pointer row's
// first path segment, or the row's entry in the explicit choice / requirement
// table. Both tables are total over their kinds, asserted by the scope tests.
func scopeBlocksFor(kind ScopeKind, path string) []string {
	var src []string
	switch kind {
	case ScopeKindChoice:
		src = choiceBlocks[path]
	case ScopeKindRequirement:
		src = requirementBlocks[path]
	default:
		// A pointer row needs no copy: the slice is freshly built here and
		// aliases nothing shared.
		rest := strings.TrimPrefix(path, "/")
		if i := strings.Index(rest, "/"); i >= 0 {
			rest = rest[:i]
		}
		if rest == "" {
			return []string{}
		}
		return []string{rest}
	}
	// Only the two package-level tables reach here, so copy: a caller holding a
	// ScopeItem must never be able to mutate the shared table through it.
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// Scope returns the graded items the rubric holds in scope for record, sorted by
// tier (core, standard, full) then by path. It walks the rubric once, keeps the
// gating rows whose applicability predicate is universal or true, and reports the
// result only. It is pure and total: any top-level JSON object yields a manifest,
// including an empty one, which scopes as a generic luminaire.
func Scope(record map[string]any) []ScopeItem {
	out := []ScopeItem{}
	for _, ru := range rubric {
		if !isGating(ru.level) {
			continue
		}
		if ru.applicable != nil && !ru.applicable(record) {
			continue
		}
		kind := scopeKindOf(ru.path)
		out = append(out, ScopeItem{
			Level:    ru.level,
			Kind:     kind,
			Path:     ru.path,
			Document: ru.document,
			Standard: ru.standard,
			Blocks:   scopeBlocksFor(kind, ru.path),
		})
	}
	// SliceStable, not Slice. (Level, Path) is unique across the gating band and
	// a test pins that, so the two are indistinguishable today and no test can
	// tell them apart. It is chosen anyway: the manifest is a byte-compared
	// published contract, and if that uniqueness ever lapses the output should
	// degrade to rubric order rather than to whatever the sort's internals
	// happen to do on that Go release.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level < out[j].Level
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// RollupBlocks returns the sorted, de-duplicated union of the items' blocks. It
// is always non-nil, so a caller marshalling it emits [] rather than null for an
// empty item list. Today its only caller is the CLI, which publishes the result
// as the manifest's `blocks` array: a rollup of what Scope already decided, never
// an independent judgement. A block in the result means at least one in-scope
// item's evidence lives there, not that the whole block is graded. It lives here,
// beside the per-item Blocks it unions, so the package that owns the rubric also
// owns and tests the shipped derivation.
func RollupBlocks(items []ScopeItem) []string {
	seen := map[string]bool{}
	for _, it := range items {
		for _, b := range it.Blocks {
			seen[b] = true
		}
	}
	out := make([]string, 0, len(seen))
	for b := range seen {
		out = append(out, b)
	}
	sort.Strings(out)
	return out
}
