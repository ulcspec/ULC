package achievements

import (
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// findFinding returns the first finding matching code and path, or false. It lets the
// rendering assertions below pin the exact level, path, and message of each emitted finding.
func findFinding(rep *findings.Report, code findings.Code, path string) (findings.Finding, bool) {
	for _, f := range rep.Findings {
		if f.Code == code && f.Path == path {
			return f, true
		}
	}
	return findings.Finding{}, false
}

// TestReportExpiryRenders exercises ReportExpiry itself (the rendering layer), which the CLI
// tests reach only for lapsed and upcoming: this pins the always-emitted summary, the
// attestation and declaration lapsed messages, the upcoming day-count message, and the
// downgrade WARNING that no other test renders. The record produces two lapsed surfaces (one
// evidence-bearing attestation that would downgrade material_health, plus a lapsed
// declaration), one upcoming attestation, and exactly one theme downgrade.
func TestReportExpiryRenders(t *testing.T) {
	asOf := "2026-07-13"
	record := map[string]any{
		"record_status_as_of": "2026-01-01",
		"attestations": []any{
			att("hpd", map[string]any{"valid_until": "2026-03-01", "source_document_ref": evidence()}),
			att("dlc_qpl", map[string]any{"valid_until": plusDays(asOf, 30)}),
		},
		"sustainability_declaration": map[string]any{
			"declaration_type": "manufacturer_recycle_program",
			"expiration_date":  "2026-02-01",
		},
	}

	rep := findings.NewReport()
	ReportExpiry(record, asOf, 90, rep)

	// Summary: always one INFO, counting entries (2 lapsed, 1 upcoming), never themes.
	if f, ok := findFinding(rep, findings.CodeExpirySummary, ""); !ok {
		t.Error("missing expiry/summary")
	} else {
		if f.Level != findings.LevelInfo {
			t.Errorf("summary level = %s, want INFO", f.Level)
		}
		want := "expiry (advisory): 2 lapsed, 1 expiring within 90 days (as of 2026-07-13)"
		if f.Message != want {
			t.Errorf("summary message = %q, want %q", f.Message, want)
		}
	}

	// Lapsed attestation: WARNING with the renew-or-set-status message.
	if f, ok := findFinding(rep, findings.CodeExpiryLapsed, "/attestations/0"); !ok {
		t.Error("missing expiry/lapsed at /attestations/0")
	} else {
		if f.Level != findings.LevelWarning {
			t.Errorf("lapsed level = %s, want WARNING", f.Level)
		}
		want := "attestation (program hpd) lapsed 2026-03-01; renew it or set status: expired"
		if f.Message != want {
			t.Errorf("lapsed attestation message = %q, want %q", f.Message, want)
		}
	}

	// Lapsed declaration: WARNING with the declaration-specific message.
	if f, ok := findFinding(rep, findings.CodeExpiryLapsed, "/sustainability_declaration/expiration_date"); !ok {
		t.Error("missing expiry/lapsed at /sustainability_declaration/expiration_date")
	} else {
		want := "sustainability_declaration lapsed 2026-02-01; it still contributes claimed on re-stamp because the compute reads only declaration_type"
		if f.Message != want {
			t.Errorf("lapsed declaration message = %q, want %q", f.Message, want)
		}
	}

	// Upcoming attestation: INFO with the actual day count.
	if f, ok := findFinding(rep, findings.CodeExpiryUpcoming, "/attestations/1"); !ok {
		t.Error("missing expiry/upcoming at /attestations/1")
	} else {
		if f.Level != findings.LevelInfo {
			t.Errorf("upcoming level = %s, want INFO", f.Level)
		}
		want := "attestation (program dlc_qpl) expires " + plusDays(asOf, 30) + " (in 30 days)"
		if f.Message != want {
			t.Errorf("upcoming message = %q, want %q", f.Message, want)
		}
	}

	// Downgrade: WARNING at the theme's index pointer. This branch is rendered by no other test.
	if f, ok := findFinding(rep, findings.CodeExpiryDowngrade, "/index/achievements/themes/material_health"); !ok {
		t.Error("missing expiry/downgrade at /index/achievements/themes/material_health")
	} else {
		if f.Level != findings.LevelWarning {
			t.Errorf("downgrade level = %s, want WARNING", f.Level)
		}
		want := "theme material_health: documented would become claimed if the record were re-stamped with record_status_as_of on or after 2026-07-13"
		if f.Message != want {
			t.Errorf("downgrade message = %q, want %q", f.Message, want)
		}
	}
}

// TestReportExpiryUpcomingDeclaration pins the one rendered message body the other tests leave
// unasserted: the upcoming-declaration variant (a contributing declaration whose expiration_date
// falls within the window). The attestation-upcoming variant is pinned in TestReportExpiryRenders.
func TestReportExpiryUpcomingDeclaration(t *testing.T) {
	asOf := "2026-07-13"
	record := map[string]any{
		"sustainability_declaration": map[string]any{
			"declaration_type": "ilfi_declare",
			"expiration_date":  plusDays(asOf, 45),
		},
	}
	rep := findings.NewReport()
	ReportExpiry(record, asOf, 90, rep)

	if f, ok := findFinding(rep, findings.CodeExpiryUpcoming, "/sustainability_declaration/expiration_date"); !ok {
		t.Error("missing expiry/upcoming at /sustainability_declaration/expiration_date")
	} else {
		if f.Level != findings.LevelInfo {
			t.Errorf("upcoming declaration level = %s, want INFO", f.Level)
		}
		want := "sustainability_declaration expires " + plusDays(asOf, 45) + " (in 45 days)"
		if f.Message != want {
			t.Errorf("upcoming declaration message = %q, want %q", f.Message, want)
		}
	}
}
