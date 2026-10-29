package achievements

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// namedThemeSets is the seven theme sets, keyed for reporting.
func namedThemeSets() map[string]map[string]bool {
	return map[string]map[string]bool{
		ThemeEmbodiedCarbon:  embodiedCarbonPrograms,
		ThemeCircularity:     circularityPrograms,
		ThemeMaterialHealth:  materialHealthPrograms,
		ThemeEnergy:          energyPrograms,
		ThemeDarkSky:         darkSkyPrograms,
		ThemeEmergency:       emergencyPrograms,
		ThemeDomesticContent: domesticContentPrograms,
	}
}

// 5.10 Every token in every named set is a real AttestationProgram enum member.
func TestThemeSetsAreRealEnumMembers(t *testing.T) {
	enum := loadTaxonomyEnum(t, "AttestationProgram")
	sets := map[string]map[string]bool{
		"restricted": restrictedSubstancePrograms,
		"unthemed":   unthemedPrograms,
	}
	for name, set := range namedThemeSets() {
		sets[name] = set
	}
	for name, set := range sets {
		for tok := range set {
			if !enum[tok] {
				t.Errorf("set %q contains %q, which is not an AttestationProgram enum member", name, tok)
			}
		}
	}
}

// 5.10 The declaration-to-program-token map keys are real SustainabilityDeclarationType
// members, the values are real AttestationProgram members, and manufacturer_recycle_program
// is a real declaration type that is deliberately NOT routed through the token map.
func TestDeclarationMapKeysAreRealDeclarationTypes(t *testing.T) {
	decl := loadTaxonomyEnum(t, "SustainabilityDeclarationType")
	prog := loadTaxonomyEnum(t, "AttestationProgram")
	for k, v := range declarationProgramTokens {
		if !decl[k] {
			t.Errorf("declarationProgramTokens key %q is not a SustainabilityDeclarationType member", k)
		}
		if !prog[v] {
			t.Errorf("declarationProgramTokens value %q is not an AttestationProgram member", v)
		}
	}
	if !decl["manufacturer_recycle_program"] {
		t.Error("manufacturer_recycle_program is expected to be a SustainabilityDeclarationType member")
	}
	if _, routed := declarationProgramTokens["manufacturer_recycle_program"]; routed {
		t.Error("manufacturer_recycle_program must NOT be routed through the declaration-to-token map (it has no AttestationProgram token)")
	}
}

// 5.10 No token is in two themes except the deliberate cradle_to_cradle dual-route, and
// no token is in both a theme and the restricted set.
func TestThemeSetsDisjointExceptDualRoute(t *testing.T) {
	seen := map[string][]string{}
	for name, set := range namedThemeSets() {
		for tok := range set {
			seen[tok] = append(seen[tok], name)
		}
	}
	for tok, themes := range seen {
		if len(themes) > 1 && tok != "cradle_to_cradle" {
			t.Errorf("token %q appears in multiple themes %v (only cradle_to_cradle may dual-route)", tok, themes)
		}
	}
	for name, set := range namedThemeSets() {
		for tok := range set {
			if restrictedSubstancePrograms[tok] {
				t.Errorf("token %q is in theme %q AND the restricted set", tok, name)
			}
			if unthemedPrograms[tok] {
				t.Errorf("token %q is in theme %q AND the unthemed residue set", tok, name)
			}
		}
	}
	for tok := range restrictedSubstancePrograms {
		if unthemedPrograms[tok] {
			t.Errorf("token %q is in both the restricted and unthemed sets", tok)
		}
	}
}

// 5.10 EXHAUSTIVENESS: the union of the seven theme sets, the restricted set, and the
// unthemed residue set equals the full AttestationProgram enum EXACTLY. cradle_to_cradle
// is the only token covered twice (its dual-route). A new enum token then fails this test
// until it is consciously triaged.
func TestEnumPartitionExhaustive(t *testing.T) {
	enum := loadTaxonomyEnum(t, "AttestationProgram")
	covered := map[string]int{}
	add := func(set map[string]bool) {
		for tok := range set {
			covered[tok]++
		}
	}
	for _, set := range namedThemeSets() {
		add(set)
	}
	add(restrictedSubstancePrograms)
	add(unthemedPrograms)

	for tok := range enum {
		want := 1
		if tok == "cradle_to_cradle" {
			want = 2
		}
		if covered[tok] != want {
			t.Errorf("token %q covered %d time(s), want %d", tok, covered[tok], want)
		}
	}
	for tok := range covered {
		if !enum[tok] {
			t.Errorf("covered token %q is not in the AttestationProgram enum", tok)
		}
	}
	union := map[string]bool{}
	for tok := range covered {
		union[tok] = true
	}
	if len(union) != len(enum) {
		t.Errorf("the partition covers %d distinct tokens; the enum has %d", len(union), len(enum))
	}
}

// T4 WIRING GUARD: themeOrder, themeSets, and the test-only namedThemeSets() must expose the
// identical key set. TestEnumPartitionExhaustive iterates namedThemeSets(), so a theme wired
// into themeOrder/themeSets but forgotten in namedThemeSets() (or the reverse) would leave the
// partition guard green while Compute badges that theme's tokens all-none. Pinning the three
// key sets together makes a half-wired theme fail loudly here instead of shipping silently.
func TestThemeSetKeysIdentical(t *testing.T) {
	orderKeys := map[string]bool{}
	for _, k := range themeOrder {
		if orderKeys[k] {
			t.Errorf("duplicate key %q in themeOrder", k)
		}
		orderKeys[k] = true
	}
	mapKeys := func(m map[string]map[string]bool) map[string]bool {
		out := map[string]bool{}
		for k := range m {
			out[k] = true
		}
		return out
	}
	setKeys, namedKeys := mapKeys(themeSets), mapKeys(namedThemeSets())

	// equalKeys asserts two key sets are identical, checking both directions so a key
	// present in either set but missing from the other fails loudly.
	equalKeys := func(a, b map[string]bool, aName, bName string) {
		for k := range a {
			if !b[k] {
				t.Errorf("%s has theme %q but %s does not", aName, k, bName)
			}
		}
		for k := range b {
			if !a[k] {
				t.Errorf("%s has theme %q but %s does not", bName, k, aName)
			}
		}
	}
	// themeOrder is the pivot: identity with both maps forces all three key sets identical.
	equalKeys(orderKeys, setKeys, "themeOrder", "themeSets")
	equalKeys(orderKeys, namedKeys, "themeOrder", "namedThemeSets")
}

// The glossary and its theme map are a second public spelling of the enum and sets.
func TestGlossaryMatchesProgramEnum(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), "docs", "compliance-attestation.md"))
	if err != nil {
		t.Fatal(err)
	}
	sections := strings.SplitN(string(data), "## Achievement themes", 2)
	if len(sections) != 2 {
		t.Fatal("missing achievement themes heading")
	}
	enum := loadTaxonomyEnum(t, "AttestationProgram")
	counts := map[string]int{}
	token := regexp.MustCompile("`([a-z][a-z0-9_]*)`")
	for _, line := range strings.Split(sections[0], "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(line, "|")
		match := token.FindStringSubmatch(cells[1])
		if len(match) != 2 {
			t.Fatalf("invalid glossary row %q", line)
		}
		counts[match[1]]++
		if !enum[match[1]] {
			t.Errorf("glossary has non-token %q", match[1])
		}
	}
	for name := range enum {
		if counts[name] != 1 {
			t.Errorf("%s has %d glossary rows", name, counts[name])
		}
	}
	count := regexp.MustCompile(`The ([0-9]+) tokens`).FindStringSubmatch(sections[0])
	if len(count) != 2 || count[1] != strconv.Itoa(len(enum)) {
		t.Errorf("preamble count %v, want %d", count, len(enum))
	}
	seen := map[string]int{}
	for _, line := range strings.Split(sections[1], "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cells := strings.Split(line, "|")
		key := token.FindStringSubmatch(cells[1])
		if len(key) != 2 {
			continue
		}
		set, ok := themeSets[key[1]]
		if !ok {
			t.Errorf("unknown theme %q", key[1])
			continue
		}
		seen[key[1]]++
		got := map[string]int{}
		for _, match := range token.FindAllStringSubmatch(cells[3], -1) {
			got[match[1]]++
			if !set[match[1]] {
				t.Errorf("%s lists unexpected %s", key[1], match[1])
			}
		}
		for name := range set {
			if got[name] != 1 {
				t.Errorf("%s lists %s %d times", key[1], name, got[name])
			}
		}
	}
	for name := range themeSets {
		if seen[name] != 1 {
			t.Errorf("theme %s has %d map rows", name, seen[name])
		}
	}
}
