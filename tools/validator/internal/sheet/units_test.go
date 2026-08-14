package sheet

import (
	"sort"
	"strings"
	"testing"
)

// TestSIValueInverse pins the entry-side conversions: every family inverts the
// same rule companionValue applies outward, and the computed SI leaf is rounded
// to at most 4 decimal places. The outward direction is pinned here too, so the
// named-constant refactor cannot move a shipped value.
func TestSIValueInverse(t *testing.T) {
	cases := []struct {
		name      string
		kind      dualUnitKind
		companion float64
		want      float64
	}{
		{"length 4.5 in", dualLength, 4.5, 114.3},
		// 96 * 25.4 evaluates to 2438.3999999999996 in IEEE 754 arithmetic; the
		// rounding removes the artifact (the linear.go precedent).
		{"length 96 in float noise", dualLength, 96, 2438.4},
		{"mass 25 lb", dualMass, 25, 11.3398},
		{"temperature 70 f", dualTemperature, 70, 21.1111},
		{"temperature -40 f", dualTemperature, -40, -40},
		{"area 10.7639 ft2", dualArea, 10.7639, 1},
		{"mass per length 0.671969 lb_per_ft", dualMassPerLength, 0.671969, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.kind.siValue(tc.companion); got != tc.want {
				t.Errorf("siValue(%v) = %v, want %v", tc.companion, got, tc.want)
			}
		})
	}

	// The authored leaf lands verbatim; only the computed side is rounded.
	obj := buildDualUnitFromImperial(dualLength, 4.45, "rated", nil)
	if in, _ := obj["in"].(float64); in != 4.45 {
		t.Errorf("authored in = %v, want 4.45 verbatim", obj["in"])
	}
	if mm, _ := obj["mm"].(float64); mm != 113.03 {
		t.Errorf("computed mm = %v, want 113.03", obj["mm"])
	}
	if vt, _ := obj["value_type"].(string); vt != "rated" {
		t.Errorf("value_type = %v, want rated", obj["value_type"])
	}
	if _, ok := obj["provenance"]; ok {
		t.Errorf("provenance must be omitted when nil, got %v", obj["provenance"])
	}

	// Integer-ness is preserved on both leaves, matching buildDualUnit.
	whole := buildDualUnitFromImperial(dualLength, 12, "rated", nil)
	if in, ok := whole["in"].(int64); !ok || in != 12 {
		t.Errorf("authored in = %#v, want int64(12)", whole["in"])
	}
	if mm, _ := whole["mm"].(float64); mm != 304.8 {
		t.Errorf("computed mm = %#v, want 304.8", whole["mm"])
	}

	// The outward direction is unmoved by the named-constant refactor.
	if got := dualLength.companionValue(113); got != 4.4488 {
		t.Errorf("companionValue(113 mm) = %v, want 4.4488", got)
	}
	if got := dualMass.companionValue(0.7); got != 1.5 {
		t.Errorf("companionValue(0.7 kg) = %v, want 1.5", got)
	}
}

// TestImperialCompanionColumns pins the generated Imperial entry columns: the
// suffix convention the derivation depends on, one companion per SI column with
// everything but the header and kind copied, the exact ten headers this release
// adds, and the uniqueness of the merged column set.
func TestImperialCompanionColumns(t *testing.T) {
	// (a) Every KindDualUnitSI header carries its family's SI suffix, so header
	// derivation cannot silently misfire.
	for _, c := range baseRecordColumns {
		if c.Kind != KindDualUnitSI {
			continue
		}
		suffixes, ok := imperialSuffix[c.DualKind]
		if !ok {
			t.Errorf("column %q has dual-unit family %d with no suffix pair", c.Header, c.DualKind)
			continue
		}
		if !strings.HasSuffix(c.Header, suffixes[0]) {
			t.Errorf("column %q does not carry its family's SI suffix %q", c.Header, suffixes[0])
		}
	}

	// (b) One companion per SI column, everything but header/kind/SIHeader copied.
	companions := imperialCompanions(baseRecordColumns)
	siCols := []Column{}
	for _, c := range baseRecordColumns {
		if c.Kind == KindDualUnitSI {
			siCols = append(siCols, c)
		}
	}
	if len(companions) != len(siCols) {
		t.Fatalf("got %d companions for %d SI columns", len(companions), len(siCols))
	}
	for i, imp := range companions {
		si := siCols[i]
		if imp.Kind != KindDualUnitImperial {
			t.Errorf("companion %q kind = %d, want KindDualUnitImperial", imp.Header, imp.Kind)
		}
		if imp.SIHeader != si.Header {
			t.Errorf("companion %q SIHeader = %q, want %q", imp.Header, imp.SIHeader, si.Header)
		}
		if imp.Path != si.Path || imp.DualKind != si.DualKind {
			t.Errorf("companion %q path/family = %q/%d, want %q/%d", imp.Header, imp.Path, imp.DualKind, si.Path, si.DualKind)
		}
		if imp.ProvSource != si.ProvSource || imp.ProvMethod != si.ProvMethod || imp.ProvValueType != si.ProvValueType {
			t.Errorf("companion %q provenance defaults differ from %q", imp.Header, si.Header)
		}
		if !imp.Provenanced() {
			// (d) The predicate must cover the new kind.
			t.Errorf("companion %q must report Provenanced", imp.Header)
		}
	}

	// (c) Exactly these ten headers.
	want := []string{
		"ceiling_aperture_in",
		"connection_cable_length_in",
		"linear_mass_per_foot_lb_per_ft",
		"luminaire_mass_lb",
		"overall_diameter_in",
		"overall_height_in",
		"overall_length_in",
		"overall_width_in",
		"recess_depth_in",
		"reference_length_in",
	}
	got := make([]string, 0, len(companions))
	for _, c := range companions {
		got = append(got, c.Header)
	}
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("generated headers =\n  %v\nwant\n  %v", got, want)
	}

	// (e) No two columns in the merged set share a header.
	seen := map[string]bool{}
	for _, c := range recordColumns {
		if seen[c.Header] {
			t.Errorf("duplicate header %q in recordColumns", c.Header)
		}
		seen[c.Header] = true
	}
	if len(recordColumns) != len(baseRecordColumns)+len(companions) {
		t.Errorf("recordColumns has %d entries, want %d base + %d generated",
			len(recordColumns), len(baseRecordColumns), len(companions))
	}
}
