package completeness

import (
	"sort"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// This file holds the v0.10.0 exit-sign & emergency class tests: the five profile
// ladders, the auto-full guards, the UL 924 acceptance/application cases, the
// mode-semantics and block-demand pins, the hasIntegralBattery negative, the input-power
// re-gate negative, and the class-category hostile-input coverage. Fixtures are synthetic
// (the real-data rule applies to example RECORDS, not test fixtures) and follow the
// discipline in §5.2: each core rung OMITS every field the sign/emergency profile marks
// not-applicable at core, so a forgotten §4.1 matrix edit surfaces as a level change.

const fakeHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// gapPaths returns the sorted paths of the applicable-but-absent gating rows at lvl:
// the exact to-lvl roadmap set. Used for the exact-set ladder assertions.
func gapPaths(rec map[string]any, lvl Level) []string {
	out := []string{}
	for _, ru := range missingAt(rec, lvl) {
		out = append(out, ru.path)
	}
	sort.Strings(out)
	return out
}

func assertPaths(t *testing.T, label string, got, want []string) {
	t.Helper()
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("%s: gap set mismatch\n got: %v\nwant: %v", label, got, want)
	}
}

// --- sign fixtures ---

// signBase is the identity + safety core shared by every exit-sign fixture, for the
// given illumination mode. It OMITS every universal field the sign profile marks N/A at
// core. ul_924 satisfies both the general safety row and the naDedicatedClass UL 924
// listing row; the region is NA (US-first).
func signBase(mode string) map[string]any {
	return map[string]any{
		"product_family": map[string]any{
			"manufacturer":       map[string]any{"slug": "acme", "display_name": "Acme Safety"},
			"catalog_model":      "ExitGuard",
			"cutsheet":           map[string]any{"filename": "eg.pdf", "sha256": fakeHash},
			"primary_category":   "exit_sign",
			"indoor_outdoor":     "indoor",
			"mounting_types":     []any{"surface_wall"},
			"environment_rating": "dry",
			"shape":              "rectangular",
			"technical_region":   "120v_60hz_north_america",
			"shared_attestations": []any{
				map[string]any{"program": "ul_924"},
			},
		},
		"exit_sign": map[string]any{
			"illumination_mode": mode,
			"legend_color":      "red",
		},
	}
}

// addSignMechanical adds the two universal standard material rows every sign needs.
func addSignMechanical(r map[string]any) {
	pf := r["product_family"].(map[string]any)
	pf["shared_mechanical"] = map[string]any{"housing_material": "aluminum_unspecified", "lens_material": "acrylic"}
}

// addSignGeometry adds the three all-sign standard rows (legend_height, face_count,
// directional_indicator).
func addSignGeometry(r map[string]any) {
	sign := r["exit_sign"].(map[string]any)
	sign["legend_height"] = map[string]any{"mm": float64(152), "in": float64(6)}
	sign["face_count"] = "single"
	sign["directional_indicator"] = []any{"none"}
}

func provNum(v float64, valueType string) map[string]any {
	return map[string]any{"value": v, "value_type": valueType}
}

func testReportNum(v float64) map[string]any {
	return map[string]any{
		"value": v, "value_type": "measured",
		"provenance": map[string]any{"source": "test_report", "method": "extracted"},
	}
}

// (a) internally illuminated LED battery sign (the category-c combo shape).
func comboSignCore() map[string]any {
	r := signBase("internally_illuminated")
	r["exit_sign"].(map[string]any)["illumination_technology"] = "led"
	r["emergency"] = map[string]any{
		"emergency_role": "combo_exit_emergency",
		"power_source":   "integral_battery",
	}
	return r
}
func comboSignStandard() map[string]any {
	r := comboSignCore()
	addSignMechanical(r)
	addSignGeometry(r)
	r["electrical"] = map[string]any{"input_power_w": provNum(3, "rated")}
	em := r["emergency"].(map[string]any)
	em["battery_duration_min"] = provNum(90, "rated")
	em["battery_chemistry"] = "ni_cd"
	em["self_test"] = "self_diagnostic"
	return r
}
func comboSignFull() map[string]any {
	r := comboSignStandard()
	r["exit_sign"].(map[string]any)["sign_face_luminance_cd_per_m2"] = testReportNum(120)
	return r
}

// (b) externally illuminated sign (unpowered face; no emergency block).
func externalSignCore() map[string]any {
	r := signBase("externally_illuminated")
	r["exit_sign"].(map[string]any)["illumination_technology"] = "led"
	return r
}
func externalSignStandard() map[string]any {
	r := externalSignCore()
	addSignMechanical(r)
	addSignGeometry(r)
	sign := r["exit_sign"].(map[string]any)
	sign["face_illuminance_lx"] = provNum(54, "rated")
	sign["contrast_ratio"] = provNum(0.6, "rated")
	return r
}
func externalSignFull() map[string]any {
	r := externalSignStandard()
	r["exit_sign"].(map[string]any)["face_illuminance_lx"] = testReportNum(60)
	return r
}

// externalComboSignFull is an externally illuminated sign that ALSO discloses a
// battery-backed emergency block (a combo unit whose heads run on a battery). power_source
// is NOT a core field for an external sign, so the block only ever appears on this full
// side; the battery trio must stay non-gating here (it nudges as enrichment). Paired with
// externalSignCore in the read-only-core invariant test, it locks that the battery-trio
// gate reads only the power-source-core class: before the §2.9 fix this pair failed.
func externalComboSignFull() map[string]any {
	r := externalSignFull()
	r["emergency"] = map[string]any{
		"emergency_role":       "combo_exit_emergency",
		"power_source":         "integral_battery",
		"battery_duration_min": provNum(90, "rated"),
		"battery_chemistry":    "ni_cd",
		"self_test":            "self_diagnostic",
	}
	return r
}

// (c) photoluminescent sign (unpowered; luminance gates at standard).
func photoSignCore() map[string]any {
	r := signBase("photoluminescent")
	r["exit_sign"].(map[string]any)["illumination_technology"] = "photoluminescent"
	return r
}
func photoSignStandard() map[string]any {
	r := photoSignCore()
	addSignMechanical(r)
	addSignGeometry(r)
	sign := r["exit_sign"].(map[string]any)
	sign["sign_face_luminance_cd_per_m2"] = provNum(0.3, "rated")
	sign["min_charging_illuminance_lx"] = provNum(54, "rated")
	return r
}
func photoSignFull() map[string]any {
	r := photoSignStandard()
	r["exit_sign"].(map[string]any)["sign_face_luminance_cd_per_m2"] = testReportNum(0.3)
	return r
}

// (d) self-luminous (tritium) sign (unpowered; luminance + rated life at standard).
func tritiumSignCore() map[string]any {
	r := signBase("self_luminous")
	r["exit_sign"].(map[string]any)["illumination_technology"] = "tritium"
	return r
}
func tritiumSignStandard() map[string]any {
	r := tritiumSignCore()
	addSignMechanical(r)
	addSignGeometry(r)
	sign := r["exit_sign"].(map[string]any)
	sign["sign_face_luminance_cd_per_m2"] = provNum(0.3, "rated")
	sign["tritium_rated_life_years"] = float64(20)
	return r
}
func tritiumSignFull() map[string]any {
	r := tritiumSignStandard()
	r["exit_sign"].(map[string]any)["sign_face_luminance_cd_per_m2"] = testReportNum(0.3)
	return r
}

// (e) dedicated emergency luminaire (category b): the normal profile minus efficacy,
// plus emergency gates. Built from the neutral bases with primary_category flipped,
// efficacy dropped (notDedicatedClass), a non_dimming driver (no dimming detail), a
// ul_924 listing, and an integral-battery emergency block.
func emgLuminaireCore() map[string]any {
	r := coreBase()
	pf := r["product_family"].(map[string]any)
	pf["primary_category"] = "emergency_luminaire"
	pf["shared_attestations"] = []any{map[string]any{"program": "ul_924"}}
	r["electrical"].(map[string]any)["driver_protocol"] = "non_dimming"
	delete(r["photometry"].(map[string]any), "luminaire_efficacy_lm_per_w") // notDedicatedClass N/A
	r["emergency"] = map[string]any{
		"emergency_role": "dedicated_emergency_luminaire",
		"power_source":   "integral_battery",
	}
	return r
}
func emgLuminaireStandard() map[string]any {
	r := standardBase()
	pf := r["product_family"].(map[string]any)
	pf["primary_category"] = "emergency_luminaire"
	pf["shared_attestations"] = []any{map[string]any{"program": "ul_924"}, map[string]any{"program": "lm_79_08"}}
	r["electrical"].(map[string]any)["driver_protocol"] = "non_dimming"
	delete(r["electrical"].(map[string]any), "dimming_method")
	delete(r["electrical"].(map[string]any), "dimming_range_percent")
	delete(r["photometry"].(map[string]any), "luminaire_efficacy_lm_per_w")
	r["emergency"] = map[string]any{
		"emergency_role":       "dedicated_emergency_luminaire",
		"power_source":         "integral_battery",
		"battery_duration_min": provNum(90, "rated"),
		"battery_chemistry":    "ni_cd",
		"self_test":            "self_test",
	}
	return r
}
func emgLuminaireFull() map[string]any {
	r := fullBase()
	pf := r["product_family"].(map[string]any)
	pf["primary_category"] = "emergency_luminaire"
	pf["shared_attestations"] = []any{map[string]any{"program": "ul_924"}, map[string]any{"program": "lm_79_08"}}
	r["electrical"].(map[string]any)["driver_protocol"] = "non_dimming"
	delete(r["electrical"].(map[string]any), "dimming_method")
	delete(r["electrical"].(map[string]any), "dimming_range_percent")
	delete(r["photometry"].(map[string]any), "luminaire_efficacy_lm_per_w")
	r["emergency"] = map[string]any{
		"emergency_role":       "dedicated_emergency_luminaire",
		"power_source":         "integral_battery",
		"battery_duration_min": provNum(90, "rated"),
		"battery_chemistry":    "ni_cd",
		"self_test":            "self_test",
	}
	return r
}

// TestSignAndEmergencyLadders walks incomplete -> core -> standard -> full for each of
// the five profiles, asserting the exact level at each rung and the exact to-standard
// gap set at the core rung (so a forgotten matrix edit or mis-scoped mode conditional
// changes the set).
func TestSignAndEmergencyLadders(t *testing.T) {
	type ladder struct {
		name              string
		core, std, full   func() map[string]any
		coreToStandardSet []string
	}
	ladders := []ladder{
		{
			name: "combo-internally-illuminated", core: comboSignCore, std: comboSignStandard, full: comboSignFull,
			coreToStandardSet: []string{
				"/electrical/input_power_w",
				"/emergency/battery_chemistry", "/emergency/battery_duration_min", "/emergency/self_test",
				"/exit_sign/directional_indicator", "/exit_sign/face_count", "/exit_sign/legend_height",
				"/product_family/shared_mechanical/housing_material", "/product_family/shared_mechanical/lens_material",
			},
		},
		{
			name: "externally-illuminated", core: externalSignCore, std: externalSignStandard, full: externalSignFull,
			coreToStandardSet: []string{
				"/exit_sign/contrast_ratio", "/exit_sign/directional_indicator", "/exit_sign/face_count",
				"/exit_sign/face_illuminance_lx", "/exit_sign/legend_height",
				"/product_family/shared_mechanical/housing_material", "/product_family/shared_mechanical/lens_material",
			},
		},
		{
			name: "photoluminescent", core: photoSignCore, std: photoSignStandard, full: photoSignFull,
			coreToStandardSet: []string{
				"/exit_sign/directional_indicator", "/exit_sign/face_count", "/exit_sign/legend_height",
				"/exit_sign/min_charging_illuminance_lx", "/exit_sign/sign_face_luminance_cd_per_m2",
				"/product_family/shared_mechanical/housing_material", "/product_family/shared_mechanical/lens_material",
			},
		},
		{
			name: "self-luminous-tritium", core: tritiumSignCore, std: tritiumSignStandard, full: tritiumSignFull,
			coreToStandardSet: []string{
				"/exit_sign/directional_indicator", "/exit_sign/face_count", "/exit_sign/legend_height",
				"/exit_sign/sign_face_luminance_cd_per_m2", "/exit_sign/tritium_rated_life_years",
				"/product_family/shared_mechanical/housing_material", "/product_family/shared_mechanical/lens_material",
			},
		},
		{
			name: "dedicated-emergency-luminaire", core: emgLuminaireCore, std: emgLuminaireStandard, full: emgLuminaireFull,
			// The normal profile minus luminaire efficacy, plus the battery trio and the UL 924
			// listing. Pinned exactly like the four sign ladders, so a mis-scoped emergency gate,
			// a dropped battery row, or efficacy leaking back in fails here rather than passing on
			// the level check alone (this profile's exact standard set is pinned nowhere else).
			coreToStandardSet: []string{
				"/colorimetry/sdcm_step", "/configuration/tested_axes/cri_tier", "/electrical/control_gear_type",
				"/emergency/battery_chemistry", "/emergency/battery_duration_min", "/emergency/self_test",
				"/instrumentation/measurement_regime", "/lumen_maintenance_luminaire (or /lumen_maintenance_package)",
				"/photometry/maximum_intensity_cd", "/photometry/photometric_coordinate_system", "/photometry/symmetry_type",
				"/product_family/shared_mechanical/housing_material", "/product_family/shared_mechanical/lens_material",
				"/test_conditions/photometry_basis", "LM-79 attestation",
			},
		},
	}
	for _, l := range ladders {
		l := l
		t.Run(l.name, func(t *testing.T) {
			if got := AchievedLevel(l.core()); got != LevelCore {
				t.Errorf("core rung graded %s, want core (to-standard gaps: %v)", got, gapPaths(l.core(), LevelStandard))
			}
			if got := AchievedLevel(l.std()); got != LevelStandard {
				t.Errorf("standard rung graded %s, want standard (to-full gaps: %v; to-standard: %v)", got, gapPaths(l.std(), LevelFull), gapPaths(l.std(), LevelStandard))
			}
			if got := AchievedLevel(l.full()); got != LevelFull {
				t.Errorf("full rung graded %s, want full (remaining gaps: %v)", got, gapPaths(l.full(), LevelFull))
			}
			// Incomplete rung: drop a core requirement (legend_color for signs,
			// power_source for the emergency luminaire) and confirm the floor.
			inc := l.core()
			if sign, ok := inc["exit_sign"].(map[string]any); ok {
				delete(sign, "legend_color")
			} else {
				delete(inc["emergency"].(map[string]any), "power_source")
			}
			if got := AchievedLevel(inc); got != LevelIncomplete {
				t.Errorf("incomplete rung graded %s, want incomplete", got)
			}
			if l.coreToStandardSet != nil {
				assertPaths(t, l.name+" core->standard", gapPaths(l.core(), LevelStandard), l.coreToStandardSet)
			} else {
				t.Logf("%s core->standard gaps: %v", l.name, gapPaths(l.core(), LevelStandard))
			}
		})
	}
}

// TestAutoFullPreventionNegatives (§5.3): a standard-complete sign whose luminance/
// illuminance value lacks test_report provenance (or has none) stays standard, never
// auto-promoting to full on the mode-partitioned full row.
func TestAutoFullPreventionNegatives(t *testing.T) {
	// External sign, standard-complete, illuminance present but NOT test_report-backed.
	ext := externalSignStandard()
	if got := AchievedLevel(ext); got != LevelStandard {
		t.Errorf("external standard sign without test-report illuminance graded %s, want standard", got)
	}
	// Internally illuminated combo, luminance present but provenance.source != test_report.
	combo := comboSignStandard()
	combo["exit_sign"].(map[string]any)["sign_face_luminance_cd_per_m2"] = map[string]any{
		"value": float64(120), "value_type": "measured",
		"provenance": map[string]any{"source": "datasheet_pdf", "method": "extracted"},
	}
	if got := AchievedLevel(combo); got != LevelStandard {
		t.Errorf("combo sign with non-test-report luminance graded %s, want standard", got)
	}
	// Same, but with a measured value and NO provenance object at all: still standard.
	combo2 := comboSignStandard()
	combo2["exit_sign"].(map[string]any)["sign_face_luminance_cd_per_m2"] = provNum(120, "measured")
	if got := AchievedLevel(combo2); got != LevelStandard {
		t.Errorf("combo sign with measured-but-unprovenanced luminance graded %s, want standard", got)
	}
}

// ruleByLevelPath finds the rubric row at (lvl, path); the applicability lookups below
// use it to pin which mode-conditional rows are (in)applicable to a given fixture.
func ruleByLevelPath(lvl Level, path string) (rule, bool) {
	for _, ru := range rubric {
		if ru.level == lvl && ru.path == path {
			return ru, true
		}
	}
	return rule{}, false
}

func applicableTo(t *testing.T, lvl Level, path string, rec map[string]any) bool {
	t.Helper()
	ru, ok := ruleByLevelPath(lvl, path)
	if !ok {
		t.Fatalf("no rubric row at (level %s, path %q)", lvl, path)
	}
	return ru.applicable == nil || ru.applicable(rec)
}

func countGapPaths(rec map[string]any, lvl Level, path string) int {
	n := 0
	for _, ru := range missingAt(rec, lvl) {
		if ru.path == path {
			n++
		}
	}
	return n
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// TestUL924AcceptanceAndApplication (§5.6): the four UL 924 cases.
func TestUL924AcceptanceAndApplication(t *testing.T) {
	// Case 1: NA sign whose ONLY listing is ul_924 clears core (both the general safety
	// row and the naDedicatedClass UL 924 listing row are satisfied by ul_924).
	t.Run("na-only-ul924-passes-both", func(t *testing.T) {
		rec := comboSignCore() // shared_attestations = [ul_924], NA region
		if got := AchievedLevel(rec); got != LevelCore {
			t.Fatalf("graded %s, want core (to-core gaps: %v)", got, gapPaths(rec, LevelCore))
		}
		if len(gapPaths(rec, LevelCore)) != 0 {
			t.Errorf("expected no to-core gaps; got %v", gapPaths(rec, LevelCore))
		}
		if !hasMarketSafetyListing(rec) || !hasUL924Listing(rec) {
			t.Error("ul_924 must satisfy both hasMarketSafetyListing and hasUL924Listing")
		}
	})

	// Case 2: non-NA sign with iec_60598 passes the general row and has NO UL 924 row
	// applied (naDedicatedClass is false outside NA).
	t.Run("non-na-iec-passes-general-no-ul924", func(t *testing.T) {
		rec := comboSignCore()
		pf := rec["product_family"].(map[string]any)
		pf["technical_region"] = "230v_50hz_europe"
		pf["shared_attestations"] = []any{map[string]any{"program": "iec_60598"}}
		if applicableTo(t, LevelCore, "UL 924 listing", rec) {
			t.Error("UL 924 listing row must NOT apply to a non-NA record")
		}
		if !hasMarketSafetyListing(rec) {
			t.Error("iec_60598 must satisfy the general safety row for a non-NA record")
		}
		if got := AchievedLevel(rec); got != LevelCore {
			t.Errorf("graded %s, want core (to-core gaps: %v)", got, gapPaths(rec, LevelCore))
		}
	})

	// Case 3: non-NA sign whose ONLY listing is ul_924 passes the general row (the
	// anySafetyListings half of the widening).
	t.Run("non-na-only-ul924-passes-general", func(t *testing.T) {
		rec := comboSignCore()
		rec["product_family"].(map[string]any)["technical_region"] = "230v_50hz_europe"
		if !hasMarketSafetyListing(rec) {
			t.Error("ul_924 must satisfy the general safety row for a non-NA record (anySafetyListings)")
		}
		if got := AchievedLevel(rec); got != LevelCore {
			t.Errorf("graded %s, want core", got)
		}
	})

	// Case 4 (under-application): a NA dedicated record whose only listing is ul_listed
	// (NOT ul_924) grades incomplete, with the UL 924 listing prose-label row in its
	// to-core roadmap. Guards against the row silently never applying, which would leave
	// cases 1-3 vacuously green.
	t.Run("na-ul_listed-not-ul924-incomplete", func(t *testing.T) {
		rec := comboSignCore()
		rec["product_family"].(map[string]any)["shared_attestations"] = []any{map[string]any{"program": "ul_listed"}}
		if got := AchievedLevel(rec); got != LevelIncomplete {
			t.Fatalf("graded %s, want incomplete (ul_listed satisfies general safety but not UL 924)", got)
		}
		if !contains(gapPaths(rec, LevelCore), "UL 924 listing") {
			t.Errorf("expected the UL 924 listing row in the to-core roadmap; got %v", gapPaths(rec, LevelCore))
		}
	})
}

// TestModeSemanticsAndBlockDemand (§5.9): mode absent, junk mode, and the block-demand
// negative.
func TestModeSemanticsAndBlockDemand(t *testing.T) {
	// (a) mode ABSENT: an otherwise class-core-complete sign grades incomplete with the
	// illumination_mode core gap; the STANDARD luminance row does NOT apply (absent mode
	// is outside {photoluminescent, self_luminous}), while the FULL != row still applies
	// (keeping the §2.6 partition total).
	t.Run("mode-absent", func(t *testing.T) {
		rec := signBase("internally_illuminated")
		delete(rec["exit_sign"].(map[string]any), "illumination_mode")
		if got := AchievedLevel(rec); got != LevelIncomplete {
			t.Fatalf("graded %s, want incomplete", got)
		}
		if !contains(gapPaths(rec, LevelCore), "/exit_sign/illumination_mode") {
			t.Errorf("expected /exit_sign/illumination_mode in to-core gaps; got %v", gapPaths(rec, LevelCore))
		}
		if applicableTo(t, LevelStandard, "/exit_sign/sign_face_luminance_cd_per_m2", rec) {
			t.Error("STANDARD luminance row must NOT apply with an absent mode")
		}
		if !applicableTo(t, LevelFull, "test-report-backed sign-face luminance", rec) {
			t.Error("FULL != luminance row MUST apply with an absent mode (partition totality)")
		}
	})

	// (b) mode JUNK TOKEN: the mode row is satisfied as a non-empty string (schema
	// validation is the token-enforcement layer), the == arms are off, and the != arm is
	// on. A junk mode therefore reaches core and does not engage the standard luminance
	// gate, but does engage the full != row.
	t.Run("mode-junk-token", func(t *testing.T) {
		rec := signBase("not_a_real_mode")
		if contains(gapPaths(rec, LevelCore), "/exit_sign/illumination_mode") {
			t.Error("a non-empty (even junk) mode string must satisfy the illumination_mode row")
		}
		if got := AchievedLevel(rec); got != LevelCore {
			t.Errorf("graded %s, want core", got)
		}
		if applicableTo(t, LevelStandard, "/exit_sign/sign_face_luminance_cd_per_m2", rec) {
			t.Error("STANDARD luminance row must NOT apply with a junk mode (outside the IN-set)")
		}
		if !applicableTo(t, LevelFull, "test-report-backed sign-face luminance", rec) {
			t.Error("FULL != luminance row MUST apply with a junk mode")
		}
	})

	// (c) BLOCK-DEMAND: an internally illuminated sign that is class-core-complete and
	// carries ul_924, but has NO emergency block, grades incomplete with EXACTLY
	// /emergency/power_source in the to-core roadmap (guards the §2.4 block-demanding
	// scope: dropping the signMode(internally_illuminated) arm would leave every other
	// test green).
	t.Run("block-demand-power-source", func(t *testing.T) {
		rec := comboSignCore()
		delete(rec, "emergency")
		if got := AchievedLevel(rec); got != LevelIncomplete {
			t.Fatalf("graded %s, want incomplete", got)
		}
		assertPaths(t, "block-demand to-core", gapPaths(rec, LevelCore), []string{"/emergency/power_source"})
	})
}

// TestInputPowerRegateNegative (§5.12): an internally illuminated sign, standard-complete
// except input_power_w, stays core with the to-standard gap at /electrical/input_power_w,
// and exactly one finding for that path (the core row is notExitSign, so only the sign
// re-gate applies).
func TestInputPowerRegateNegative(t *testing.T) {
	rec := comboSignStandard()
	delete(rec, "electrical")
	if got := AchievedLevel(rec); got != LevelCore {
		t.Fatalf("graded %s, want core", got)
	}
	if !contains(gapPaths(rec, LevelStandard), "/electrical/input_power_w") {
		t.Errorf("expected /electrical/input_power_w in to-standard gaps; got %v", gapPaths(rec, LevelStandard))
	}
	if n := countGapPaths(rec, LevelStandard, "/electrical/input_power_w"); n != 1 {
		t.Errorf("expected exactly one input_power_w gap at standard, got %d", n)
	}
}

// TestNotExitSignExclusivity (§5.12 / Phase B.1): notExitSign is the literal negation of
// isExitSign, so the core input-power row (notExitSign) and the standard sign re-gate
// (isExitSign AND internally illuminated) are never both applicable to one record.
func TestNotExitSignExclusivity(t *testing.T) {
	sign := comboSignCore() // internally illuminated exit sign
	troffer := coreBase()   // a normal fixture
	for _, rec := range []map[string]any{sign, troffer} {
		if notExitSign(rec) == isExitSign(rec) {
			t.Errorf("notExitSign must be the literal negation of isExitSign; both = %v", isExitSign(rec))
		}
	}
	coreApplies := applicableTo(t, LevelCore, "/electrical/input_power_w", sign)
	regateApplies := applicableTo(t, LevelStandard, "/electrical/input_power_w", sign)
	if coreApplies && regateApplies {
		t.Error("core input-power row and the sign re-gate must never both apply to one record")
	}
	if coreApplies {
		t.Error("the core input-power row (notExitSign) must NOT apply to an exit sign")
	}
	if !regateApplies {
		t.Error("the standard re-gate must apply to an internally illuminated exit sign")
	}
	// On a troffer the core row applies and the re-gate does not.
	if !applicableTo(t, LevelCore, "/electrical/input_power_w", troffer) {
		t.Error("the core input-power row must apply to a normal fixture")
	}
	if applicableTo(t, LevelStandard, "/electrical/input_power_w", troffer) {
		t.Error("the sign re-gate must NOT apply to a normal fixture")
	}
}

// TestHasIntegralBatteryFalseNegative (§5.14): an internally illuminated ac_only sign,
// standard-complete for its mode with the battery trio absent, grades standard with ZERO
// findings at the three battery paths (guards against a regression of hasIntegralBattery
// to block-presence semantics, which would over-gate every ac_only / inverter product).
func TestHasIntegralBatteryFalseNegative(t *testing.T) {
	rec := signBase("internally_illuminated")
	rec["exit_sign"].(map[string]any)["illumination_technology"] = "led"
	rec["emergency"] = map[string]any{"emergency_role": "exit_sign_only", "power_source": "ac_only"}
	addSignMechanical(rec)
	addSignGeometry(rec)
	rec["electrical"] = map[string]any{"input_power_w": provNum(3, "rated")}
	if hasIntegralBattery(rec) {
		t.Fatal("hasIntegralBattery must be false for an ac_only unit")
	}
	if got := AchievedLevel(rec); got != LevelStandard {
		t.Fatalf("graded %s, want standard (to-standard gaps: %v)", got, gapPaths(rec, LevelStandard))
	}
	for _, p := range []string{"/emergency/battery_duration_min", "/emergency/battery_chemistry", "/emergency/self_test"} {
		if n := countGapPaths(rec, LevelStandard, p); n != 0 {
			t.Errorf("ac_only sign must have zero battery gaps at %s, got %d", p, n)
		}
	}
}

// TestGradeHostileInputClassFixtures (§5.7): the standard hostile harness substitutes
// into a troffer, where the class closures never execute. This substitutes wrong-typed
// values into a sign-full and an emergency-full fixture at every path the new closures
// read (the blocks, the class-core leaves, the provenance object and its source leaf,
// and the attestation surfaces) and asserts AchievedLevel and Report never panic.
func TestGradeHostileInputClassFixtures(t *testing.T) {
	hostiles := []any{map[string]any{}, []any{}, nil, "x", 1.0, []any{1.0, 2.0}}
	targets := []struct {
		parent []string
		key    string
	}{
		{nil, "exit_sign"},
		{[]string{"exit_sign"}, "illumination_mode"},
		{[]string{"exit_sign"}, "legend_height"},
		{[]string{"exit_sign"}, "sign_face_luminance_cd_per_m2"},
		{[]string{"exit_sign", "sign_face_luminance_cd_per_m2"}, "provenance"},
		{[]string{"exit_sign", "sign_face_luminance_cd_per_m2", "provenance"}, "source"},
		{nil, "emergency"},
		{[]string{"emergency"}, "power_source"},
		{[]string{"emergency"}, "battery_duration_min"},
		{[]string{"product_family"}, "shared_attestations"},
		{nil, "attestations"},
	}
	builders := map[string]func() map[string]any{"sign-full": comboSignFull, "emergency-full": emgLuminaireFull}
	for name, build := range builders {
		for _, tgt := range targets {
			for _, h := range hostiles {
				name, tgt, h := name, tgt, h
				label := name + ":" + strings.Join(tgt.parent, ".") + "." + tgt.key
				t.Run(label, func(t *testing.T) {
					defer func() {
						if r := recover(); r != nil {
							t.Fatalf("panic on %s = %T: %v", label, h, r)
						}
					}()
					rec := build()
					parent := rec
					ok := true
					for _, p := range tgt.parent {
						next, isMap := parent[p].(map[string]any)
						if !isMap {
							ok = false
							break
						}
						parent = next
					}
					if !ok {
						return // a prior hostile already replaced this parent
					}
					parent[tgt.key] = h
					_ = AchievedLevel(rec)
					report := findings.NewReport()
					Report(rec, report)
					report.Finalize()
				})
			}
		}
	}
}

// enrichmentClassPrefixes scopes the enrichment assertions to the §4.5 exit-sign /
// emergency namespaces, so the exact-set checks test the v0.10.0 nudges precisely and are
// not coupled to the universal v0.9.0 nudges (orientation, warranty, ...) that ride their
// existing predicates and intentionally nudge the new classes too (§4.1).
var enrichmentClassPrefixes = []string{"/exit_sign/", "/emergency/", "/electrical/input_voltage"}

func classEnrichmentPaths(t *testing.T, rec map[string]any) []string {
	t.Helper()
	report := findings.NewReport()
	Report(rec, report)
	report.Finalize()
	out := []string{}
	for _, f := range report.Findings {
		if f.Code != findings.CodeConformanceEnrichment {
			continue
		}
		for _, p := range enrichmentClassPrefixes {
			if strings.HasPrefix(f.Path, p) {
				out = append(out, f.Path)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

const inputVoltagePath = "/electrical/input_voltage_v (or input_voltage_class)"

// TestSignEnrichmentSets (§5.11): the exact §4.5-namespace enrichment set for each of the
// six role/mode shapes. The technology nudge fires on every non-externally-illuminated
// sign when illumination_technology is absent (a negation-applicability easy to miss);
// fixtures that declare it (a, c) omit it from the expected set.
func TestSignEnrichmentSets(t *testing.T) {
	// (a) internally illuminated combo (integral_battery, combo_exit_emergency), technology
	// declared: the three emergency-mode nudges + input_voltage + rated_viewing +
	// internal-luminance. The battery trio are STANDARD GATES (dedicated class), not nudges.
	t.Run("combo", func(t *testing.T) {
		assertPaths(t, "combo", classEnrichmentPaths(t, comboSignCore()), []string{
			inputVoltagePath,
			"/emergency/emergency_input_power_w", "/emergency/emergency_lumen_output_lm", "/emergency/photometry_reference",
			"/exit_sign/rated_viewing_distance_ft", "/exit_sign/sign_face_luminance_cd_per_m2",
		})
	})

	// (b) category-d unit (non-dedicated, emergency_power_option, integral_battery): the
	// three emergency-mode nudges AND the battery trio as nudges. No sign nudges.
	t.Run("category-d", func(t *testing.T) {
		rec := coreBase()
		rec["emergency"] = map[string]any{"emergency_role": "emergency_power_option", "power_source": "integral_battery"}
		assertPaths(t, "category-d", classEnrichmentPaths(t, rec), []string{
			"/emergency/battery_chemistry", "/emergency/battery_duration_min", "/emergency/self_test",
			"/emergency/emergency_input_power_w", "/emergency/emergency_lumen_output_lm", "/emergency/photometry_reference",
		})
	})

	// (c) exit_sign_only plain battery sign: NONE of the three emergency-mode nudges (role
	// not in the set) and NO battery nudges (dedicated -> those are standard gates). Only
	// the sign nudges (technology declared, so omitted).
	t.Run("exit_sign_only", func(t *testing.T) {
		rec := signBase("internally_illuminated")
		rec["exit_sign"].(map[string]any)["illumination_technology"] = "led"
		rec["emergency"] = map[string]any{"emergency_role": "exit_sign_only", "power_source": "integral_battery"}
		assertPaths(t, "exit_sign_only", classEnrichmentPaths(t, rec), []string{
			inputVoltagePath,
			"/exit_sign/rated_viewing_distance_ft", "/exit_sign/sign_face_luminance_cd_per_m2",
		})
	})

	// (d) photoluminescent sign (no emergency block), technology absent: the technology
	// nudge fires; no emergency-block nudges; luminance sits at STANDARD, not enrichment.
	t.Run("photoluminescent", func(t *testing.T) {
		rec := signBase("photoluminescent") // no illumination_technology
		assertPaths(t, "photoluminescent", classEnrichmentPaths(t, rec), []string{
			"/exit_sign/illumination_technology", "/exit_sign/rated_viewing_distance_ft",
		})
	})

	// (e) externally illuminated sign: the three geometry nudges, but NOT the technology
	// nudge (mode excluded) nor the internal-luminance nudge (not internally illuminated).
	t.Run("externally-illuminated", func(t *testing.T) {
		rec := externalSignCore() // declares technology=led, which is not applicable to external anyway
		assertPaths(t, "externally-illuminated", classEnrichmentPaths(t, rec), []string{
			"/exit_sign/letter_spacing", "/exit_sign/letter_width",
			"/exit_sign/rated_viewing_distance_ft", "/exit_sign/stroke_width",
		})
	})

	// (f) category-b dedicated emergency luminaire (block present, dual-mode fields absent):
	// ZERO emergency-mode nudges (role not in emergencyModeRoles). Guards §2.5's
	// no-duplication rule, the more dangerous omission since category b CAN populate those.
	t.Run("category-b", func(t *testing.T) {
		got := classEnrichmentPaths(t, emgLuminaireCore())
		for _, p := range []string{"/emergency/emergency_lumen_output_lm", "/emergency/photometry_reference", "/emergency/emergency_input_power_w"} {
			if contains(got, p) {
				t.Errorf("category-b luminaire must NOT get emergency-mode nudge %q; got set %v", p, got)
			}
		}
		if len(got) != 0 {
			t.Errorf("category-b core luminaire should have no §4.5-namespace enrichment (battery trio are standard gates); got %v", got)
		}
	})
}

// TestCategoryDNoGating (§5.4): a normal fixture plus a minimal emergency block grades
// identically to the same record without the block (the block is non-gating for
// category d), and enrichment nudges appear only at core and above.
func TestCategoryDNoGating(t *testing.T) {
	without := coreBase()
	with := coreBase()
	with["emergency"] = map[string]any{"emergency_role": "emergency_power_option", "power_source": "integral_battery"}
	if AchievedLevel(with) != AchievedLevel(without) {
		t.Errorf("adding a minimal emergency block changed the grade: with=%s without=%s", AchievedLevel(with), AchievedLevel(without))
	}
	// Render gate: an incomplete record emits no enrichment findings.
	inc := coreBase()
	delete(inc["product_family"].(map[string]any), "shared_attestations") // drop the core safety listing
	inc["emergency"] = map[string]any{"emergency_role": "emergency_power_option", "power_source": "integral_battery"}
	if got := AchievedLevel(inc); got != LevelIncomplete {
		t.Fatalf("expected incomplete, got %s", got)
	}
	if paths := classEnrichmentPaths(t, inc); len(paths) != 0 {
		t.Errorf("an incomplete record must emit no enrichment (render is core-gated); got %v", paths)
	}
}

// TestDoubleReportGuard (§5.5): a dedicated-class record missing self_test (and,
// separately, battery_duration_min) yields exactly one finding for it (the standard gap)
// and zero enrichment, so the field is never reported twice (§2.9 disjointness).
func TestDoubleReportGuard(t *testing.T) {
	for _, field := range []string{"self_test", "battery_duration_min"} {
		field := field
		t.Run(field, func(t *testing.T) {
			rec := comboSignStandard() // dedicated class, integral battery, standard-complete
			delete(rec["emergency"].(map[string]any), field)
			path := "/emergency/" + field
			if got := AchievedLevel(rec); got != LevelCore {
				t.Fatalf("dropping %s should leave the sign at core (one standard gap), got %s", field, got)
			}
			if n := countGapPaths(rec, LevelStandard, path); n != 1 {
				t.Errorf("expected exactly one standard gap at %s, got %d", path, n)
			}
			if contains(classEnrichmentPaths(t, rec), path) {
				t.Errorf("%s must not ALSO appear as an enrichment nudge on a dedicated class (double report)", path)
			}
		})
	}
}

// TestExternalComboBatteryIsNudgeNotGate (§2.9 P1 regression): power_source is not a core
// field for an externally illuminated sign, so the battery trio must surface as enrichment,
// never as a standard gate. Disclosing an integral-battery emergency block must therefore
// never LOWER the grade (monotonic disclosure), while the battery fields still nudge so the
// disclosure stays valued.
func TestExternalComboBatteryIsNudgeNotGate(t *testing.T) {
	base := externalSignStandard() // standard, no emergency block
	if got := AchievedLevel(base); got != LevelStandard {
		t.Fatalf("external sign baseline graded %s, want standard", got)
	}
	combo := externalSignStandard()
	combo["emergency"] = map[string]any{
		"emergency_role": "combo_exit_emergency",
		"power_source":   "integral_battery",
		// deliberately no battery depth
	}
	if got := AchievedLevel(combo); got != LevelStandard {
		t.Errorf("disclosing an integral-battery block lowered an external sign from standard to %s (non-monotonic); to-standard gaps: %v", got, gapPaths(combo, LevelStandard))
	}
	trio := []string{"/emergency/battery_duration_min", "/emergency/battery_chemistry", "/emergency/self_test"}
	for _, p := range trio {
		if n := countGapPaths(combo, LevelStandard, p); n != 0 {
			t.Errorf("external combo sign has %s as a standard gap (%d); the battery trio must not gate outside the power-source-core class", p, n)
		}
	}
	enr := classEnrichmentPaths(t, combo)
	for _, p := range trio {
		if !contains(enr, p) {
			t.Errorf("external combo sign should nudge %s as enrichment; enrichment paths: %v", p, enr)
		}
	}
}

// TestSelfEmittingSign pins the named predicate extracted from the sign full-luminance and
// illumination-technology rows: true for every self-emitting mode (internally illuminated,
// photoluminescent, self-luminous, and absent/junk modes via the !externally_illuminated
// arm), false for an externally illuminated sign and for any non-sign.
func TestSelfEmittingSign(t *testing.T) {
	for name, rec := range map[string]map[string]any{
		"internally_illuminated": comboSignCore(),
		"photoluminescent":       photoSignCore(),
		"self_luminous":          tritiumSignCore(),
	} {
		if !selfEmittingSign(rec) {
			t.Errorf("selfEmittingSign(%s sign) = false, want true", name)
		}
	}
	absent := signBase("internally_illuminated")
	delete(absent["exit_sign"].(map[string]any), "illumination_mode")
	if !selfEmittingSign(absent) {
		t.Error("selfEmittingSign(sign with absent mode) = false, want true")
	}
	if selfEmittingSign(externalSignCore()) {
		t.Error("selfEmittingSign(externally illuminated sign) = true, want false")
	}
	if selfEmittingSign(coreBase()) {
		t.Error("selfEmittingSign(non-sign troffer) = true, want false")
	}
}

// TestEmergencyPhotometryReferencePresent (§4.5 present-branch): a dual-mode record that
// attaches an emergency-mode photometry file with a content hash satisfies the enrichment
// row, so the "not attached" nudge does not fire. Exercises hasEmergencyPhotometryReference's
// true branch (the ladder tests exercise the absent branch) and guards its sha256 logic.
func TestEmergencyPhotometryReferencePresent(t *testing.T) {
	const path = "/emergency/photometry_reference"
	absent := comboSignStandard() // dual-mode, rendered at standard, no photometry file
	if !contains(classEnrichmentPaths(t, absent), path) {
		t.Fatalf("a dual-mode record with no emergency photometry file should nudge %s", path)
	}
	present := comboSignStandard()
	present["emergency"].(map[string]any)["photometry_reference"] = map[string]any{
		"filename": "emg.ies", "sha256": fakeHash,
	}
	if contains(classEnrichmentPaths(t, present), path) {
		t.Errorf("attaching an emergency photometry file with a sha256 should satisfy %s; the nudge must not fire", path)
	}
	// Present-but-hashless: a FileReference needs a content hash, so a reference with no
	// sha256 must NOT satisfy the row (the nudge still fires). This pins sha256-keying, not
	// block-presence semantics, mirroring TestHasIntegralBatteryFalseNegative.
	hashless := comboSignStandard()
	hashless["emergency"].(map[string]any)["photometry_reference"] = map[string]any{"filename": "emg.ies"}
	if !contains(classEnrichmentPaths(t, hashless), path) {
		t.Errorf("a photometry reference with no sha256 must not satisfy %s (block-presence regression)", path)
	}
}
