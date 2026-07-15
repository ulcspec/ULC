package achievements

import "time"

// The expiry advisory is an opt-in, report-only surface on `ulc validate --expiry`. It
// previews attestation and declaration expiry against a caller-chosen as-of date without
// touching the compute, the stamped index, or the exit code. It reuses mergeLedger's walk
// (via the ledgerEntry pointer field) so its entry set can never diverge from the ledger the
// achievements axis grades, and it computes theme downgrades by diffing the record-relative
// picture against the as-of picture. Every dated surface the achievements axis silently
// carries forward (an attestation valid_until, a sustainability_declaration expiration_date)
// is surfaced here so a stale record is visible before its next re-stamp. EvaluateExpiry is
// pure; the findings rendering lives in expiry_report.go.

// ExpiryEntry is one dated ledger surface (an attestation or the sustainability declaration)
// evaluated against the as-of date. Program is the attestation's program token, empty for the
// declaration. DaysUntil is the actual day count from the as-of date to ValidUntil, meaningful
// for upcoming entries.
type ExpiryEntry struct {
	Pointer       string
	Program       string
	ValidUntil    string
	DaysUntil     int
	IsDeclaration bool
}

// ExpiryResult is the advisory expiry picture: entries already lapsed as of the check date,
// entries expiring within the window, and the theme-level downgrades a re-stamp on or after
// the as-of date would cause. Lapsed and Upcoming count entries (attestations and the
// declaration alike); Downgrades count themes.
type ExpiryResult struct {
	AsOf       string
	WindowDays int
	Lapsed     []ExpiryEntry
	Upcoming   []ExpiryEntry
	Downgrades []string // theme keys, in themeOrder
}

// EvaluateExpiry returns the advisory expiry picture for record as of asOf (an ISO date the
// caller has already parsed) with the given upcoming-window in days. It is pure, total on
// hostile input, and never mutates record. asOf and windowDays are the caller's; the wall
// clock never enters here.
func EvaluateExpiry(record map[string]any, asOf string, windowDays int) ExpiryResult {
	res := ExpiryResult{AsOf: asOf, WindowDays: windowDays}

	classify := func(e ExpiryEntry) {
		switch {
		case expired(e.ValidUntil, asOf):
			res.Lapsed = append(res.Lapsed, e)
		case upcoming(e.ValidUntil, asOf, windowDays):
			e.DaysUntil = daysBetween(asOf, e.ValidUntil)
			res.Upcoming = append(res.Upcoming, e)
		}
	}

	// Attestation surfaces: reuse the merged ledger exactly. Disqualified-status entries
	// contribute nothing to the achievements axis, so their dated evidence is nothing to
	// surface; skip them by the same predicate the compute uses.
	for _, entry := range mergeLedger(record) {
		if disqualified(entry.status) {
			continue
		}
		if entry.validUntil == "" {
			continue
		}
		classify(ExpiryEntry{Pointer: entry.pointer, Program: entry.program, ValidUntil: entry.validUntil})
	}

	// Declaration surface: mergeLedger does not cover it. Evaluated only when the
	// declaration_type actually contributes a claim in the compute (declarationContributes),
	// so a declaration whose type has no achievements effect (including a typeless one) has
	// nothing to surface, and the lapsed message's "still contributes claimed" clause is
	// accurate for every type this reaches.
	if sd, ok := record["sustainability_declaration"].(map[string]any); ok {
		if dt, ok := sd["declaration_type"].(string); ok && declarationContributes(dt) {
			if vu := isoDateOrEmpty(sd["expiration_date"]); vu != "" {
				classify(ExpiryEntry{
					Pointer:       "/sustainability_declaration/expiration_date",
					ValidUntil:    vu,
					IsDeclaration: true,
				})
			}
		}
	}

	// Theme downgrades: diff the record-relative picture against the as-of picture. A theme
	// that is documented normatively but only claimed once evidence expiry is evaluated at
	// asOf would drop on the next re-stamp. The diff is exact under multi-entry redundancy:
	// if a second documented entry survives asOf, the theme holds and no downgrade is
	// reported.
	base := computeAsOf(record, "")
	preview := computeAsOf(record, asOf)
	for _, th := range themeOrder {
		if base.Themes[th].State == StateDocumented && preview.Themes[th].State == StateClaimed {
			res.Downgrades = append(res.Downgrades, th)
		}
	}

	return res
}

// upcoming reports whether validUntil falls within [asOf, asOf+windowDays], both ends
// inclusive. The window edge is decided on parsed time.Time values (AddDate then
// Before/After), so no lexicographic-equals-chronological assumption is relied on at the
// edge and a large window that would push the bound past year 9999 stays correct.
func upcoming(validUntil, asOf string, windowDays int) bool {
	vu, err := time.Parse("2006-01-02", validUntil)
	if err != nil {
		return false
	}
	from, err := time.Parse("2006-01-02", asOf)
	if err != nil {
		return false
	}
	if vu.Before(from) {
		return false
	}
	return !vu.After(from.AddDate(0, 0, windowDays))
}

// daysBetween returns the whole-day count from fromISO to toISO. Both are valid ISO dates
// parsed at UTC midnight, so their difference is an exact multiple of 24 hours.
func daysBetween(fromISO, toISO string) int {
	from, _ := time.Parse("2006-01-02", fromISO)
	to, _ := time.Parse("2006-01-02", toISO)
	return int(to.Sub(from).Hours() / 24)
}
