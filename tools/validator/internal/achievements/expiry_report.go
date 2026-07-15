package achievements

import (
	"fmt"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// ReportExpiry appends the advisory expiry findings to report: one INFO summary always, one
// WARNING per lapsed entry, one WARNING per theme downgrade, and one INFO per upcoming entry.
// The word "advisory" in the summary line marks the whole surface non-normative for a consumer
// reading only the output. Every message is a static template over program tokens, theme
// constants, and parse-validated ISO dates; no free-text record input is echoed. This mirrors
// report.go but is called only when `--expiry` is set, so its findings never reach a default
// run.
func ReportExpiry(record map[string]any, asOf string, windowDays int, report *findings.Report) {
	res := EvaluateExpiry(record, asOf, windowDays)

	report.AddInfo(findings.CodeExpirySummary, "",
		fmt.Sprintf("expiry (advisory): %d lapsed, %d expiring within %d days (as of %s)",
			len(res.Lapsed), len(res.Upcoming), windowDays, asOf))

	for _, e := range res.Lapsed {
		if e.IsDeclaration {
			report.AddWarning(findings.CodeExpiryLapsed, e.Pointer,
				fmt.Sprintf("sustainability_declaration lapsed %s; it still contributes claimed on re-stamp because the compute reads only declaration_type", e.ValidUntil))
			continue
		}
		report.AddWarning(findings.CodeExpiryLapsed, e.Pointer,
			fmt.Sprintf("attestation (program %s) lapsed %s; renew it or set status: expired", e.Program, e.ValidUntil))
	}

	for _, th := range res.Downgrades {
		report.AddWarning(findings.CodeExpiryDowngrade, "/index/achievements/themes/"+th,
			fmt.Sprintf("theme %s: documented would become claimed if the record were re-stamped with record_status_as_of on or after %s", th, asOf))
	}

	for _, e := range res.Upcoming {
		if e.IsDeclaration {
			report.AddInfo(findings.CodeExpiryUpcoming, e.Pointer,
				fmt.Sprintf("sustainability_declaration expires %s (in %d days)", e.ValidUntil, e.DaysUntil))
			continue
		}
		report.AddInfo(findings.CodeExpiryUpcoming, e.Pointer,
			fmt.Sprintf("attestation (program %s) expires %s (in %d days)", e.Program, e.ValidUntil, e.DaysUntil))
	}
}
