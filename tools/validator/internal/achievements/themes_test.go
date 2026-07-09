package achievements

import "testing"

// namedThemeSets is the six theme sets, keyed for reporting.
func namedThemeSets() map[string]map[string]bool {
	return map[string]map[string]bool{
		ThemeEmbodiedCarbon: embodiedCarbonPrograms,
		ThemeCircularity:    circularityPrograms,
		ThemeMaterialHealth: materialHealthPrograms,
		ThemeEnergy:         energyPrograms,
		ThemeDarkSky:        darkSkyPrograms,
		ThemeEmergency:      emergencyPrograms,
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
	for k, v := range declarationTokens {
		if !decl[k] {
			t.Errorf("declarationTokens key %q is not a SustainabilityDeclarationType member", k)
		}
		if !prog[v] {
			t.Errorf("declarationTokens value %q is not an AttestationProgram member", v)
		}
	}
	if !decl["manufacturer_recycle_program"] {
		t.Error("manufacturer_recycle_program is expected to be a SustainabilityDeclarationType member")
	}
	if _, routed := declarationTokens["manufacturer_recycle_program"]; routed {
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

// 5.10 EXHAUSTIVENESS: the union of the six theme sets, the restricted set, and the
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
