package achievements

import (
	"testing"
	"time"
)

// plusDays returns iso + n days as an ISO date, for deterministic boundary fixtures.
func plusDays(iso string, n int) string {
	t, err := time.Parse("2006-01-02", iso)
	if err != nil {
		panic(err)
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

// rawRec builds a record from raw attestation arrays (so non-map and program-less entries
// can be placed at chosen indices) plus optional envelope fields.
func rawRec(attestations, shared []any, extra map[string]any) map[string]any {
	r := map[string]any{"attestations": attestations}
	if shared != nil {
		r["product_family"] = map[string]any{"shared_attestations": shared}
	}
	for k, v := range extra {
		r[k] = v
	}
	return r
}

// findEntry returns the entry at the given pointer from a slice, or false.
func findEntry(entries []ExpiryEntry, pointer string) (ExpiryEntry, bool) {
	for _, e := range entries {
		if e.Pointer == pointer {
			return e, true
		}
	}
	return ExpiryEntry{}, false
}

// T1: comparator boundaries, malformed/absent skipping, disqualified skipping, pointer paths
// across both ledger homes, the raw-index pointer pin, the declaration route, unthemed
// entries, previewing with no record_status_as_of, and summary counts.
func TestExpiryComparatorBoundaries(t *testing.T) {
	asOf := "2026-07-13"
	window := 90

	// valid_until == asOf: not lapsed, and within the window (0 days out) so upcoming.
	res := EvaluateExpiry(rec(att("hpd", map[string]any{"valid_until": asOf})), asOf, window)
	if len(res.Lapsed) != 0 {
		t.Errorf("valid_until == asOf: got %d lapsed, want 0", len(res.Lapsed))
	}
	if len(res.Upcoming) != 1 || res.Upcoming[0].DaysUntil != 0 {
		t.Errorf("valid_until == asOf: upcoming=%+v, want one entry 0 days out", res.Upcoming)
	}

	// valid_until == asOf + window: still upcoming (inclusive upper edge).
	edge := plusDays(asOf, window)
	res = EvaluateExpiry(rec(att("hpd", map[string]any{"valid_until": edge})), asOf, window)
	if len(res.Upcoming) != 1 || res.Upcoming[0].DaysUntil != window {
		t.Errorf("valid_until == asOf+window: upcoming=%+v, want one entry %d days out", res.Upcoming, window)
	}

	// valid_until == asOf + window + 1: outside the window, neither lapsed nor upcoming.
	res = EvaluateExpiry(rec(att("hpd", map[string]any{"valid_until": plusDays(asOf, window+1)})), asOf, window)
	if len(res.Lapsed) != 0 || len(res.Upcoming) != 0 {
		t.Errorf("valid_until beyond window: lapsed=%d upcoming=%d, want 0/0", len(res.Lapsed), len(res.Upcoming))
	}

	// valid_until == asOf - 1 day: lapsed.
	res = EvaluateExpiry(rec(att("hpd", map[string]any{"valid_until": plusDays(asOf, -1)})), asOf, window)
	if len(res.Lapsed) != 1 || len(res.Upcoming) != 0 {
		t.Errorf("valid_until == asOf-1: lapsed=%d upcoming=%d, want 1/0", len(res.Lapsed), len(res.Upcoming))
	}
}

func TestExpirySkipsMalformedAbsentDisqualified(t *testing.T) {
	asOf := "2026-07-13"
	lapsedDate := "2026-01-01"

	// Malformed and absent valid_until are silently skipped.
	res := EvaluateExpiry(rec(
		att("hpd", map[string]any{"valid_until": "not-a-date"}),
		att("hpd", nil),
	), asOf, 90)
	if len(res.Lapsed) != 0 || len(res.Upcoming) != 0 {
		t.Errorf("malformed/absent: lapsed=%d upcoming=%d, want 0/0", len(res.Lapsed), len(res.Upcoming))
	}

	// A disqualified-status entry is skipped even though it carries a lapsed valid_until.
	res = EvaluateExpiry(rec(
		att("hpd", map[string]any{"valid_until": lapsedDate, "status": "expired"}),
	), asOf, 90)
	if len(res.Lapsed) != 0 {
		t.Errorf("disqualified: got %d lapsed, want 0 (skipped by predicate)", len(res.Lapsed))
	}
}

func TestExpiryPointerPaths(t *testing.T) {
	asOf := "2026-07-13"
	lapsedDate := "2026-01-01"

	// Shared attestations get the /product_family/shared_attestations/<i> pointer.
	res := EvaluateExpiry(rawRec(
		[]any{},
		[]any{att("hpd", map[string]any{"valid_until": lapsedDate})},
		nil,
	), asOf, 90)
	if _, ok := findEntry(res.Lapsed, "/product_family/shared_attestations/0"); !ok {
		t.Errorf("shared_attestations pointer: lapsed=%+v, want /product_family/shared_attestations/0", res.Lapsed)
	}

	// POINTER-INDEX pin: a program-less entry at index 0 preceding a dated entry at index 1
	// yields the raw-array pointer /attestations/1, not the emitted-position /attestations/0.
	res = EvaluateExpiry(rawRec(
		[]any{
			map[string]any{"attestation_id": "no-program"},
			att("hpd", map[string]any{"valid_until": lapsedDate}),
		},
		nil, nil,
	), asOf, 90)
	if len(res.Lapsed) != 1 {
		t.Fatalf("pointer-index pin: got %d lapsed, want 1", len(res.Lapsed))
	}
	if res.Lapsed[0].Pointer != "/attestations/1" {
		t.Errorf("pointer-index pin: pointer=%q, want /attestations/1", res.Lapsed[0].Pointer)
	}
}

func TestExpiryDeclarationRoute(t *testing.T) {
	asOf := "2026-07-13"
	ptr := "/sustainability_declaration/expiration_date"

	// Declaration lapsed: declaration_type present, expiration_date before asOf.
	res := EvaluateExpiry(map[string]any{
		"sustainability_declaration": map[string]any{
			"declaration_type": "red_list_approved",
			"expiration_date":  "2026-01-01",
		},
	}, asOf, 90)
	e, ok := findEntry(res.Lapsed, ptr)
	if !ok || !e.IsDeclaration {
		t.Errorf("declaration lapsed: lapsed=%+v, want declaration entry at %s", res.Lapsed, ptr)
	}

	// Declaration upcoming, including a manufacturer_recycle_program type (present as a
	// string, so the surface is evaluated regardless of which type it is).
	res = EvaluateExpiry(map[string]any{
		"sustainability_declaration": map[string]any{
			"declaration_type": "manufacturer_recycle_program",
			"expiration_date":  plusDays(asOf, 30),
		},
	}, asOf, 90)
	if e, ok := findEntry(res.Upcoming, ptr); !ok || e.DaysUntil != 30 {
		t.Errorf("declaration upcoming: upcoming=%+v, want declaration entry 30 days out", res.Upcoming)
	}

	// No declaration_type: the dated declaration is not surfaced.
	res = EvaluateExpiry(map[string]any{
		"sustainability_declaration": map[string]any{"expiration_date": "2026-01-01"},
	}, asOf, 90)
	if len(res.Lapsed) != 0 || len(res.Upcoming) != 0 {
		t.Errorf("typeless declaration: lapsed=%d upcoming=%d, want 0/0", len(res.Lapsed), len(res.Upcoming))
	}
}

func TestExpiryUnthemedEntryReported(t *testing.T) {
	asOf := "2026-07-13"
	// An unthemed program (ce is a regional market-access mark, in no theme) is still a dated
	// ledger surface: it is reported at entry level, names its program, and never a theme.
	res := EvaluateExpiry(rec(att("ce", map[string]any{"valid_until": "2026-01-01"})), asOf, 90)
	if len(res.Lapsed) != 1 || res.Lapsed[0].Program != "ce" {
		t.Errorf("unthemed lapsed: lapsed=%+v, want one ce entry", res.Lapsed)
	}
	if len(res.Downgrades) != 0 {
		t.Errorf("unthemed: got downgrades %v, want none", res.Downgrades)
	}
}

func TestExpiryPreviewsWithoutRecordAsOf(t *testing.T) {
	asOf := "2026-07-13"
	// A record with no record_status_as_of still previews against asOf: an evidence-bearing
	// themed attestation lapsed at asOf downgrades documented (record-relative) to claimed.
	res := EvaluateExpiry(rec(att("hpd", map[string]any{
		"valid_until":         "2026-01-01",
		"source_document_ref": evidence(),
	})), asOf, 90)
	if len(res.Downgrades) != 1 || res.Downgrades[0] != ThemeMaterialHealth {
		t.Errorf("preview without record_status_as_of: downgrades=%v, want [material_health]", res.Downgrades)
	}
}

func TestExpirySummaryCounts(t *testing.T) {
	asOf := "2026-07-13"
	// Two lapsed attestations, one upcoming attestation, one lapsed declaration.
	res := EvaluateExpiry(map[string]any{
		"attestations": []any{
			att("hpd", map[string]any{"valid_until": "2026-01-01"}),
			att("declare", map[string]any{"valid_until": "2026-02-01"}),
			att("dlc_qpl", map[string]any{"valid_until": plusDays(asOf, 30)}),
		},
		"sustainability_declaration": map[string]any{
			"declaration_type": "red_list_approved",
			"expiration_date":  "2026-03-01",
		},
	}, asOf, 90)
	if len(res.Lapsed) != 3 {
		t.Errorf("summary: got %d lapsed, want 3 (2 attestations + 1 declaration)", len(res.Lapsed))
	}
	if len(res.Upcoming) != 1 {
		t.Errorf("summary: got %d upcoming, want 1", len(res.Upcoming))
	}
}

// T2: synthetic downgrade pins. A theme downgrades only when no documented entry survives asOf.
func TestExpiryDowngradeSingleEntry(t *testing.T) {
	asOf := "2026-07-13"
	// One evidence-bearing attestation, valid_until after record_status_as_of (documented
	// record-relatively) but before asOf (lapsed at the preview date): exactly one downgrade
	// for its theme plus one lapsed for the entry.
	res := EvaluateExpiry(map[string]any{
		"record_status_as_of": "2026-01-01",
		"attestations": []any{
			att("hpd", map[string]any{"valid_until": "2026-03-01", "source_document_ref": evidence()}),
		},
	}, asOf, 90)
	if len(res.Downgrades) != 1 || res.Downgrades[0] != ThemeMaterialHealth {
		t.Errorf("single-entry downgrade: downgrades=%v, want [material_health]", res.Downgrades)
	}
	if len(res.Lapsed) != 1 {
		t.Errorf("single-entry downgrade: got %d lapsed, want 1", len(res.Lapsed))
	}
}

func TestExpiryDowngradeRedundancyHolds(t *testing.T) {
	asOf := "2026-07-13"
	// Two evidence-bearing documented entries on material_health; one lapses at asOf, the
	// other survives. The theme holds: lapsed yes, downgrade NO.
	res := EvaluateExpiry(map[string]any{
		"record_status_as_of": "2026-01-01",
		"attestations": []any{
			att("hpd", map[string]any{"valid_until": "2026-03-01", "source_document_ref": evidence()}),
			att("declare", map[string]any{"valid_until": "2027-01-01", "source_document_ref": evidence()}),
		},
	}, asOf, 90)
	if len(res.Lapsed) != 1 {
		t.Errorf("redundancy: got %d lapsed, want 1", len(res.Lapsed))
	}
	if len(res.Downgrades) != 0 {
		t.Errorf("redundancy: got downgrades %v, want none (a documented entry survives)", res.Downgrades)
	}
}

// T5: walk-parity drift guard. The set of (program, attestation_id, pointer) the expiry walk
// consumes must equal what mergeLedger yields, so any future mergeLedger skip-rule change
// breaks this test. Also pins that the disqualified and undated entries mergeLedger yields are
// correctly filtered out of the reported results while sharing mergeLedger's pointers.
func TestExpiryWalkParity(t *testing.T) {
	asOf := "2026-07-13"
	fixture := rawRec(
		[]any{
			"not-a-map", // index 0: skipped by mergeLedger
			map[string]any{"attestation_id": "no-program"},                                                            // index 1: program-less, skipped
			att("hpd", map[string]any{"attestation_id": "a", "valid_until": "2026-01-01"}),                            // index 2: lapsed
			att("declare", map[string]any{"attestation_id": "b", "valid_until": "2026-01-01", "status": "withdrawn"}), // index 3: disqualified
		},
		[]any{
			att("dlc_qpl", map[string]any{"attestation_id": "c", "valid_until": plusDays(asOf, 30)}), // shared index 0: upcoming
		},
		nil,
	)

	type triple struct{ program, id, pointer string }
	want := map[triple]bool{
		{"hpd", "a", "/attestations/2"}:                           true,
		{"declare", "b", "/attestations/3"}:                       true,
		{"dlc_qpl", "c", "/product_family/shared_attestations/0"}: true,
	}
	got := map[triple]bool{}
	for _, e := range mergeLedger(fixture) {
		got[triple{e.program, e.id, e.pointer}] = true
	}
	if len(got) != len(want) {
		t.Fatalf("mergeLedger yielded %d entries, want %d: %+v", len(got), len(want), got)
	}
	for k := range want {
		if !got[k] {
			t.Errorf("mergeLedger missing %+v", k)
		}
	}

	// The reported entries share mergeLedger's pointers; the disqualified entry (index 3) is
	// filtered out, and the two non-map/program-less entries never appear.
	res := EvaluateExpiry(fixture, asOf, 90)
	if _, ok := findEntry(res.Lapsed, "/attestations/2"); !ok {
		t.Errorf("expiry lapsed missing /attestations/2: %+v", res.Lapsed)
	}
	if _, ok := findEntry(res.Lapsed, "/attestations/3"); ok {
		t.Errorf("expiry lapsed wrongly includes disqualified /attestations/3")
	}
	if _, ok := findEntry(res.Upcoming, "/product_family/shared_attestations/0"); !ok {
		t.Errorf("expiry upcoming missing /product_family/shared_attestations/0: %+v", res.Upcoming)
	}
}
