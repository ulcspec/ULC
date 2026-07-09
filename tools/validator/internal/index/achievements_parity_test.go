package index

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/achievements"
)

// exampleRecords loads every example record keyed by base filename.
func exampleRecords(t *testing.T) map[string]Record {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	matches, err := filepath.Glob(filepath.Join(root, "examples", "*.ulc"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("no example records found")
	}
	out := map[string]Record{}
	for _, path := range matches {
		record, err := loadRecord(path)
		if err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		out[filepath.Base(path)] = record
	}
	return out
}

// 5.11 TestBuildAchievementsMatchesCompute: the achievements subtree Build() stamps
// reflects achievements.Compute for every example (the builder is only the JSON-normalizing
// conversion layer, so the two must agree on states, count, and restricted programs).
func TestBuildAchievementsMatchesCompute(t *testing.T) {
	for name, record := range exampleRecords(t) {
		t.Run(name, func(t *testing.T) {
			built := Build(record)
			res := achievements.Compute(record)

			ach, ok := built["achievements"].(map[string]any)
			if !ok {
				t.Fatalf("built index has no achievements object")
			}
			themes, ok := ach["themes"].(map[string]any)
			if !ok {
				t.Fatalf("achievements has no themes object")
			}
			if len(themes) != len(res.Themes) {
				t.Errorf("themes count %d != Compute %d", len(themes), len(res.Themes))
			}
			for theme, want := range res.Themes {
				entry, ok := themes[theme].(map[string]any)
				if !ok {
					t.Fatalf("themes missing %q", theme)
				}
				if entry["state"] != want.State.String() {
					t.Errorf("%s state = %v, Compute = %q", theme, entry["state"], want.State.String())
				}
			}
			if got := ach["documented_count"]; got != int64(res.DocumentedCount) {
				t.Errorf("documented_count = %v (%T), Compute = %d", got, got, res.DocumentedCount)
			}
			rs, ok := built["restricted_substances_declared"].([]any)
			if !ok {
				t.Fatalf("built index has no restricted_substances_declared array")
			}
			if len(rs) != len(res.RestrictedSubstances) {
				t.Errorf("restricted count %d != Compute %d", len(rs), len(res.RestrictedSubstances))
			}
		})
	}
}

// 5.12 TestRealExampleAchievementStates asserts the section 4.3 expected-state table
// verbatim over the real examples: the six theme states, documented_count, the restricted
// array, and that conformance_level is unchanged on every record. Escalate any mismatch;
// never adjust a record to force a state.
func TestRealExampleAchievementStates(t *testing.T) {
	type row struct {
		emergency, energy, darkSky, materialHealth, embodiedCarbon, circularity string
		documentedCount                                                         int
		restricted                                                              []string
		conformance                                                             string
	}
	want := map[string]row{
		"cooper-atlite-auxswhsd.ulc":                               {"documented", "none", "none", "none", "none", "none", 1, []string{}, "standard"},
		"cooper-sure-lites-es61src.ulc":                            {"claimed", "none", "none", "none", "none", "none", 0, []string{}, "standard"},
		"cooper-sure-lites-lpx7sd.ulc":                             {"claimed", "claimed", "none", "none", "none", "none", 0, []string{}, "core"},
		"erco-quintessence-30416-023.ulc":                          {"none", "none", "none", "none", "none", "none", 0, []string{}, "standard"},
		"lumenpulse-lumenfacade-loi-12-rgb-30x60-ts0.ulc":          {"none", "none", "none", "none", "none", "none", 0, []string{"rohs"}, "standard"},
		"lumenpulse-lumenfacade-loi-12-rgbw30k-10x60-ts2-5.ulc":    {"none", "none", "none", "none", "none", "none", 0, []string{"rohs"}, "standard"},
		"selux-aya-pole-sr-ho-3000k.ulc":                           {"none", "none", "claimed", "claimed", "none", "none", 0, []string{"rohs"}, "standard"},
		"vode-nexa-suspended-807-so-3500k-90cri-hl-black-48in.ulc": {"none", "none", "none", "claimed", "none", "none", 0, []string{}, "core"},
	}
	records := exampleRecords(t)
	if len(records) != len(want) {
		t.Fatalf("example count %d != table rows %d", len(records), len(want))
	}
	stateOf := func(themes map[string]any, key string) string {
		e, _ := themes[key].(map[string]any)
		s, _ := e["state"].(string)
		return s
	}
	for name, w := range want {
		t.Run(name, func(t *testing.T) {
			record, ok := records[name]
			if !ok {
				t.Fatalf("example %q not found", name)
			}
			built := Build(record)
			ach := built["achievements"].(map[string]any)
			themes := ach["themes"].(map[string]any)
			checks := []struct{ theme, got, want string }{
				{"emergency", stateOf(themes, "emergency"), w.emergency},
				{"energy", stateOf(themes, "energy"), w.energy},
				{"dark_sky", stateOf(themes, "dark_sky"), w.darkSky},
				{"material_health", stateOf(themes, "material_health"), w.materialHealth},
				{"embodied_carbon", stateOf(themes, "embodied_carbon"), w.embodiedCarbon},
				{"circularity", stateOf(themes, "circularity"), w.circularity},
			}
			for _, c := range checks {
				if c.got != c.want {
					t.Errorf("%s: %s = %q, want %q", name, c.theme, c.got, c.want)
				}
			}
			if got := ach["documented_count"]; got != int64(w.documentedCount) {
				t.Errorf("%s: documented_count = %v, want %d", name, got, w.documentedCount)
			}
			rs, _ := built["restricted_substances_declared"].([]any)
			gotRS := make([]string, len(rs))
			for i, v := range rs {
				gotRS[i], _ = v.(string)
			}
			if strings.Join(gotRS, ",") != strings.Join(w.restricted, ",") {
				t.Errorf("%s: restricted_substances_declared = %v, want %v", name, gotRS, w.restricted)
			}
			// conformance_level must be unchanged from the record's stored index.
			if got := built["conformance_level"]; got != w.conformance {
				t.Errorf("%s: conformance_level = %v, want %q", name, got, w.conformance)
			}
			stored, _ := record["index"].(map[string]any)
			if storedCL, _ := stored["conformance_level"].(string); storedCL != w.conformance {
				t.Errorf("%s: stored conformance_level = %q, want %q (grade must not move)", name, storedCL, w.conformance)
			}
		})
	}
}

// 5.12 TestEmergencyThemeIsLedgerOnly is the D1 pin the corpus cannot supply: a normal
// fixture (panel_troffer) with NO emergency block and NO emergency_role, carrying only a
// ul_924 attestation, still earns the emergency achievement from the ledger alone. A
// reintroduced role or category gate on the emergency theme would fail this test.
func TestEmergencyThemeIsLedgerOnly(t *testing.T) {
	sha := strings.Repeat("a", 64)
	documented := map[string]any{
		"product_family": map[string]any{"primary_category": "panel_troffer"},
		"attestations": []any{
			map[string]any{
				"program":             "ul_924",
				"source_document_ref": map[string]any{"filename": "cert.pdf", "sha256": sha},
			},
		},
	}
	if got := achievements.Compute(documented).Themes["emergency"].State.String(); got != "documented" {
		t.Errorf("normal fixture, ul_924 + doc: emergency = %q, want documented (ledger-only, no role gate)", got)
	}

	claimed := map[string]any{
		"product_family": map[string]any{"primary_category": "panel_troffer"},
		"attestations":   []any{map[string]any{"program": "ul_924"}},
	}
	if got := achievements.Compute(claimed).Themes["emergency"].State.String(); got != "claimed" {
		t.Errorf("normal fixture, ul_924 no doc: emergency = %q, want claimed", got)
	}
}
