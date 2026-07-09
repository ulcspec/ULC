package achievements

import (
	"fmt"
	"sort"
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
}

// Compute walks the merged ledger once and returns the full achievements picture. It is
// pure, total on hostile input, and never mutates record.
func Compute(record map[string]any) Result {
	recordAsOf := isoDateOrEmpty(record["record_status_as_of"])
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
		// best_metric_ref applies to embodied_carbon and circularity only: a documented
		// metric-bearing attestation wins over a claimed one; among same-state candidates
		// the first in ledger order wins.
		if (theme == ThemeEmbodiedCarbon || theme == ThemeCircularity) && e.metric != nil {
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
			if tok, mapped := declarationTokens[dt]; mapped {
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
	collect := func(v any) {
		arr, ok := v.([]any)
		if !ok {
			return
		}
		for _, a := range arr {
			m, ok := a.(map[string]any)
			if !ok {
				continue
			}
			p, ok := m["program"].(string)
			if !ok || p == "" {
				continue
			}
			e := ledgerEntry{program: p}
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
	collect(record["attestations"])
	if pf, ok := record["product_family"].(map[string]any); ok {
		collect(pf["shared_attestations"])
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

// hasEvidence reports whether an attestation carries an attached evidence document: a
// source_document_ref that is an object with a non-empty sha256. Hostile shapes (a
// non-map ref, or a missing/empty sha256) are not evidence, so they support claimed at
// most. The schema guarantees filename + sha256 on a valid FileReference; this stays
// total for hostile fixtures.
func hasEvidence(att map[string]any) bool {
	ref, ok := att["source_document_ref"].(map[string]any)
	if !ok {
		return false
	}
	sha, ok := ref["sha256"].(string)
	return ok && sha != ""
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
