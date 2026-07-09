package achievements

import (
	"fmt"
	"strings"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// Report computes the achievements picture and appends its INFO findings to report:
// one default-visible summary, one verbose-only state finding per non-none theme, and
// the verbose-only claimed-to-documented roadmap. It emits no WARNINGs or ERRORs: an
// achievement state is a record fact, never a defect, and emission is not gated on the
// conformance level (the axes are orthogonal).
func Report(record map[string]any, report *findings.Report) {
	render(Compute(record), report)
}

// render emits the findings from a computed Result.
func render(res Result, report *findings.Report) {
	claimed := 0
	for _, th := range themeOrder {
		if res.Themes[th].State == StateClaimed {
			claimed++
		}
	}
	report.AddInfo(findings.CodeAchievementsSummary, "/index/achievements",
		fmt.Sprintf("achievements: %d documented, %d claimed", res.DocumentedCount, claimed))

	for _, th := range themeOrder {
		t := res.Themes[th]
		if t.State == StateNone {
			continue
		}
		report.Add(findings.Finding{
			Level:   findings.LevelInfo,
			Code:    findings.CodeAchievementsState,
			Path:    "/index/achievements/themes/" + th,
			Message: stateMessage(th, t),
		})
	}

	for _, it := range res.Roadmap {
		report.Add(findings.Finding{
			Level:   findings.LevelInfo,
			Code:    findings.CodeAchievementsRoadmap,
			Path:    it.Path,
			Message: it.Message,
		})
	}
}

// stateMessage builds a theme's verbose state line, naming the qualifying programs when
// any exist (the manufacturer_recycle_program-only circularity case has none). Program
// tokens are a closed vocabulary, never free-text record input.
func stateMessage(theme string, t Theme) string {
	if len(t.Programs) == 0 {
		return fmt.Sprintf("%s achievement: %s", theme, t.State.String())
	}
	return fmt.Sprintf("%s achievement: %s (%s)", theme, t.State.String(), strings.Join(t.Programs, ", "))
}
