package completeness

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// updateGatingTable regenerates the frozen gating-row table instead of comparing.
// Run: go test ./internal/completeness -run TestScopeGatingTable -update-gating-table
var updateGatingTable = flag.Bool("update-gating-table", false, "rewrite testdata/gating-table.txt from the rubric")

// gatingTablePath is deliberately NOT under testdata/golden/, so the release gate
// that requires zero churn across the validate goldens stays an exact directory diff.
const gatingTablePath = "testdata/gating-table.txt"

// scopeExamples is every shipped example record, by filename.
func scopeExamples(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(repoRoot(t), "examples", "*.ulc"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no example records found")
	}
	sort.Strings(matches)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, filepath.Base(m))
	}
	return out
}

// scopeKey is the (tier, path) identity of a scope item.
type scopeKey struct {
	level Level
	path  string
}

// scopeKeys indexes a manifest by (tier, path).
func scopeKeys(items []ScopeItem) map[scopeKey]ScopeItem {
	out := make(map[scopeKey]ScopeItem, len(items))
	for _, it := range items {
		out[scopeKey{it.Level, it.Path}] = it
	}
	return out
}

// hasScope reports whether the manifest holds (lvl, path).
func hasScope(items []ScopeItem, lvl Level, path string) bool {
	_, ok := scopeKeys(items)[scopeKey{lvl, path}]
	return ok
}

// rolledUpBlocks is the sorted, de-duplicated union of the items' blocks, the
// same derivation the CLI publishes as the manifest's blocks array.
func rolledUpBlocks(items []ScopeItem) []string {
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

// tierCounts returns the per-tier item counts of a manifest.
func tierCounts(items []ScopeItem) (core, standard, full int) {
	for _, it := range items {
		switch it.Level {
		case LevelCore:
			core++
		case LevelStandard:
			standard++
		case LevelFull:
			full++
		}
	}
	return
}

// --- T1: scope semantics per class ---

// TestScopePerClass asserts the class-aware cut the manifest exists to expose:
// an exit sign is never held to photometric distribution, a generic luminaire is
// never held to the exit_sign dataset, and a dedicated emergency luminaire is
// excused efficacy. Assertions are on exact (tier, path) membership.
func TestScopePerClass(t *testing.T) {
	t.Run("generic luminaire", func(t *testing.T) {
		items := Scope(coreBase())
		for _, want := range []scopeKey{
			{LevelCore, "/photometry/distribution_type"},
			{LevelCore, "/electrical/driver_protocol"},
			{LevelCore, "/photometry/luminaire_efficacy_lm_per_w"},
		} {
			if !hasScope(items, want.level, want.path) {
				t.Errorf("generic luminaire scope is missing %s %s", want.level, want.path)
			}
		}
		for _, it := range items {
			if strings.HasPrefix(it.Path, "/exit_sign/") {
				t.Errorf("generic luminaire scope contains an exit-sign row: %s %s", it.Level, it.Path)
			}
		}
	})

	t.Run("internally illuminated sign", func(t *testing.T) {
		items := Scope(comboSignCore())
		for _, want := range []scopeKey{
			{LevelCore, "/exit_sign/illumination_mode"},
			{LevelCore, "/exit_sign/legend_color"},
			{LevelCore, "/emergency/power_source"},
			{LevelCore, "UL 924 listing"},
			{LevelStandard, "/exit_sign/legend_height"},
			{LevelStandard, "/exit_sign/face_count"},
			{LevelStandard, "/exit_sign/directional_indicator"},
			{LevelStandard, "/electrical/input_power_w"},
			{LevelFull, "test-report-backed sign-face luminance"},
		} {
			if !hasScope(items, want.level, want.path) {
				t.Errorf("sign scope is missing %s %s", want.level, want.path)
			}
		}
		for _, absent := range []scopeKey{
			{LevelCore, "/photometry/distribution_type"},
			{LevelCore, "/electrical/driver_protocol"},
			{LevelCore, "/colorimetry/nominal_cct_k"},
			{LevelCore, "/photometry/luminaire_efficacy_lm_per_w"},
		} {
			if hasScope(items, absent.level, absent.path) {
				t.Errorf("sign scope should not contain %s %s", absent.level, absent.path)
			}
		}
	})

	t.Run("UL 924 listing is region-gated", func(t *testing.T) {
		na := comboSignCore()
		if !hasScope(Scope(na), LevelCore, "UL 924 listing") {
			t.Error("an NA sign should carry the UL 924 listing requirement")
		}
		nonNA := comboSignCore()
		nonNA["product_family"].(map[string]any)["technical_region"] = "230v_50hz_europe"
		if hasScope(Scope(nonNA), LevelCore, "UL 924 listing") {
			t.Error("a non-NA sign should not carry the UL 924 listing requirement")
		}
	})

	t.Run("dedicated emergency luminaire", func(t *testing.T) {
		items := Scope(emgLuminaireCore())
		if hasScope(items, LevelCore, "/photometry/luminaire_efficacy_lm_per_w") {
			t.Error("a dedicated emergency luminaire should be excused luminaire efficacy")
		}
		if !hasScope(items, LevelCore, "/emergency/power_source") {
			t.Error("a dedicated emergency luminaire should carry /emergency/power_source at core")
		}
	})

	t.Run("externally illuminated sign full-tier partition", func(t *testing.T) {
		items := Scope(externalSignCore())
		if !hasScope(items, LevelFull, "test-report-backed face illuminance") {
			t.Error("an externally illuminated sign should carry the face-illuminance requirement at full")
		}
		if hasScope(items, LevelFull, "test-report-backed sign-face luminance") {
			t.Error("an externally illuminated sign should not carry the sign-face luminance requirement")
		}
	})

	t.Run("empty object", func(t *testing.T) {
		items := Scope(map[string]any{})
		core, standard, full := tierCounts(items)
		if len(items) != 35 || core != 19 || standard != 10 || full != 6 {
			t.Errorf("empty-object scope = %d items (%d/%d/%d), want 35 (19/10/6)", len(items), core, standard, full)
		}
		want := []string{
			"attestations", "configuration", "corrections_applied", "electrical",
			"instrumentation", "lumen_maintenance_luminaire", "lumen_maintenance_package",
			"operating_point", "photometry", "product_family", "test_conditions", "uncertainty",
		}
		if got := rolledUpBlocks(items); !equalStrings(got, want) {
			t.Errorf("empty-object blocks = %v, want %v", got, want)
		}
	})
}

// --- T2: the frozen public table ---

// TestScopeGatingTable freezes all 68 gating rows as (tier, kind, path,
// source_document, standard). The manifest publishes those three strings for
// every in-scope row unconditionally, where today they reach the public only for
// rows some record happens to miss, so any reword, any added or removed gating
// row, and any kind reclassification must fail here with a readable diff.
func TestScopeGatingTable(t *testing.T) {
	lines := []string{}
	for _, ru := range rubric {
		if !isGating(ru.level) {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%s\t%s",
			ru.level, scopeKindOf(ru.path), ru.path, ru.document, ru.standard))
	}
	sort.Strings(lines)
	got := strings.Join(lines, "\n") + "\n"

	if *updateGatingTable {
		if err := os.WriteFile(gatingTablePath, []byte(got), 0o644); err != nil {
			t.Fatalf("write %s: %v", gatingTablePath, err)
		}
		t.Logf("wrote %s (%d gating rows)", gatingTablePath, len(lines))
		return
	}

	want, err := os.ReadFile(gatingTablePath)
	if err != nil {
		t.Fatalf("read %s (regenerate with -update-gating-table): %v", gatingTablePath, err)
	}
	if got != string(want) {
		t.Errorf("the frozen gating table drifted.\n--- want (%s) ---\n%s\n--- got ---\n%s", gatingTablePath, want, got)
	}
}

// TestScopeKindPartition pins the kind partition over the gating rows, asserts
// both contribution tables are total over their kinds, and asserts every block
// the manifest can name is a real top-level property of ulc.schema.json.
func TestScopeKindPartition(t *testing.T) {
	ulc := loadJSONSchema(t, "ulc.schema.json")
	props, ok := mapOf(ulc["properties"])
	if !ok {
		t.Fatal("ulc.schema.json has no properties object")
	}

	counts := map[ScopeKind]int{}
	for _, ru := range rubric {
		if !isGating(ru.level) {
			continue
		}
		kind := scopeKindOf(ru.path)
		counts[kind]++

		// Totality: every choice and requirement row must be mapped, by construction.
		switch kind {
		case ScopeKindChoice:
			if _, mapped := choiceBlocks[ru.path]; !mapped {
				t.Errorf("choice row %q has no entry in choiceBlocks", ru.path)
			}
		case ScopeKindRequirement:
			if _, mapped := requirementBlocks[ru.path]; !mapped {
				t.Errorf("requirement row %q has no entry in requirementBlocks", ru.path)
			}
		}

		for _, b := range scopeBlocksFor(kind, ru.path) {
			if _, exists := props[b]; !exists {
				t.Errorf("row %q contributes block %q, which is not a top-level property of ulc.schema.json", ru.path, b)
			}
		}
	}

	if counts[ScopeKindPointer] != 59 || counts[ScopeKindChoice] != 2 || counts[ScopeKindRequirement] != 7 {
		t.Errorf("gating kind partition = %d pointer / %d choice / %d requirement, want 59/2/7",
			counts[ScopeKindPointer], counts[ScopeKindChoice], counts[ScopeKindRequirement])
	}
	if total := counts[ScopeKindPointer] + counts[ScopeKindChoice] + counts[ScopeKindRequirement]; total != 68 {
		t.Errorf("gating row count = %d, want 68", total)
	}

	// The tables must not carry entries no gating row claims.
	for path := range choiceBlocks {
		if _, found := ruleByGatingPath(path); !found {
			t.Errorf("choiceBlocks carries %q, which is not a gating rubric row", path)
		}
	}
	for path := range requirementBlocks {
		if _, found := ruleByGatingPath(path); !found {
			t.Errorf("requirementBlocks carries %q, which is not a gating rubric row", path)
		}
	}
}

// ruleByGatingPath finds a gating rubric row by its path string.
func ruleByGatingPath(path string) (rule, bool) {
	for _, ru := range rubric {
		if isGating(ru.level) && ru.path == path {
			return ru, true
		}
	}
	return rule{}, false
}

// --- T3: scope is a superset of the gaps ---

// TestScopeSupersetOfGaps asserts the invariant that makes the manifest usable
// alongside validate: every reported gap is a member of the scope, and the scope
// never names a (tier, path) that is not a gating rubric row. This is the single
// statement of the no-leak invariant.
func TestScopeSupersetOfGaps(t *testing.T) {
	gating := map[scopeKey]bool{}
	for _, ru := range rubric {
		if isGating(ru.level) {
			gating[scopeKey{ru.level, ru.path}] = true
		}
	}

	for _, name := range scopeExamples(t) {
		rec := exampleRecord(t, name)
		items := Scope(rec)
		index := scopeKeys(items)

		for _, gap := range Compute(rec).TierRoadmap {
			if _, ok := index[scopeKey{gap.NextLevel, gap.Path}]; !ok {
				t.Errorf("%s: gap %s %s is not in the scope manifest", name, gap.NextLevel, gap.Path)
			}
		}
		for _, it := range items {
			if !gating[scopeKey{it.Level, it.Path}] {
				t.Errorf("%s: scope names %s %s, which is not a gating rubric row", name, it.Level, it.Path)
			}
		}
	}
}

// --- T4: the universal floor ---

// universalGatingRows is every gating row with no applicability predicate. Each
// one is in scope for every record, whatever its class.
func universalGatingRows() []scopeKey {
	out := []scopeKey{}
	for _, ru := range rubric {
		if isGating(ru.level) && ru.applicable == nil {
			out = append(out, scopeKey{ru.level, ru.path})
		}
	}
	return out
}

// TestScopeUniversalFloor asserts that every record's manifest carries the 13
// universal gating rows and names both product_family and attestations, the two
// blocks no record can reach core without.
func TestScopeUniversalFloor(t *testing.T) {
	universal := universalGatingRows()
	if len(universal) != 13 {
		t.Fatalf("universal gating rows = %d, want 13", len(universal))
	}

	records := map[string]map[string]any{"{} (empty object)": {}}
	for _, name := range scopeExamples(t) {
		records[name] = exampleRecord(t, name)
	}

	for name, rec := range records {
		items := Scope(rec)
		for _, u := range universal {
			if !hasScope(items, u.level, u.path) {
				t.Errorf("%s: manifest is missing the universal row %s %s", name, u.level, u.path)
			}
		}
		blocks := rolledUpBlocks(items)
		for _, want := range []string{"attestations", "product_family"} {
			if !containsString(blocks, want) {
				t.Errorf("%s: blocks %v does not contain %q", name, blocks, want)
			}
		}
	}
}

// --- T6: blocks derivation, asserted per item ---

// TestScopeBlocksDerivation asserts the two contribution tables entry by entry on
// the INDIVIDUAL items, not on the rolled-up union. The union cannot discriminate
// them: the method-backed lumen-maintenance requirement row contributes the same
// two blocks as the lumen-maintenance choice row, and electrical is independently
// supplied by six pointer rows, so deleting the whole choice table leaves every
// rolled-up blocks list byte-identical.
func TestScopeBlocksDerivation(t *testing.T) {
	// A record broad enough to hold every non-pointer gating row in scope needs
	// both classes, which no single record can be, so the two are checked apart.
	generic := Scope(coreBase())
	sign := Scope(comboSignCore())
	external := Scope(externalSignCore())

	type expect struct {
		items  []ScopeItem
		level  Level
		path   string
		blocks []string
	}
	for _, e := range []expect{
		// choice rows
		{generic, LevelCore, "/electrical/input_voltage_v (or input_voltage_class)", []string{"electrical"}},
		{generic, LevelStandard, "/lumen_maintenance_luminaire (or /lumen_maintenance_package)", []string{"lumen_maintenance_luminaire", "lumen_maintenance_package"}},
		// requirement rows
		{generic, LevelCore, "safety listing (UL/cUL/ETL/CSA for NA; CE/ENEC/IEC 60598 otherwise)", []string{"attestations", "product_family"}},
		{sign, LevelCore, "UL 924 listing", []string{"attestations", "product_family"}},
		{generic, LevelStandard, "LM-79 attestation", []string{"attestations", "product_family"}},
		{generic, LevelFull, "instrumentation depth (goniometer/lab)", []string{"instrumentation"}},
		{generic, LevelFull, "method-backed lumen maintenance (TM-21 hours or TM-28)", []string{"lumen_maintenance_luminaire", "lumen_maintenance_package"}},
		{sign, LevelFull, "test-report-backed sign-face luminance", []string{"exit_sign"}},
		{external, LevelFull, "test-report-backed face illuminance", []string{"exit_sign"}},
	} {
		it, ok := scopeKeys(e.items)[scopeKey{e.level, e.path}]
		if !ok {
			t.Errorf("expected %s %s to be in scope for its fixture", e.level, e.path)
			continue
		}
		if !equalStrings(it.Blocks, e.blocks) {
			t.Errorf("%s %s: blocks = %v, want %v", e.level, e.path, it.Blocks, e.blocks)
		}
	}

	// A pointer row contributes its first path segment and nothing else.
	it, ok := scopeKeys(generic)[scopeKey{LevelCore, "/product_family/manufacturer/slug"}]
	if !ok {
		t.Fatal("/product_family/manufacturer/slug should be universally in scope")
	}
	if !equalStrings(it.Blocks, []string{"product_family"}) {
		t.Errorf("/product_family/manufacturer/slug: blocks = %v, want [product_family]", it.Blocks)
	}

	// The exit-sign example's rolled-up blocks, verbatim.
	signExample := Scope(exampleRecord(t, "cooper-sure-lites-lpx7sd.ulc"))
	wantSignBlocks := []string{"attestations", "electrical", "emergency", "exit_sign", "product_family"}
	if got := rolledUpBlocks(signExample); !equalStrings(got, wantSignBlocks) {
		t.Errorf("exit-sign example blocks = %v, want %v", got, wantSignBlocks)
	}

	// input_voltage_class is a leaf inside electrical, never a block. A naive
	// parenthesis-split of the choice row would publish it as one.
	records := map[string]map[string]any{"{} (empty object)": {}, "coreBase": coreBase()}
	for _, name := range scopeExamples(t) {
		records[name] = exampleRecord(t, name)
	}
	for name, rec := range records {
		blocks := rolledUpBlocks(Scope(rec))
		if containsString(blocks, "input_voltage_class") {
			t.Errorf("%s: blocks %v contains input_voltage_class", name, blocks)
		}
		if !containsString(blocks, "attestations") {
			t.Errorf("%s: blocks %v does not contain attestations", name, blocks)
		}
	}
}

// --- small helpers ---

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
