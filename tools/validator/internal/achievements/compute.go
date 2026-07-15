package achievements

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"time"
)

// ledgerEntry is one attestation from the merged ledger, carrying the context the
// achievements compute needs beyond its program token.
type ledgerEntry struct {
	program    string
	id         string         // attestation_id, "" if absent
	status     string         // AttestationStatus token, "" if absent
	validUntil string         // ISO date, "" if absent or malformed
	evidence   bool           // source_document_ref present with a non-empty sha256
	metric     map[string]any // sustainability_metric, nil if absent
	// pointer is the entry's JSON Pointer in the record (/attestations/<i> or
	// /product_family/shared_attestations/<i>, raw array index), filled by mergeLedger
	// during its single walk. Compute never reads it; it exists so expiry.go can consume
	// mergeLedger's output directly and report each entry's location without a second walk.
	pointer string
}

// Compute walks the merged ledger once and returns the full achievements picture. It is
// pure, total on hostile input, and never mutates record. It reports the record-relative
// picture, comparing evidence expiry against record_status_as_of.
func Compute(record map[string]any) Result {
	return computeAsOf(record, "")
}

// computeAsOf is Compute's implementation with an optional as-of override. An empty
// override reproduces Compute's exact behavior (evidence expiry compared against
// record_status_as_of); a non-empty override substitutes that ISO date for
// record_status_as_of in the expiry comparison (including when the record carries none),
// which the expiry advisory uses to preview the picture at a caller-chosen date. The
// override is trusted to be a valid ISO date (the CLI parses it before calling).
func computeAsOf(record map[string]any, asOfOverride string) Result {
	recordAsOf := isoDateOrEmpty(record["record_status_as_of"])
	if asOfOverride != "" {
		recordAsOf = asOfOverride
	}
	entries := mergeLedger(record)

	// Per-theme accumulator.
	type agg struct {
		state    State
		programs map[string]bool
		ids      map[string]bool
		evidence bool
		bestRef  string
		bestSet  bool
		bestDoc  bool
	}
	aggs := map[string]*agg{}
	for _, th := range themeOrder {
		aggs[th] = &agg{programs: map[string]bool{}, ids: map[string]bool{}}
	}

	raise := func(a *agg, to State) {
		if to > a.state {
			a.state = to
		}
	}
	contribute := func(theme string, e ledgerEntry, documented bool) {
		a := aggs[theme]
		a.programs[e.program] = true
		if e.id != "" {
			a.ids[e.id] = true
		}
		if e.evidence {
			a.evidence = true
		}
		if documented {
			raise(a, StateDocumented)
		} else {
			raise(a, StateClaimed)
		}
		// best_metric_ref names the attestation_id whose sustainability_metric best represents
		// the theme (embodied_carbon or circularity only): a documented metric-bearing
		// attestation wins over a claimed one, and among same-state candidates the first in
		// ledger order wins. A candidate must carry an attestation_id (there is nothing to
		// reference otherwise) and a metric that actually represents the theme, so an id-less
		// metric and a mismatched-payload metric (an embodied-carbon attestation whose metric
		// holds only a circularity figure, say) neither claim nor shadow the slot.
		if e.metric != nil && e.id != "" && metricRepresentsTheme(theme, e.metric) {
			if !a.bestSet || (documented && !a.bestDoc) {
				a.bestSet = true
				a.bestDoc = documented
				a.bestRef = e.id
			}
		}
	}

	for _, e := range entries {
		if disqualified(e.status) {
			continue
		}
		documented := e.evidence && !expired(e.validUntil, recordAsOf)
		for _, th := range themeOrder {
			if themeSets[th][e.program] {
				contribute(th, e, documented)
			}
		}
	}

	// Declaration contributions, always capped at claimed (a declaration carries no
	// evidence document). The mapped declaration types add a material_health program
	// token; manufacturer_recycle_program is a direct declaration-to-circularity(claimed)
	// rule with no program token.
	if sd, ok := record["sustainability_declaration"].(map[string]any); ok {
		if dt, ok := sd["declaration_type"].(string); ok {
			if tok, mapped := declarationProgramTokens[dt]; mapped {
				a := aggs[ThemeMaterialHealth]
				a.programs[tok] = true
				raise(a, StateClaimed)
			} else if dt == "manufacturer_recycle_program" {
				raise(aggs[ThemeCircularity], StateClaimed)
			}
		}
	}

	themes := make(map[string]Theme, len(themeOrder))
	docCount := 0
	var roadmap []Item
	for _, th := range themeOrder {
		a := aggs[th]
		t := Theme{
			State:                a.state,
			Programs:             sortedKeys(a.programs),
			SourceAttestationIDs: sortedKeys(a.ids),
			EvidencePresent:      a.evidence,
		}
		if a.bestSet {
			t.BestMetricRef = a.bestRef
		}
		themes[th] = t
		if a.state == StateDocumented {
			docCount++
		}
		// One claimed-to-documented roadmap item per claimed theme that names at least one
		// program. A claimed theme with zero programs (the manufacturer_recycle_program-only
		// circularity case) gets no item: there is no attestation to attach a document to.
		if a.state == StateClaimed && len(t.Programs) > 0 {
			roadmap = append(roadmap, roadmapItem(th, t.Programs[0]))
		}
	}

	return Result{
		Themes:               themes,
		DocumentedCount:      docCount,
		RestrictedSubstances: restrictedSubstances(entries),
		Roadmap:              roadmap,
	}
}

// mergeLedger reads the two ledger homes in the completeness attestationPrograms order:
// top-level attestations[] first, then product_family.shared_attestations[]. Non-map
// entries and entries with an empty program token are skipped.
func mergeLedger(record map[string]any) []ledgerEntry {
	var out []ledgerEntry
	collect := func(v any, prefix string) {
		arr, ok := v.([]any)
		if !ok {
			return
		}
		for i, a := range arr {
			m, ok := a.(map[string]any)
			if !ok {
				continue
			}
			p, ok := m["program"].(string)
			if !ok || p == "" {
				continue
			}
			// pointer uses the raw array index i, so a skipped non-map or program-less entry
			// does not shift the location of the entries that follow it.
			e := ledgerEntry{program: p, pointer: prefix + "/" + strconv.Itoa(i)}
			if id, ok := m["attestation_id"].(string); ok {
				e.id = id
			}
			if st, ok := m["status"].(string); ok {
				e.status = st
			}
			e.validUntil = isoDateOrEmpty(m["valid_until"])
			e.evidence = hasEvidence(m)
			if met, ok := m["sustainability_metric"].(map[string]any); ok {
				e.metric = met
			}
			out = append(out, e)
		}
	}
	collect(record["attestations"], "/attestations")
	if pf, ok := record["product_family"].(map[string]any); ok {
		collect(pf["shared_attestations"], "/product_family/shared_attestations")
	}
	return out
}

// restrictedSubstances returns the sorted, deduplicated restricted-substances program
// tokens from the non-disqualified entries. Never nil, so the builder emits [] not null.
func restrictedSubstances(entries []ledgerEntry) []string {
	set := map[string]bool{}
	for _, e := range entries {
		if disqualified(e.status) {
			continue
		}
		if restrictedSubstancePrograms[e.program] {
			set[e.program] = true
		}
	}
	return sortedKeys(set)
}

// metricRepresentsTheme reports whether a sustainability_metric carries a figure that
// belongs to the theme, so best_metric_ref names an attestation whose metric actually
// represents the theme (the schema: "the attestation_id whose sustainability_metric best
// represents the theme"). embodied_carbon needs a kg CO2e figure; circularity needs a TM66
// CEAM score or a Cradle to Cradle level. A mismatched payload (an embodied-carbon
// attestation whose metric holds only a ceam_score, say) does not qualify. Any non-metric
// theme is never a best_metric_ref candidate.
func metricRepresentsTheme(theme string, metric map[string]any) bool {
	switch theme {
	case ThemeEmbodiedCarbon:
		_, ok := metric["embodied_carbon_kgco2e"]
		return ok
	case ThemeCircularity:
		if _, ok := metric["ceam_score"]; ok {
			return true
		}
		_, ok := metric["c2c_overall_level"]
		return ok
	}
	return false
}

// sha256Pattern matches a FileReference sha256: exactly 64 lowercase hex characters.
var sha256Pattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// hasEvidence reports whether an attestation carries an attached evidence document: a
// source_document_ref shaped like a schema-valid FileReference, meaning both keys are present
// with the right types (a filename string and a 64-character lowercase-hex sha256), mirroring
// the schema's required [filename, sha256] and its sha256 pattern. Presence is the gate; the
// document bytes are never fetched or re-hashed, and the filename may be empty (the schema does
// not constrain its length) but it must be present as a string. A ref missing either key, with
// a non-string filename, or with an ill-formed sha256 is not evidence, so build-index cannot
// stamp a theme documented from a ref that schema validation would reject. Stays total on
// hostile fixtures.
func hasEvidence(att map[string]any) bool {
	ref, ok := att["source_document_ref"].(map[string]any)
	if !ok {
		return false
	}
	if _, ok := ref["filename"].(string); !ok {
		return false
	}
	sha, ok := ref["sha256"].(string)
	return ok && sha256Pattern.MatchString(sha)
}

// isoDateOrEmpty returns v as a full ISO date string (YYYY-MM-DD) if it is one, else "".
// A non-string or a malformed date evaluates as absent, so a malformed valid_until or
// record_status_as_of simply skips the expiry comparison.
func isoDateOrEmpty(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return ""
	}
	return s
}

// expired reports whether validUntil is strictly earlier than recordAsOf. When either
// date is absent, there is no expiry evaluation (not expired). Equal dates are not
// expired: an attestation valid through the record's as-of day still documents. The
// comparison is lexicographic, which equals chronological on valid full ISO dates.
func expired(validUntil, recordAsOf string) bool {
	if validUntil == "" || recordAsOf == "" {
		return false
	}
	return validUntil < recordAsOf
}

// disqualified reports whether a status token removes an attestation from all themes.
func disqualified(status string) bool {
	switch status {
	case "expired", "withdrawn", "not_applicable":
		return true
	}
	return false
}

// roadmapItem is the single source of truth for a claimed-to-documented roadmap row, so
// the structured Result and the emitted finding never drift on wording. The message is a
// static template over the theme and program tokens; it never echoes free-text record input.
func roadmapItem(theme, program string) Item {
	return Item{
		Path:    "/index/achievements/themes/" + theme,
		Message: fmt.Sprintf("to document the %s achievement, attach the certificate document to the %s attestation", theme, program),
	}
}

// sortedKeys returns the map's keys sorted, always non-nil (so empty arrays serialize as
// [] not null through the builder).
func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
