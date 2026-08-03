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

// genericBlocks is the blocks list of a record that scopes as a generic
// luminaire with no colorimetry and no outdoor classification: the empty object,
// and the RGB lumenpulse example. Shared so a new block is edited in one place.
var genericBlocks = []string{
	"attestations", "configuration", "corrections_applied", "electrical",
	"instrumentation", "lumen_maintenance_luminaire", "lumen_maintenance_package",
	"operating_point", "photometry", "product_family", "test_conditions", "uncertainty",
}

// universalGatingRowCount is how many gating rows carry no applicability
// predicate, and so appear in every record's manifest.
const universalGatingRowCount = 13

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

// rolledUpBlocks is the SHIPPED derivation, not a test-local copy of it: the CLI
// publishes the manifest's blocks array from this exact function, so every
// assertion below constrains production code.
func rolledUpBlocks(items []ScopeItem) []string { return RollupBlocks(items) }

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
		// Item counts and identities are pinned by TestScopeEmptyObjectIdentity,
		// which implies them exactly; blocks are the independent witness here.
		items := Scope(map[string]any{})
		if got := rolledUpBlocks(items); !equalStrings(got, genericBlocks) {
			t.Errorf("empty-object blocks = %v, want %v", got, genericBlocks)
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
		t.Errorf("the frozen gating table drifted. Every one of these rows is published "+
			"verbatim by the scope manifest and by conformance/gap findings, so a diff here "+
			"is a BREAKING CHANGE to a public contract, not a refactor. Regenerating with "+
			"-update-gating-table is only correct once that break is intended and recorded.\n"+
			"--- want (%s) ---\n%s\n--- got ---\n%s", gatingTablePath, want, got)
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

	// Level totality. isGating is an allowlist, so a future Level constant would
	// otherwise be dropped from every manifest with the whole suite still green:
	// this test, the frozen table and the floor test all filter through the same
	// predicate, so none of them can notice on its own. Pinning the three-way
	// split against len(rubric) forces an explicit decision instead.
	var gating, enrichment, observation int
	for _, ru := range rubric {
		switch {
		case isGating(ru.level):
			gating++
		case ru.level == LevelEnrichment:
			enrichment++
		case ru.level == LevelObservation:
			observation++
		default:
			t.Errorf("rubric row %q carries level %s, which is neither gating nor a known sentinel; decide whether Scope must publish it", ru.path, ru.level)
		}
	}
	if gating+enrichment+observation != len(rubric) {
		t.Errorf("level split = %d gating + %d enrichment + %d observation, want %d total", gating, enrichment, observation, len(rubric))
	}
	if gating != 68 || enrichment != 70 || observation != 2 {
		t.Errorf("level split = %d/%d/%d, want 68 gating / 70 enrichment / 2 observation", gating, enrichment, observation)
	}

	// Every tier token the manifest can emit must be one of the three documented
	// values. Level.String() falls back to "incomplete" for an unmapped constant,
	// which would publish a token that is absent from the contract but looks
	// known, defeating the ignore-unrecognized-values rule consumers were given.
	for _, ru := range rubric {
		if !isGating(ru.level) {
			continue
		}
		switch ru.level.String() {
		case "core", "standard", "full":
		default:
			t.Errorf("gating row %q emits tier token %q, which is not in the documented set", ru.path, ru.level.String())
		}
	}

	// (level, path) uniqueness across the gating band is what makes Scope's sort a
	// total order and the goldens byte-stable.
	seenKey := map[scopeKey]string{}
	for _, ru := range rubric {
		if !isGating(ru.level) {
			continue
		}
		k := scopeKey{ru.level, ru.path}
		if prev, dup := seenKey[k]; dup {
			t.Errorf("duplicate gating (level, path) %s %s: item order between this row and %q is then unspecified", ru.level, ru.path, prev)
		}
		seenKey[k] = ru.path
	}

	if counts[ScopeKindPointer] != 59 || counts[ScopeKindChoice] != 2 || counts[ScopeKindRequirement] != 7 {
		t.Errorf("gating kind partition = %d pointer / %d choice / %d requirement, want 59/2/7",
			counts[ScopeKindPointer], counts[ScopeKindChoice], counts[ScopeKindRequirement])
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

		gaps := map[scopeKey]bool{}
		for _, gap := range Compute(rec).TierRoadmap {
			gaps[scopeKey{gap.NextLevel, gap.Path}] = true
			if _, ok := index[scopeKey{gap.NextLevel, gap.Path}]; !ok {
				t.Errorf("%s: gap %s %s is not in the scope manifest", name, gap.NextLevel, gap.Path)
			}
		}
		for _, it := range items {
			if !gating[scopeKey{it.Level, it.Path}] {
				t.Errorf("%s: scope names %s %s, which is not a gating rubric row", name, it.Level, it.Path)
			}
		}

		// The other direction, which is the half the shipped contract sells:
		// "scope minus the gaps validate reports gives the satisfied set". That
		// only holds if every in-scope item NOT reported as a gap is genuinely
		// present, which in turn depends on AchievedLevel's ladder agreeing with
		// missingAt at every tier at or below the achieved level (Compute skips
		// whole tiers below the grade). Without this, a consumer could read an
		// absent field as satisfied and nothing would object.
		for _, it := range items {
			if gaps[scopeKey{it.Level, it.Path}] {
				continue
			}
			ru, found := ruleByLevelPath(it.Level, it.Path)
			if !found {
				t.Errorf("%s: scope item %s %s has no rubric row", name, it.Level, it.Path)
				continue
			}
			if !ru.present(rec) {
				t.Errorf("%s: %s %s is in scope and is NOT reported as a gap, so a consumer reads it as satisfied, but its present-closure is false", name, it.Level, it.Path)
			}
		}
	}
}

// TestScopeCorpusShape is the second witness the CLI goldens lack. The goldens
// are regenerated by their own -update flag, so a predicate regression that shows
// up only on a real example (selux losing outdoor_classification, erco losing
// colorimetry) would be baked in silently. These numbers were derived
// independently of any generated artifact and are asserted against Scope
// directly, never through a golden file.
func TestScopeCorpusShape(t *testing.T) {
	base := genericBlocks
	withColor := func(extra ...string) []string {
		out := append([]string{}, base...)
		out = append(out, extra...)
		sort.Strings(out)
		return out
	}
	sign := []string{"attestations", "electrical", "emergency", "exit_sign", "product_family"}

	cases := []struct {
		record                 string
		items, core, std, full int
		blocks                 []string
	}{
		{"cooper-atlite-auxswhsd.ulc", 25, 15, 9, 1, sign},
		{"cooper-sure-lites-es61src.ulc", 22, 15, 6, 1, sign},
		{"cooper-sure-lites-lpx7sd.ulc", 25, 15, 9, 1, sign},
		{"erco-quintessence-30416-023.ulc", 44, 21, 15, 8, withColor("colorimetry")},
		{"lumenpulse-lumenfacade-loi-12-rgb-30x60-ts0.ulc", 37, 19, 12, 6, base},
		{"lumenpulse-lumenfacade-loi-12-rgbw30k-10x60-ts2-5.ulc", 38, 20, 12, 6, withColor("colorimetry")},
		{"selux-aya-pole-sr-ho-3000k.ulc", 47, 21, 18, 8, withColor("colorimetry", "outdoor_classification")},
		{"vode-nexa-suspended-807-so-3500k-90cri-hl-black-48in.ulc", 45, 21, 16, 8, withColor("colorimetry")},
	}
	if got := len(cases); got != len(scopeExamples(t)) {
		t.Fatalf("this table covers %d records but examples/ holds %d; add the new record here too", got, len(scopeExamples(t)))
	}
	for _, c := range cases {
		c := c
		t.Run(c.record, func(t *testing.T) {
			items := Scope(exampleRecord(t, c.record))
			core, std, full := tierCounts(items)
			if len(items) != c.items || core != c.core || std != c.std || full != c.full {
				t.Errorf("scope = %d items (%d/%d/%d), want %d (%d/%d/%d)",
					len(items), core, std, full, c.items, c.core, c.std, c.full)
			}
			if got := rolledUpBlocks(items); !equalStrings(got, c.blocks) {
				t.Errorf("blocks = %v, want %v", got, c.blocks)
			}
		})
	}
}

// TestScopeEmptyObjectIdentity pins the identity, not just the count, of the
// broadest manifest. The empty object scopes as a generic luminaire and is the
// one record whose expectations were derived by hand, so a within-tier predicate
// swap that preserves the 19/10/6 counts has to fail somewhere: here.
func TestScopeEmptyObjectIdentity(t *testing.T) {
	// No colorimetry row appears: with no white point declared, hasWhitePoint and
	// isWhiteLightPrimary are both false, so nominal_cct_k and cri_ra are out of
	// scope. That is the class-aware cut working on the degenerate record.
	want := []string{
		"core|/configuration/tested_axes/color_tunability",
		"core|/electrical/driver_protocol",
		"core|/electrical/input_power_w",
		"core|/electrical/input_voltage_v (or input_voltage_class)",
		"core|/photometry/distribution_type",
		"core|/photometry/luminaire_efficacy_lm_per_w",
		"core|/photometry/total_luminous_flux_lm",
		"core|/product_family/catalog_model",
		"core|/product_family/cutsheet",
		"core|/product_family/environment_rating",
		"core|/product_family/indoor_outdoor",
		"core|/product_family/manufacturer/display_name",
		"core|/product_family/manufacturer/slug",
		"core|/product_family/mounting_types",
		"core|/product_family/primary_category",
		"core|/product_family/secondary_function",
		"core|/product_family/shape",
		"core|/product_family/technical_region",
		"core|safety listing (UL/cUL/ETL/CSA for NA; CE/ENEC/IEC 60598 otherwise)",
		"standard|/electrical/control_gear_type",
		"standard|/instrumentation/measurement_regime",
		"standard|/lumen_maintenance_luminaire (or /lumen_maintenance_package)",
		"standard|/photometry/maximum_intensity_cd",
		"standard|/photometry/photometric_coordinate_system",
		"standard|/photometry/symmetry_type",
		"standard|/product_family/shared_mechanical/housing_material",
		"standard|/product_family/shared_mechanical/lens_material",
		"standard|/test_conditions/photometry_basis",
		"standard|LM-79 attestation",
		"full|/corrections_applied",
		"full|/operating_point",
		"full|/photometry/zonal_lumens",
		"full|/uncertainty",
		"full|instrumentation depth (goniometer/lab)",
		"full|method-backed lumen maintenance (TM-21 hours or TM-28)",
	}
	got := []string{}
	for _, it := range Scope(map[string]any{}) {
		got = append(got, it.Level.String()+"|"+it.Path)
	}
	if !equalStrings(got, want) {
		t.Errorf("empty-object manifest identity drifted.\n got: %v\nwant: %v", got, want)
	}
}

// TestScopePathUniqueWithinManifest backs the join contract the docs publish.
// Identity is (tier, path) because a few paths are graded at two tiers, but the
// shipped guidance also says a per-record join on `path` alone is sound. That is
// only true if no single manifest ever holds one path twice, which is a property
// of the applicability predicates (the two-tier paths are class-exclusive) and
// not of the rubric, so it needs its own assertion rather than the (level, path)
// uniqueness check, which would stay green the day two such rows became
// co-applicable to one record.
func TestScopePathUniqueWithinManifest(t *testing.T) {
	records := map[string]map[string]any{
		"{} (empty object)":     {},
		"coreBase":              coreBase(),
		"comboSignCore":         comboSignCore(),
		"externalSignCore":      externalSignCore(),
		"photoSignCore":         photoSignCore(),
		"tritiumSignCore":       tritiumSignCore(),
		"emgLuminaireCore":      emgLuminaireCore(),
		"emgLuminaireFull":      emgLuminaireFull(),
		"comboSignFull":         comboSignFull(),
		"externalComboSignFull": externalComboSignFull(),
	}
	for _, name := range scopeExamples(t) {
		records[name] = exampleRecord(t, name)
	}
	for name, rec := range records {
		seen := map[string]Level{}
		for _, it := range Scope(rec) {
			if prev, dup := seen[it.Path]; dup {
				t.Errorf("%s: path %q appears at both %s and %s in one manifest, so a per-record join on path alone is ambiguous", name, it.Path, prev, it.Level)
			}
			seen[it.Path] = it.Level
		}
	}
}

// TestRollupBlocksEmpty pins the non-nil guarantee that makes the manifest's
// blocks array marshal as [] rather than null. Scope always yields items today,
// so this case is unreachable through the CLI and would otherwise go untested.
func TestRollupBlocksEmpty(t *testing.T) {
	for _, in := range [][]ScopeItem{nil, {}} {
		got := RollupBlocks(in)
		if got == nil {
			t.Errorf("RollupBlocks(%v) returned nil; it must be non-nil so callers emit [] rather than null", in)
		}
		if len(got) != 0 {
			t.Errorf("RollupBlocks(%v) = %v, want empty", in, got)
		}
	}
}

// TestScopeBlocksNotAliased pins the immutability guarantee scopeBlocksFor
// documents. Scope is exported and ScopeItem.Blocks is an exported field, so
// handing back the package-level table's own slice would let one consumer corrupt
// the rubric tables process-wide for every later call.
func TestScopeBlocksNotAliased(t *testing.T) {
	const path = "safety listing (UL/cUL/ETL/CSA for NA; CE/ENEC/IEC 60598 otherwise)"
	first, ok := scopeKeys(Scope(coreBase()))[scopeKey{LevelCore, path}]
	if !ok {
		t.Fatal("the universal safety-listing row should always be in scope")
	}
	if len(first.Blocks) == 0 {
		t.Fatal("the safety-listing row should contribute blocks")
	}
	first.Blocks[0] = "CORRUPTED"

	second, ok := scopeKeys(Scope(coreBase()))[scopeKey{LevelCore, path}]
	if !ok {
		t.Fatal("the universal safety-listing row should always be in scope")
	}
	if !equalStrings(second.Blocks, []string{"attestations", "product_family"}) {
		t.Errorf("mutating a returned Blocks slice corrupted the shared table: second call = %v", second.Blocks)
	}
}

// TestScopeHostileInput pins the totality claim Scope's doc comment makes. Scope
// runs every gating applicability predicate against attacker-controlled JSON
// before any schema validation, so a wrong type at any level must yield a
// manifest rather than a panic.
func TestScopeHostileInput(t *testing.T) {
	cases := map[string]map[string]any{
		"empty":              {},
		"nulls":              {"product_family": nil, "photometry": nil, "exit_sign": nil, "emergency": nil},
		"arrays where maps":  {"product_family": []any{1, 2}, "colorimetry": []any{}, "configuration": []any{nil}},
		"scalars where maps": {"product_family": 7, "electrical": "x", "exit_sign": true},
		"wrong leaf types": {"product_family": map[string]any{
			"primary_category": []any{"exit_sign"}, "technical_region": 3, "indoor_outdoor": nil,
		}},
		"nested wrong types": {"product_family": map[string]any{
			"shared_mechanical": "not-a-map", "manufacturer": []any{},
		}, "exit_sign": map[string]any{"illumination_mode": map[string]any{}}},
		"junk class tokens": {"product_family": map[string]any{
			"primary_category": "\x00� not a category", "technical_region": "",
		}},
	}
	for name, rec := range cases {
		name, rec := name, rec
		t.Run(name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Scope panicked on hostile input: %v", r)
				}
			}()
			items := Scope(rec)
			if len(items) < universalGatingRowCount {
				t.Errorf("hostile input yielded %d items; every record carries at least the %d universal rows", len(items), universalGatingRowCount)
			}
			if !contains(RollupBlocks(items), "attestations") {
				t.Error("hostile input dropped the universal attestations block")
			}
		})
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
	if len(universal) != universalGatingRowCount {
		t.Fatalf("universal gating rows = %d, want %d", len(universal), universalGatingRowCount)
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
			if !contains(blocks, want) {
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
		if contains(blocks, "input_voltage_class") {
			t.Errorf("%s: blocks %v contains input_voltage_class", name, blocks)
		}
		if !contains(blocks, "attestations") {
			t.Errorf("%s: blocks %v does not contain attestations", name, blocks)
		}
	}
}

// --- small helpers ---

// equalStrings compares two string slices element-wise. (containsString is not
// defined here: the package already carries `contains` in emergency_test.go.)
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

// TestNonPointerBlocksAreNecessary is the DRIFT GUARD for the two contribution
// tables, and the reason the tables can be hand-maintained safely.
//
// choiceBlocks and requirementBlocks republish, as public contract, which record
// blocks each non-pointer row's present-closure reads. Asserting the tables
// against a hand-written list (TestScopeBlocksDerivation) pins the published
// values but proves nothing about the closures, so a closure that later starts
// reading an additional block would leave the manifest quietly under-reporting
// it. That is exactly the defect this feature's first review round found in an
// earlier draft, where every manifest omitted `attestations`.
//
// This test closes it behaviorally instead of by inspection: take a record whose
// closure is satisfied, delete every block the table declares, and require the
// closure to go false. A closure reading an undeclared block would stay
// satisfied through it and fail here.
func TestNonPointerBlocksAreNecessary(t *testing.T) {
	cases := []struct {
		level  Level
		path   string
		record map[string]any // must satisfy the row's present-closure
	}{
		{LevelCore, "/electrical/input_voltage_v (or input_voltage_class)", coreBase()},
		{LevelCore, "safety listing (UL/cUL/ETL/CSA for NA; CE/ENEC/IEC 60598 otherwise)", coreBase()},
		{LevelCore, "UL 924 listing", comboSignCore()},
		{LevelStandard, "LM-79 attestation", standardBase()},
		{LevelStandard, "/lumen_maintenance_luminaire (or /lumen_maintenance_package)", standardBase()},
		{LevelFull, "instrumentation depth (goniometer/lab)", fullBase()},
		{LevelFull, "method-backed lumen maintenance (TM-21 hours or TM-28)", fullBase()},
		{LevelFull, "test-report-backed sign-face luminance", comboSignFull()},
		{LevelFull, "test-report-backed face illuminance", externalComboSignFull()},
	}

	// Every non-pointer gating row must appear above, so a new one cannot be
	// added without also being guarded.
	covered := map[scopeKey]bool{}
	for _, c := range cases {
		covered[scopeKey{c.level, c.path}] = true
	}
	for _, ru := range rubric {
		if !isGating(ru.level) || scopeKindOf(ru.path) == ScopeKindPointer {
			continue
		}
		if !covered[scopeKey{ru.level, ru.path}] {
			t.Errorf("non-pointer gating row %s %q has no necessity case; add one so its declared blocks stay guarded", ru.level, ru.path)
		}
	}

	for _, c := range cases {
		c := c
		t.Run(c.path, func(t *testing.T) {
			ru, found := ruleByLevelPath(c.level, c.path)
			if !found {
				t.Fatalf("no rubric row for %s %q", c.level, c.path)
			}
			if !ru.present(c.record) {
				t.Fatalf("fixture does not satisfy the closure, so the deletion below would prove nothing")
			}
			declared := scopeBlocksFor(scopeKindOf(c.path), c.path)
			if len(declared) == 0 {
				t.Fatalf("row declares no blocks")
			}
			for _, b := range declared {
				delete(c.record, b)
			}
			if ru.present(c.record) {
				t.Errorf("after deleting the declared blocks %v the closure is STILL satisfied, so it reads a block the table does not declare and the manifest under-reports it", declared)
			}
		})
	}
}
