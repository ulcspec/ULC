package achievements

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

// 5.2 State ladders per theme: none, then claimed (program, no doc), then documented
// (add source_document_ref) for each of the six themes.
func TestStateLaddersPerTheme(t *testing.T) {
	rep := map[string]string{
		ThemeEmbodiedCarbon: "epd_iso_14025",
		ThemeCircularity:    "tm66_assured",
		ThemeMaterialHealth: "hpd",
		ThemeEnergy:         "dlc_qpl",
		ThemeDarkSky:        "darksky_approved",
		ThemeEmergency:      "ul_924",
	}
	for theme, prog := range rep {
		if got := Compute(map[string]any{}).Themes[theme].State; got != StateNone {
			t.Errorf("%s empty: state=%s want none", theme, got)
		}
		if got := Compute(rec(att(prog, nil))).Themes[theme].State; got != StateClaimed {
			t.Errorf("%s program no doc: state=%s want claimed", theme, got)
		}
		doc := Compute(rec(att(prog, map[string]any{"source_document_ref": evidence()}))).Themes[theme]
		if doc.State != StateDocumented {
			t.Errorf("%s program+doc: state=%s want documented", theme, doc.State)
		}
		if !doc.EvidencePresent {
			t.Errorf("%s program+doc: evidence_present false", theme)
		}
		if !contains(doc.Programs, prog) {
			t.Errorf("%s: programs %v missing %q", theme, doc.Programs, prog)
		}
	}
}

// 5.2 best_metric_ref selection for the metric-bearing themes: a documented metric-bearing
// attestation wins over a claimed one; among same-state candidates the first in ledger
// order wins; no metric means best_metric_ref stays empty.
func TestBestMetricRefSelection(t *testing.T) {
	carbonDoc := map[string]any{"embodied_carbon_kgco2e": 10.0, "embodied_carbon_scope": "a1_a3", "embodied_carbon_functional_unit": "one luminaire"}

	// documented wins over an earlier claimed candidate.
	r := Compute(rec(
		att("epd_iso_14025", map[string]any{"attestation_id": "claimed_one", "sustainability_metric": metric(map[string]any{"ceam_score": 1.0})}),
		att("epd_iso_14025", map[string]any{"attestation_id": "doc_one", "source_document_ref": evidence(), "sustainability_metric": metric(carbonDoc)}),
	))
	if got := r.Themes[ThemeEmbodiedCarbon].BestMetricRef; got != "doc_one" {
		t.Errorf("best_metric_ref=%q want doc_one (documented wins)", got)
	}

	// among same-state (both claimed) candidates, first in ledger order wins.
	r = Compute(rec(
		att("tm66_assured", map[string]any{"attestation_id": "first", "sustainability_metric": metric(map[string]any{"ceam_score": 3.0})}),
		att("tm66_assured", map[string]any{"attestation_id": "second", "sustainability_metric": metric(map[string]any{"ceam_score": 4.0})}),
	))
	if got := r.Themes[ThemeCircularity].BestMetricRef; got != "first" {
		t.Errorf("best_metric_ref=%q want first (first in ledger order)", got)
	}

	// no metric -> best_metric_ref empty (builder omits it).
	r = Compute(rec(att("epd_iso_14025", map[string]any{"attestation_id": "x"})))
	if got := r.Themes[ThemeEmbodiedCarbon].BestMetricRef; got != "" {
		t.Errorf("best_metric_ref=%q want empty when no metric present", got)
	}
}

// best_metric_ref names an attestation_id, so an id-less metric-bearing attestation is not a
// candidate: it must not claim the slot nor shadow a later id-bearing metric of the same or a
// better state, and a metric that also routes to material_health (cradle_to_cradle) never
// populates that theme's best_metric_ref (the metric guard covers embodied_carbon/circularity only).
func TestBestMetricRefIdRequired(t *testing.T) {
	m := func() map[string]any { return metric(map[string]any{"ceam_score": 2.0}) }

	// id-less claimed metric first must not shadow a later id-bearing claimed metric.
	r := Compute(rec(
		att("epd_iso_14025", map[string]any{"sustainability_metric": m()}),
		att("epd_iso_14025", map[string]any{"attestation_id": "has_id", "sustainability_metric": m()}),
	))
	if got := r.Themes[ThemeEmbodiedCarbon].BestMetricRef; got != "has_id" {
		t.Errorf("best_metric_ref=%q want has_id (an id-less first metric must not shadow)", got)
	}

	// an id-less documented metric must not shadow an id-bearing claimed metric: the
	// referenceable claimed id wins over the unreferenceable documented one.
	r = Compute(rec(
		att("tm66_assured", map[string]any{"attestation_id": "claimed_id", "sustainability_metric": m()}),
		att("tm66_assured", map[string]any{"source_document_ref": evidence(), "sustainability_metric": m()}),
	))
	if got := r.Themes[ThemeCircularity].BestMetricRef; got != "claimed_id" {
		t.Errorf("best_metric_ref=%q want claimed_id (id-less documented must not shadow a referenceable claimed)", got)
	}

	// every metric candidate id-less -> best_metric_ref stays empty (builder omits it).
	r = Compute(rec(att("epd_iso_14025", map[string]any{"sustainability_metric": m()})))
	if got := r.Themes[ThemeEmbodiedCarbon].BestMetricRef; got != "" {
		t.Errorf("best_metric_ref=%q want empty when no metric bears an id", got)
	}

	// theme guard: a cradle_to_cradle metric sets circularity's best_metric_ref only.
	r = Compute(rec(att("cradle_to_cradle", map[string]any{"attestation_id": "c2c", "source_document_ref": evidence(), "sustainability_metric": m()})))
	if got := r.Themes[ThemeCircularity].BestMetricRef; got != "c2c" {
		t.Errorf("circularity best_metric_ref=%q want c2c", got)
	}
	if got := r.Themes[ThemeMaterialHealth].BestMetricRef; got != "" {
		t.Errorf("material_health best_metric_ref=%q want empty (the metric guard excludes material_health)", got)
	}
}

// 5.3 Evidence discriminator: source_document_ref present flips claimed to documented;
// hostile ref shapes contribute claimed at most.
func TestEvidenceDiscriminator(t *testing.T) {
	if got := Compute(rec(att("ul_924", map[string]any{"source_document_ref": evidence()}))).Themes[ThemeEmergency].State; got != StateDocumented {
		t.Errorf("well-formed evidence: state=%s want documented", got)
	}
	hostile := []any{
		"not-a-map",
		42.0,
		map[string]any{"filename": "x.pdf", "sha256": ""},
		map[string]any{"filename": "x.pdf"},
		map[string]any{"sha256": 123},
	}
	for i, ref := range hostile {
		th := Compute(rec(att("ul_924", map[string]any{"source_document_ref": ref}))).Themes[ThemeEmergency]
		if th.State != StateClaimed {
			t.Errorf("hostile ref %d: state=%s want claimed", i, th.State)
		}
		if th.EvidencePresent {
			t.Errorf("hostile ref %d: evidence_present true, want false", i)
		}
	}
}

// 5.4 Status disqualifiers: expired/withdrawn/not_applicable contribute nothing; other
// statuses contribute; the discriminator never reads status for the claimed/documented split.
func TestStatusDisqualifiers(t *testing.T) {
	for _, st := range []string{"expired", "withdrawn", "not_applicable"} {
		th := Compute(rec(att("ul_924", map[string]any{"status": st, "source_document_ref": evidence()}))).Themes[ThemeEmergency]
		if th.State != StateNone {
			t.Errorf("status %q: emergency=%s want none (as if absent)", st, th.State)
		}
	}
	for _, st := range []string{"claimed", "verified", "listed", "audited_member", "provisional_member"} {
		if got := Compute(rec(att("ul_924", map[string]any{"status": st}))).Themes[ThemeEmergency].State; got != StateClaimed {
			t.Errorf("status %q, no doc: emergency=%s want claimed", st, got)
		}
	}
	// A record whose ONLY ul_924 is withdrawn shows emergency none.
	if got := Compute(rec(att("ul_924", map[string]any{"status": "withdrawn"}))).Themes[ThemeEmergency].State; got != StateNone {
		t.Errorf("sole withdrawn ul_924: emergency=%s want none", got)
	}
	// The split comes from evidence, not status: a "claimed" status WITH a doc is documented.
	if got := Compute(rec(att("ul_924", map[string]any{"status": "claimed", "source_document_ref": evidence()}))).Themes[ThemeEmergency].State; got != StateDocumented {
		t.Errorf("status claimed + doc: emergency=%s want documented (evidence drives the split)", got)
	}
}

// 5.5 Record-relative expiry: valid_until < record_status_as_of caps at claimed; the
// on-the-day case documents; either date absent or malformed means no expiry evaluation.
func TestRecordRelativeExpiry(t *testing.T) {
	withAsOf := func(asOf string, a map[string]any) map[string]any {
		m := rec(a)
		if asOf != "" {
			m["record_status_as_of"] = asOf
		}
		return m
	}
	docRef := map[string]any{"source_document_ref": evidence()}
	merge := func(base map[string]any, kv map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range base {
			out[k] = v
		}
		for k, v := range kv {
			out[k] = v
		}
		return out
	}

	cases := []struct {
		name       string
		asOf       string
		validUntil string
		want       State
	}{
		{"expired caps to claimed", "2026-07-01", "2026-01-01", StateClaimed},
		{"on-the-day documents", "2026-07-01", "2026-07-01", StateDocumented},
		{"future documents", "2026-07-01", "2026-12-01", StateDocumented},
		{"asOf absent no eval", "", "2020-01-01", StateDocumented},
		{"malformed validUntil no eval", "2026-07-01", "not-a-date", StateDocumented},
		{"malformed asOf no eval", "garbage", "2020-01-01", StateDocumented},
	}
	for _, c := range cases {
		a := merge(att("ul_924", docRef), nil)
		if c.validUntil != "" {
			a["valid_until"] = c.validUntil
		}
		th := Compute(withAsOf(c.asOf, a)).Themes[ThemeEmergency]
		if th.State != c.want {
			t.Errorf("%s: state=%s want %s", c.name, th.State, c.want)
		}
		// Evidence presence is independent of the expiry cap.
		if !th.EvidencePresent {
			t.Errorf("%s: evidence_present false, but a doc is attached", c.name)
		}
	}

	// valid_until absent entirely: no eval, documented.
	if got := Compute(withAsOf("2026-07-01", att("ul_924", docRef))).Themes[ThemeEmergency].State; got != StateDocumented {
		t.Errorf("valid_until absent: state=%s want documented", got)
	}
}

// 5.6 C2C dual-route: one cradle_to_cradle attestation with evidence yields circularity
// documented AND material_health documented from the same source id.
func TestC2CDualRoute(t *testing.T) {
	r := Compute(rec(att("cradle_to_cradle", map[string]any{"attestation_id": "c2c1", "source_document_ref": evidence()})))
	circ := r.Themes[ThemeCircularity]
	mh := r.Themes[ThemeMaterialHealth]
	if circ.State != StateDocumented {
		t.Errorf("circularity=%s want documented", circ.State)
	}
	if mh.State != StateDocumented {
		t.Errorf("material_health=%s want documented", mh.State)
	}
	if !contains(circ.SourceAttestationIDs, "c2c1") {
		t.Errorf("circularity ids %v missing c2c1", circ.SourceAttestationIDs)
	}
	if !contains(mh.SourceAttestationIDs, "c2c1") {
		t.Errorf("material_health ids %v missing c2c1", mh.SourceAttestationIDs)
	}
}

// 5.7 Declaration mapping.
func TestDeclarationMapping(t *testing.T) {
	// Only sustainability_declaration (red_list_approved) shows material_health claimed
	// with the mapped token and no evidence.
	r := Compute(map[string]any{"sustainability_declaration": map[string]any{"declaration_type": "red_list_approved"}})
	mh := r.Themes[ThemeMaterialHealth]
	if mh.State != StateClaimed {
		t.Errorf("declaration red_list_approved: material_health=%s want claimed", mh.State)
	}
	if !contains(mh.Programs, "lbc_red_list_approved") {
		t.Errorf("declaration: programs %v missing lbc_red_list_approved", mh.Programs)
	}
	if mh.EvidencePresent {
		t.Error("declaration carries no evidence document")
	}

	// manufacturer_recycle_program: circularity claimed, EMPTY programs, material_health
	// untouched (no accidental cradle_to_cradle route), no roadmap item.
	r = Compute(map[string]any{"sustainability_declaration": map[string]any{"declaration_type": "manufacturer_recycle_program"}})
	circ := r.Themes[ThemeCircularity]
	if circ.State != StateClaimed {
		t.Errorf("manufacturer_recycle_program: circularity=%s want claimed", circ.State)
	}
	if len(circ.Programs) != 0 {
		t.Errorf("manufacturer_recycle_program: circularity programs=%v want empty", circ.Programs)
	}
	if r.Themes[ThemeMaterialHealth].State != StateNone {
		t.Errorf("manufacturer_recycle_program: material_health=%s want none (no dual-route)", r.Themes[ThemeMaterialHealth].State)
	}
	for _, it := range r.Roadmap {
		if strings.Contains(it.Path, ThemeCircularity) {
			t.Error("a zero-program claimed theme must emit no roadmap item")
		}
	}

	// Both declaration and matching ledger attestation: union without duplication.
	r = Compute(map[string]any{
		"attestations":               []any{att("lbc_red_list_approved", nil)},
		"sustainability_declaration": map[string]any{"declaration_type": "red_list_approved"},
	})
	n := 0
	for _, p := range r.Themes[ThemeMaterialHealth].Programs {
		if p == "lbc_red_list_approved" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("union should carry lbc_red_list_approved once, got %d", n)
	}
}

// Every declarationProgramTokens mapping routes its declaration_type to material_health
// claimed with the exact program token. Iterating the map itself keeps all four (and any
// future entry) behaviorally exercised, not just the red_list_approved case above.
func TestDeclarationTypeContributionsAllMapped(t *testing.T) {
	if len(declarationProgramTokens) == 0 {
		t.Fatal("declarationProgramTokens is empty")
	}
	for dt, want := range declarationProgramTokens {
		r := Compute(map[string]any{"sustainability_declaration": map[string]any{"declaration_type": dt}})
		mh := r.Themes[ThemeMaterialHealth]
		if mh.State != StateClaimed {
			t.Errorf("declaration %q: material_health=%s want claimed", dt, mh.State)
		}
		if !contains(mh.Programs, want) {
			t.Errorf("declaration %q: programs %v missing token %q", dt, mh.Programs, want)
		}
		if mh.EvidencePresent {
			t.Errorf("declaration %q: a declaration carries no evidence document", dt)
		}
	}
}

// The restricted-substances rollup applies the same status disqualifier as the themes: a
// disqualified restricted token is excluded, a clean one is included.
func TestRestrictedSubstancesDisqualified(t *testing.T) {
	if got := Compute(rec(att("rohs", nil))).RestrictedSubstances; !contains(got, "rohs") {
		t.Errorf("clean rohs: restricted=%v want it to include rohs", got)
	}
	for _, st := range []string{"withdrawn", "expired", "not_applicable"} {
		got := Compute(rec(att("rohs", map[string]any{"status": st}))).RestrictedSubstances
		if contains(got, "rohs") {
			t.Errorf("status %q: restricted=%v must exclude the disqualified rohs", st, got)
		}
	}
}

// 5.13 Hostile input: Compute is total and returns an all-none Result with a non-nil
// empty restricted array on junk.
func TestHostileInput(t *testing.T) {
	allNone := []map[string]any{
		nil,
		{"attestations": "not-an-array"},
		{"attestations": []any{"not-a-map", 42.0, nil}},
		{"attestations": []any{map[string]any{"program": ""}, map[string]any{"program": 123}, map[string]any{}}},
		{"product_family": "not-a-map", "attestations": "junk"},
		{"product_family": map[string]any{"shared_attestations": "junk"}, "attestations": []any{"junk"}},
		{"sustainability_declaration": "not-a-map"},
		{"sustainability_declaration": map[string]any{"declaration_type": 99}},
	}
	for i, c := range allNone {
		r := Compute(c)
		for _, th := range themeOrder {
			if r.Themes[th].State != StateNone {
				t.Errorf("all-none case %d: %s=%s want none", i, th, r.Themes[th].State)
			}
		}
		if r.RestrictedSubstances == nil {
			t.Errorf("all-none case %d: restricted is nil, want non-nil empty", i)
		}
		if len(r.RestrictedSubstances) != 0 {
			t.Errorf("all-none case %d: restricted=%v want empty", i, r.RestrictedSubstances)
		}
	}

	// A valid program with a junk sustainability_metric: still claimed, metric ignored,
	// no panic, no best_metric_ref.
	r := Compute(rec(att("epd_iso_14025", map[string]any{"sustainability_metric": "junk"})))
	if r.Themes[ThemeEmbodiedCarbon].State != StateClaimed {
		t.Errorf("junk metric: embodied_carbon=%s want claimed", r.Themes[ThemeEmbodiedCarbon].State)
	}
	if r.Themes[ThemeEmbodiedCarbon].BestMetricRef != "" {
		t.Error("junk metric should not set best_metric_ref")
	}
	// Both homes junk simultaneously does not panic (covered above); restricted still empty.
}

// 5.14 Roadmap exactness.
func TestRoadmapExactness(t *testing.T) {
	// A claimed theme yields exactly ONE item naming that theme and one program.
	r := Compute(rec(att("ul_924", nil)))
	if len(r.Roadmap) != 1 {
		t.Fatalf("claimed emergency: roadmap len=%d want 1", len(r.Roadmap))
	}
	it := r.Roadmap[0]
	if !strings.Contains(it.Path, ThemeEmergency) {
		t.Errorf("roadmap path %q does not name emergency", it.Path)
	}
	if !strings.Contains(it.Message, "ul_924") || !strings.Contains(it.Message, ThemeEmergency) {
		t.Errorf("roadmap message %q must name the theme and program", it.Message)
	}
	if it.Message != roadmapItem(ThemeEmergency, "ul_924").Message {
		t.Errorf("roadmap message not from the static table: %q", it.Message)
	}

	// documented yields zero.
	if r := Compute(rec(att("ul_924", map[string]any{"source_document_ref": evidence()}))); len(r.Roadmap) != 0 {
		t.Errorf("documented: roadmap len=%d want 0", len(r.Roadmap))
	}
	// none yields zero.
	if r := Compute(map[string]any{}); len(r.Roadmap) != 0 {
		t.Errorf("none: roadmap len=%d want 0", len(r.Roadmap))
	}
}

// 5.15 Determinism: two Compute calls marshal byte-identically; programs and ids are sorted.
func TestDeterminism(t *testing.T) {
	record := rec(
		att("declare", map[string]any{"attestation_id": "zeta"}),
		att("hpd", map[string]any{"attestation_id": "alpha"}),
	)
	b1, _ := json.Marshal(Compute(record))
	b2, _ := json.Marshal(Compute(record))
	if !bytes.Equal(b1, b2) {
		t.Error("Compute is not deterministic across calls")
	}
	mh := Compute(record).Themes[ThemeMaterialHealth]
	if !sort.StringsAreSorted(mh.Programs) {
		t.Errorf("programs not sorted: %v", mh.Programs)
	}
	if !sort.StringsAreSorted(mh.SourceAttestationIDs) {
		t.Errorf("source_attestation_ids not sorted: %v", mh.SourceAttestationIDs)
	}
}
