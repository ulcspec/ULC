package completeness

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// repoRoot resolves the repository root from this test package's directory
// (tools/validator/internal/completeness -> four levels up).
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return root
}

// exampleRecord loads and normalizes a record under examples/.
func exampleRecord(t *testing.T, name string) map[string]any {
	t.Helper()
	return loadRecord(t, filepath.Join(repoRoot(t), "examples", name))
}

// findingsFor returns every finding carrying the given code.
func findingsFor(report *findings.Report, code findings.Code) []findings.Finding {
	out := []findings.Finding{}
	for _, f := range report.Findings {
		if f.Code == code {
			out = append(out, f)
		}
	}
	return out
}

func hasObservationAt(report *findings.Report, path string) bool {
	for _, f := range findingsFor(report, findings.CodeConformanceObservation) {
		if f.Path == path {
			return true
		}
	}
	return false
}

func enrichmentAt(report *findings.Report, path string) (findings.Finding, bool) {
	for _, f := range findingsFor(report, findings.CodeConformanceEnrichment) {
		if f.Path == path {
			return f, true
		}
	}
	return findings.Finding{}, false
}

func hasGapFor(report *findings.Report, path string) (findings.Finding, bool) {
	for _, f := range findingsFor(report, findings.CodeConformanceGap) {
		if f.Path == path {
			return f, true
		}
	}
	return findings.Finding{}, false
}

// --- synthetic fixtures ---
//
// The base fixtures use primary_category "panel_troffer", which is intentionally
// neither directional, outdoor-site, nor linear, so the only conditionals that
// fire are the white-light family (static_white) and the analog/phase dimming spec.
// Individual tests flip one axis at a time to isolate a single gate. Integral
// numbers are float64 here; the grader treats float64/int/int64 identically.

// coreBase meets every CORE hard requirement and nothing above it, so it grades
// exactly core. It carries an NA technical_region and a UL listing under
// product_family.shared_attestations (so attestationPrograms reads it there).
func coreBase() map[string]any {
	return map[string]any{
		"product_family": map[string]any{
			"manufacturer":       map[string]any{"slug": "acme", "display_name": "Acme Lighting"},
			"catalog_model":      "Orbit 1200",
			"cutsheet":           map[string]any{"filename": "orbit-1200.pdf", "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			"primary_category":   "panel_troffer",
			"indoor_outdoor":     "indoor",
			"secondary_function": []any{"general_ambient"},
			"mounting_types":     []any{"recessed"},
			"environment_rating": "dry",
			"shape":              "square",
			"technical_region":   "120v_60hz_north_america",
			"shared_attestations": []any{
				map[string]any{"program": "ul_listed"},
			},
		},
		"configuration": map[string]any{
			"tested_axes": map[string]any{"color_tunability": "static_white"},
		},
		"photometry": map[string]any{
			"distribution_type":           "direct",
			"total_luminous_flux_lm":      map[string]any{"value": float64(1200)},
			"luminaire_efficacy_lm_per_w": map[string]any{"value": float64(120)},
		},
		"electrical": map[string]any{
			"driver_protocol": "0-10v",
			"input_power_w":   map[string]any{"value": float64(10)},
			"input_voltage_v": map[string]any{"value": float64(120)},
		},
		"colorimetry": map[string]any{
			"nominal_cct_k": "4000",
			"cri_ra":        map[string]any{"value": float64(90)},
		},
	}
}

// standardBase extends coreBase to meet every hard standard requirement (the
// universal photometry/material/test fields, an LM-79 attestation, a
// lumen-maintenance framework, the white-light cri_tier + sdcm_step, and the
// analog dimming method + range). It grades exactly standard.
func standardBase() map[string]any {
	rec := coreBase()
	phot := rec["photometry"].(map[string]any)
	phot["maximum_intensity_cd"] = map[string]any{"value": float64(5000)}
	phot["symmetry_type"] = "symm_quad"
	phot["photometric_coordinate_system"] = "ies_c"
	el := rec["electrical"].(map[string]any)
	el["control_gear_type"] = "led_driver_constant_current"
	el["dimming_method"] = "pwm"
	el["dimming_range_percent"] = map[string]any{"min": float64(1), "max": float64(100)}
	pf := rec["product_family"].(map[string]any)
	pf["shared_mechanical"] = map[string]any{"housing_material": "aluminum", "lens_material": "pmma"}
	pf["shared_attestations"] = append(pf["shared_attestations"].([]any), map[string]any{"program": "lm_79_08"})
	rec["test_conditions"] = map[string]any{"photometry_basis": "absolute"}
	rec["instrumentation"] = map[string]any{"measurement_regime": "far_field"}
	rec["colorimetry"].(map[string]any)["sdcm_step"] = map[string]any{"value": float64(3)}
	rec["configuration"].(map[string]any)["tested_axes"].(map[string]any)["cri_tier"] = "cri_90"
	rec["lumen_maintenance_luminaire"] = map[string]any{
		"declaration_framework": "manufacturer_rated_claim",
		"manufacturer_rated_claim": map[string]any{
			"claim_type":    "L70",
			"claimed_hours": map[string]any{"value": float64(50000), "value_type": "rated"},
		},
	}
	return rec
}

// fullBase extends standardBase to meet every hard full requirement: zonal lumens,
// an operating point, measurement uncertainty, corrections, instrumentation depth,
// a method-backed maintenance projection, and TM-30 fidelity + hue bins. It grades
// full.
func fullBase() map[string]any {
	rec := standardBase()
	rec["photometry"].(map[string]any)["zonal_lumens"] = []any{
		map[string]any{"zone_label": "0-30", "lumens": map[string]any{"value": float64(500)}},
	}
	rec["operating_point"] = map[string]any{
		"drive_current_ma": map[string]any{"value": float64(350)},
	}
	rec["uncertainty"] = map[string]any{
		"coverage_factor_k":                       float64(2),
		"expanded_uncertainty_total_flux_percent": map[string]any{"value": float64(5), "value_type": "measured"},
	}
	rec["corrections_applied"] = map[string]any{"self_absorption_corrected": true}
	rec["instrumentation"].(map[string]any)["laboratory_accreditation_scheme"] = "iso_17025"
	rec["lumen_maintenance_luminaire"].(map[string]any)["tm_28"] = map[string]any{
		"tm_28_projection_hours": map[string]any{"value": float64(60000)},
	}
	rec["colorimetry"].(map[string]any)["tm_30"] = map[string]any{
		"rf": map[string]any{"value": float64(90)},
		"rf_h_per_bin": []any{
			map[string]any{"bin": float64(1), "rf_h": map[string]any{"value": float64(95), "value_type": "measured"}},
		},
	}
	return rec
}

// --- example pins ---

// TestAchievedLevel pins the computed level on the committed example records.
// These are the values the builder stamps into index.conformance_level.
func TestAchievedLevel(t *testing.T) {
	cases := []struct {
		name string
		want Level
		why  string
	}{
		{"erco-quintessence-30416-023.ulc", LevelStandard, "full ceiling: no zonal/uncertainty/corrections/instrumentation/TM-30 depth"},
		{"selux-aya-pole-sr-ho-3000k.ulc", LevelStandard, "carries zonal + operating_point but lacks test-report depth"},
		{"vode-nexa-suspended-807-so-3500k-90cri-hl-black-48in.ulc", LevelCore, "sole standard gap is sdcm_step (no published MacAdam step)"},
		{"lumenpulse-lumenfacade-loi-12-rgb-30x60-ts0.ulc", LevelStandard, "DMX exempts the dimming detail; pure RGB waives CCT/CRI/SDCM"},
		{"lumenpulse-lumenfacade-loi-12-rgbw30k-10x60-ts2-5.ulc", LevelStandard, "DMX exempts the dimming detail; RGBW carries CCT, waives CRI/SDCM"},
		{"cooper-sure-lites-lpx7sd.ulc", LevelCore, "self-powered internally illuminated exit; standard blocked by its two real disclosure gaps (legend height, self-powered wattage)"},
		{"cooper-sure-lites-es61src.ulc", LevelStandard, "AC-only edge-lit exit; battery trio skips (ac_only); full blocked only by test-report-backed luminance"},
		{"cooper-atlite-auxswhsd.ulc", LevelStandard, "self-powered edge-lit exit; battery trio met; full blocked only by test-report-backed luminance"},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			rec := exampleRecord(t, c.name)
			if got := AchievedLevel(rec); got != c.want {
				t.Errorf("AchievedLevel(%s) = %s, want %s (%s)", c.name, got, c.want, c.why)
			}
		})
	}
}

// TestReportEmitsInfoOnly confirms conformance grading produces only INFO findings
// (no WARNING, no ERROR): a record is whatever level its data achieves.
func TestReportEmitsInfoOnly(t *testing.T) {
	for _, name := range []string{
		"erco-quintessence-30416-023.ulc",
		"selux-aya-pole-sr-ho-3000k.ulc",
		"vode-nexa-suspended-807-so-3500k-90cri-hl-black-48in.ulc",
		"lumenpulse-lumenfacade-loi-12-rgb-30x60-ts0.ulc",
		"lumenpulse-lumenfacade-loi-12-rgbw30k-10x60-ts2-5.ulc",
		"cooper-sure-lites-lpx7sd.ulc",
		"cooper-sure-lites-es61src.ulc",
		"cooper-atlite-auxswhsd.ulc",
	} {
		name := name
		t.Run(name, func(t *testing.T) {
			rec := exampleRecord(t, name)
			report := findings.NewReport()
			Report(rec, report)
			report.Finalize()
			if report.Summary.Errors != 0 || report.Summary.Warnings != 0 {
				t.Errorf("%s: expected INFO-only, got %d errors, %d warnings",
					name, report.Summary.Errors, report.Summary.Warnings)
			}
		})
	}
}

// --- the ladder ---

// TestAchievedLevelIncomplete pins the incomplete rung: a core-complete record
// missing only one core requirement grades incomplete; adding the requirement lifts
// it to core; an anchorless record also grades incomplete (the floor), never below.
func TestAchievedLevelIncomplete(t *testing.T) {
	// Drop the safety listing: core safety gate unmet -> incomplete.
	rec := coreBase()
	delete(rec["product_family"].(map[string]any), "shared_attestations")
	if got := AchievedLevel(rec); got != LevelIncomplete {
		t.Errorf("no safety listing = %s, want incomplete", got)
	}
	// Restore a listing -> core.
	rec["product_family"].(map[string]any)["shared_attestations"] = []any{
		map[string]any{"program": "ul_listed"},
	}
	if got := AchievedLevel(rec); got != LevelCore {
		t.Errorf("with safety listing = %s, want core", got)
	}
	// No photometric anchors -> incomplete (the floor), not a below-floor sentinel.
	if got := AchievedLevel(map[string]any{
		"product_family": map[string]any{"primary_category": "panel_troffer"},
	}); got != LevelIncomplete {
		t.Errorf("no anchors = %s, want incomplete (the floor)", got)
	}
}

// TestReportFloorIsIncomplete pins the floor: an anchorless record grades incomplete
// (never a below-floor sentinel), its CodeConformanceLevel INFO names the "incomplete"
// token, and it carries a to-core roadmap rather than refusing to grade.
func TestReportFloorIsIncomplete(t *testing.T) {
	rec := map[string]any{
		"product_family": map[string]any{"primary_category": "panel_troffer"},
	}
	report := findings.NewReport()
	if got := Report(rec, report); got != LevelIncomplete {
		t.Fatalf("anchorless record = %s, want incomplete", got)
	}
	report.Finalize()
	levels := findingsFor(report, findings.CodeConformanceLevel)
	if len(levels) != 1 {
		t.Fatalf("expected exactly one conformance-level finding, got %d: %+v", len(levels), levels)
	}
	msg := levels[0].Message
	if !strings.Contains(msg, `conformance level "incomplete"`) {
		t.Errorf("floor message should name the incomplete token: %q", msg)
	}
	// The floor always travels with a to-core roadmap.
	if gaps := findingsFor(report, findings.CodeConformanceGap); len(gaps) == 0 {
		t.Errorf("an incomplete record must carry a to-core roadmap; got none")
	}
	// No satisfied-grade findings: nothing is achieved at the floor.
	if sat := findingsFor(report, findings.CodeConformanceGradeSatisfied); len(sat) != 0 {
		t.Errorf("floor record should emit no satisfied-grade findings, got %d", len(sat))
	}
}

// TestSafetyListingCoreGate pins the region-conditional safety acceptance.
func TestSafetyListingCoreGate(t *testing.T) {
	setListing := func(rec map[string]any, programs ...string) {
		arr := []any{}
		for _, p := range programs {
			arr = append(arr, map[string]any{"program": p})
		}
		rec["product_family"].(map[string]any)["shared_attestations"] = arr
	}
	setRegion := func(rec map[string]any, region string) {
		if region == "" {
			delete(rec["product_family"].(map[string]any), "technical_region")
			return
		}
		rec["product_family"].(map[string]any)["technical_region"] = region
	}

	// NA region with only a non-NA listing (CE) -> incomplete.
	rec := coreBase()
	setListing(rec, "ce")
	if got := AchievedLevel(rec); got != LevelIncomplete {
		t.Errorf("NA + CE only = %s, want incomplete", got)
	}
	// NA region with csa_listed -> core.
	setListing(rec, "csa_listed")
	if got := AchievedLevel(rec); got != LevelCore {
		t.Errorf("NA + csa_listed = %s, want core", got)
	}
	// universal region with iec_60598 -> core (anySafetyListings accepts it).
	rec2 := coreBase()
	setRegion(rec2, "universal")
	setListing(rec2, "iec_60598")
	if got := AchievedLevel(rec2); got != LevelCore {
		t.Errorf("universal + iec_60598 = %s, want core", got)
	}
	// hasMarketSafetyListing defaults an empty/unknown region to anySafetyListings.
	// Tested directly because technical_region is itself a core rule, so an
	// empty-region record would grade incomplete on that field, not on the listing.
	noRegion := map[string]any{
		"product_family": map[string]any{
			"shared_attestations": []any{map[string]any{"program": "ce"}},
		},
	}
	if !hasMarketSafetyListing(noRegion) {
		t.Error("empty region should accept CE via anySafetyListings")
	}
	// A North American region rejects CE (not in naSafetyListings).
	noRegion["product_family"].(map[string]any)["technical_region"] = "120v_60hz_north_america"
	if hasMarketSafetyListing(noRegion) {
		t.Error("NA region must reject CE (not an NA-recognized listing)")
	}
}

// TestStandardRequiresSdcm pins the SDCM standard gate (conditional on a primary
// white-light fixture): a white-light record missing sdcm_step caps at core and
// the roadmap names it with its document + standard.
func TestStandardRequiresSdcm(t *testing.T) {
	rec := standardBase()
	delete(rec["colorimetry"].(map[string]any), "sdcm_step")
	report := findings.NewReport()
	if got := Report(rec, report); got != LevelCore {
		t.Fatalf("white-light without sdcm_step = %s, want core", got)
	}
	report.Finalize()
	gap, ok := hasGapFor(report, "/colorimetry/sdcm_step")
	if !ok {
		t.Fatalf("expected a gap finding for /colorimetry/sdcm_step; gaps: %+v",
			findingsFor(report, findings.CodeConformanceGap))
	}
	if gap.NextConformanceLevel != "standard" {
		t.Errorf("sdcm gap next level = %q, want standard", gap.NextConformanceLevel)
	}
	if gap.Standard != "ANSI C78.377" || gap.SourceDocument != "datasheet_pdf" {
		t.Errorf("sdcm gap detail = (%q, %q), want (datasheet_pdf, ANSI C78.377)",
			gap.SourceDocument, gap.Standard)
	}
}

// TestFullRequiresTestReportDepth pins the full tier: fullBase reaches full, and
// removing any single full hard requirement drops it back to standard.
func TestFullRequiresTestReportDepth(t *testing.T) {
	if got := AchievedLevel(fullBase()); got != LevelFull {
		t.Fatalf("fullBase = %s, want full", got)
	}
	cases := []struct {
		name  string
		strip func(map[string]any)
	}{
		{"no zonal_lumens", func(r map[string]any) { delete(r["photometry"].(map[string]any), "zonal_lumens") }},
		{"no operating_point", func(r map[string]any) { delete(r, "operating_point") }},
		{"no uncertainty", func(r map[string]any) { delete(r, "uncertainty") }},
		{"no corrections", func(r map[string]any) { delete(r, "corrections_applied") }},
		{"no instrumentation depth", func(r map[string]any) {
			delete(r["instrumentation"].(map[string]any), "laboratory_accreditation_scheme")
		}},
		{"no method-backed maintenance", func(r map[string]any) {
			delete(r["lumen_maintenance_luminaire"].(map[string]any), "tm_28")
		}},
		{"no TM-30 hue bins", func(r map[string]any) {
			delete(r["colorimetry"].(map[string]any)["tm_30"].(map[string]any), "rf_h_per_bin")
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			rec := fullBase()
			c.strip(rec)
			if got := AchievedLevel(rec); got != LevelStandard {
				t.Errorf("%s = %s, want standard", c.name, got)
			}
		})
	}
}

// TestWhitePointVsWhiteLightPrimary pins the CCT vs white-light-quality split:
// static_white needs both CCT and CRI/SDCM; rgbw needs CCT but waives CRI/SDCM;
// pure rgb waives both.
func TestWhitePointVsWhiteLightPrimary(t *testing.T) {
	setTun := func(rec map[string]any, tun string) {
		rec["configuration"].(map[string]any)["tested_axes"].(map[string]any)["color_tunability"] = tun
	}

	// static_white missing CCT -> incomplete (CCT is core for a white point).
	rec := coreBase()
	delete(rec["colorimetry"].(map[string]any), "nominal_cct_k")
	if got := AchievedLevel(rec); got != LevelIncomplete {
		t.Errorf("static_white without CCT = %s, want incomplete", got)
	}
	// rgbw missing CCT -> incomplete (rgbw has a white point, so CCT is core).
	rec = coreBase()
	setTun(rec, "rgbw")
	delete(rec["colorimetry"].(map[string]any), "nominal_cct_k")
	if got := AchievedLevel(rec); got != LevelIncomplete {
		t.Errorf("rgbw without CCT = %s, want incomplete", got)
	}
	// rgbw missing CRI -> core, NOT incomplete (color-mixing waives CRI). It has CCT.
	rec = coreBase()
	setTun(rec, "rgbw")
	delete(rec["colorimetry"].(map[string]any), "cri_ra")
	if got := AchievedLevel(rec); got != LevelCore {
		t.Errorf("rgbw without CRI = %s, want core (CRI waived for color-mixing)", got)
	}
	// pure rgb missing both CCT and CRI -> core (no white point at all).
	rec = coreBase()
	setTun(rec, "rgb")
	delete(rec["colorimetry"].(map[string]any), "cri_ra")
	delete(rec["colorimetry"].(map[string]any), "nominal_cct_k")
	if got := AchievedLevel(rec); got != LevelCore {
		t.Errorf("pure rgb without CCT/CRI = %s, want core", got)
	}
	// rgbw at standard waives sdcm_step + cri_tier.
	rec = standardBase()
	setTun(rec, "rgbw")
	delete(rec["colorimetry"].(map[string]any), "sdcm_step")
	delete(rec["configuration"].(map[string]any)["tested_axes"].(map[string]any), "cri_tier")
	if got := AchievedLevel(rec); got != LevelStandard {
		t.Errorf("rgbw without sdcm/cri_tier = %s, want standard (waived for color-mixing)", got)
	}
}

// TestDirectionalGate pins the directional conditional: a directional category
// makes beam_angle_deg a hard standard requirement.
func TestDirectionalGate(t *testing.T) {
	cases := []struct {
		category string
		want     Level
	}{
		{"downlight", LevelCore},         // directional: beam_angle required, absent here
		{"in_ground_uplight", LevelCore}, // directional (architectural uplight)
		{"panel_troffer", LevelStandard}, // not directional: no beam angle needed
	}
	for _, c := range cases {
		c := c
		t.Run(c.category, func(t *testing.T) {
			rec := standardBase()
			rec["product_family"].(map[string]any)["primary_category"] = c.category
			if got := AchievedLevel(rec); got != c.want {
				t.Errorf("%s = %s, want %s", c.category, got, c.want)
			}
		})
	}
}

// TestOutdoorSiteGate pins the outdoor-site conditional: an outdoor-site category
// makes the BUG rating, outdoor distribution type, and longitudinal range hard
// standard requirements.
func TestOutdoorSiteGate(t *testing.T) {
	// flood_area_site needs the outdoor-site set; standardBase lacks it -> core.
	rec := standardBase()
	rec["product_family"].(map[string]any)["primary_category"] = "flood_area_site"
	if got := AchievedLevel(rec); got != LevelCore {
		t.Fatalf("flood_area_site without outdoor-site set = %s, want core", got)
	}
	// Add the outdoor-site fields -> standard.
	rec["outdoor_classification"] = map[string]any{
		"outdoor_distribution_type":       "type_iii",
		"longitudinal_distribution_range": "short",
		"bug_rating":                      map[string]any{"b": float64(1), "u": float64(0), "g": float64(1)},
	}
	if got := AchievedLevel(rec); got != LevelStandard {
		t.Errorf("flood_area_site with outdoor-site set = %s, want standard", got)
	}
	// in_ground_uplight is directional (not outdoor-site): no BUG required.
	rec2 := standardBase()
	rec2["product_family"].(map[string]any)["primary_category"] = "in_ground_uplight"
	rec2["photometry"].(map[string]any)["beam_angle_deg"] = map[string]any{"value": float64(15)}
	if got := AchievedLevel(rec2); got != LevelStandard {
		t.Errorf("in_ground_uplight (directional, not outdoor-site) = %s, want standard", got)
	}
}

// TestLinearGate pins the linear conditional: a linear category makes
// per_length_normalized + declared_by_length hard standard requirements.
func TestLinearGate(t *testing.T) {
	rec := standardBase()
	rec["product_family"].(map[string]any)["primary_category"] = "linear"
	if got := AchievedLevel(rec); got != LevelCore {
		t.Fatalf("linear without per-length data = %s, want core", got)
	}
	phot := rec["photometry"].(map[string]any)
	phot["per_length_normalized"] = map[string]any{"lumens_per_meter": map[string]any{"value": float64(800)}}
	phot["declared_by_length"] = []any{map[string]any{"length_mm": float64(1219)}}
	if got := AchievedLevel(rec); got != LevelStandard {
		t.Errorf("linear with per-length data = %s, want standard", got)
	}
}

// TestWetOrExposedGate pins the IP conditional: a wet/outdoor fixture makes
// ip_rating a hard standard requirement; a damp indoor fixture does not.
func TestWetOrExposedGate(t *testing.T) {
	// Outdoor fixture without ip_rating -> core.
	rec := standardBase()
	rec["product_family"].(map[string]any)["indoor_outdoor"] = "outdoor"
	if got := AchievedLevel(rec); got != LevelCore {
		t.Errorf("outdoor without ip_rating = %s, want core", got)
	}
	// With ip_rating -> standard.
	rec["product_family"].(map[string]any)["shared_mechanical"].(map[string]any)["ip_rating"] = "IP65"
	if got := AchievedLevel(rec); got != LevelStandard {
		t.Errorf("outdoor with ip_rating = %s, want standard", got)
	}
	// Damp indoor fixture is NOT wet/exposed: no ip_rating required.
	rec2 := standardBase()
	rec2["product_family"].(map[string]any)["environment_rating"] = "damp"
	if got := AchievedLevel(rec2); got != LevelStandard {
		t.Errorf("damp indoor without ip_rating = %s, want standard (damp waives IP)", got)
	}
}

// TestRequiresDimmingDetailGate pins the analog/phase conditional: a 0-10V (or
// phase) driver makes dimming_method + dimming_range_percent hard standard
// requirements; digital (DMX/DALI), wireless, and non_dimming drivers are exempt
// because that detail is commanded externally or not conventionally published.
func TestRequiresDimmingDetailGate(t *testing.T) {
	// Analog (0-10V), missing the dimming spec -> core.
	rec := standardBase()
	delete(rec["electrical"].(map[string]any), "dimming_method")
	delete(rec["electrical"].(map[string]any), "dimming_range_percent")
	if got := AchievedLevel(rec); got != LevelCore {
		t.Errorf("analog driver without dimming spec = %s, want core", got)
	}
	// A DMX driver is exempt -> the same record grades standard.
	rec["electrical"].(map[string]any)["driver_protocol"] = "dmx_rdm"
	if got := AchievedLevel(rec); got != LevelStandard {
		t.Errorf("dmx driver without dimming spec = %s, want standard (exempt)", got)
	}
	// non_dimming is likewise exempt -> standard.
	rec["electrical"].(map[string]any)["driver_protocol"] = "non_dimming"
	if got := AchievedLevel(rec); got != LevelStandard {
		t.Errorf("non_dimming without dimming spec = %s, want standard", got)
	}
}

// --- roadmap + observations ---

// TestRoadmapNamesDocumentAndStandard confirms the per-grade roadmap: an incomplete
// record (missing only the core safety listing) emits a to-core delta plus to-standard
// and to-full deltas, and every gap carries the structured next-grade / source-document
// / standard detail.
func TestRoadmapNamesDocumentAndStandard(t *testing.T) {
	rec := coreBase()
	delete(rec["product_family"].(map[string]any), "shared_attestations")
	report := findings.NewReport()
	if got := Report(rec, report); got != LevelIncomplete {
		t.Fatalf("achieved = %s, want incomplete", got)
	}
	report.Finalize()
	gaps := findingsFor(report, findings.CodeConformanceGap)
	if len(gaps) == 0 {
		t.Fatal("expected roadmap gap findings, got none")
	}
	validNext := map[string]bool{"core": true, "standard": true, "full": true}
	coreGaps := 0
	for _, g := range gaps {
		if !validNext[g.NextConformanceLevel] {
			t.Errorf("gap %q next level = %q, want one of core/standard/full", g.Path, g.NextConformanceLevel)
		}
		if g.NextConformanceLevel == "core" {
			coreGaps++
		}
		if g.SourceDocument == "" || g.Standard == "" {
			t.Errorf("gap %q missing document/standard: %+v", g.Path, g)
		}
	}
	if coreGaps == 0 {
		t.Error("expected at least one to-core gap (the missing safety listing)")
	}
}

// TestCutsheetIsCoreRule confirms the cutsheet is a graded core requirement: a record
// otherwise core-complete but missing the cutsheet grades incomplete, and its to-core
// roadmap names /product_family/cutsheet.
func TestCutsheetIsCoreRule(t *testing.T) {
	rec := coreBase()
	delete(rec["product_family"].(map[string]any), "cutsheet")
	if got := AchievedLevel(rec); got != LevelIncomplete {
		t.Fatalf("no cutsheet = %s, want incomplete", got)
	}
	report := findings.NewReport()
	Report(rec, report)
	report.Finalize()
	named := false
	for _, g := range findingsFor(report, findings.CodeConformanceGap) {
		if g.Path == "/product_family/cutsheet" && g.NextConformanceLevel == "core" {
			named = true
		}
	}
	if !named {
		t.Error("to-core roadmap must name /product_family/cutsheet")
	}
}

// TestSatisfiedGradesReported confirms the satisfied-grade findings: a standard record
// reports core+standard satisfied; a full record reports all three and emits no
// roadmap; a core record reports only core satisfied.
func TestSatisfiedGradesReported(t *testing.T) {
	count := func(rec map[string]any, code findings.Code) int {
		report := findings.NewReport()
		Report(rec, report)
		report.Finalize()
		return len(findingsFor(report, code))
	}
	if got := count(standardBase(), findings.CodeConformanceGradeSatisfied); got != 2 {
		t.Errorf("standard record satisfied-grade findings = %d, want 2 (core, standard)", got)
	}
	if got := count(fullBase(), findings.CodeConformanceGradeSatisfied); got != 3 {
		t.Errorf("full record satisfied-grade findings = %d, want 3", got)
	}
	if got := count(fullBase(), findings.CodeConformanceGap); got != 0 {
		t.Errorf("full record gap findings = %d, want 0", got)
	}
	if got := count(coreBase(), findings.CodeConformanceGradeSatisfied); got != 1 {
		t.Errorf("core record satisfied-grade findings = %d, want 1 (core)", got)
	}
}

// TestGatedGradesReported pins the cascade case: a record carrying full standard- and
// full-grade data but missing only the cutsheet grades incomplete, emits NO satisfied-
// grade finding, a to-core roadmap naming the cutsheet, and a gated-grade finding for
// BOTH standard and full (their own requirements are met, gated by core).
func TestGatedGradesReported(t *testing.T) {
	rec := fullBase()
	delete(rec["product_family"].(map[string]any), "cutsheet")
	if got := AchievedLevel(rec); got != LevelIncomplete {
		t.Fatalf("full-data record missing cutsheet = %s, want incomplete", got)
	}
	report := findings.NewReport()
	Report(rec, report)
	report.Finalize()

	if sat := findingsFor(report, findings.CodeConformanceGradeSatisfied); len(sat) != 0 {
		t.Errorf("cascade record should emit no satisfied-grade findings, got %d", len(sat))
	}
	gated := map[string]bool{}
	for _, g := range findingsFor(report, findings.CodeConformanceGradeGated) {
		if strings.Contains(g.Message, `"standard"`) {
			gated["standard"] = true
		}
		if strings.Contains(g.Message, `"full"`) {
			gated["full"] = true
		}
		// The gated message must name the blocker grade to reach (the cascade reveal):
		// core is the only unmet grade, so every gated grade unlocks once core is reached.
		if !strings.Contains(g.Message, `unlocked once "core" is reached`) {
			t.Errorf("gated message should name the blocker grade to reach: %q", g.Message)
		}
	}
	if !gated["standard"] || !gated["full"] {
		t.Errorf("expected gated findings for standard AND full, got %v", gated)
	}
	coreGapNamesCutsheet := false
	for _, g := range findingsFor(report, findings.CodeConformanceGap) {
		if g.NextConformanceLevel == "core" && g.Path == "/product_family/cutsheet" {
			coreGapNamesCutsheet = true
		}
		if g.NextConformanceLevel != "core" {
			t.Errorf("cascade record should have only to-core gaps, got %q at %q", g.NextConformanceLevel, g.Path)
		}
	}
	if !coreGapNamesCutsheet {
		t.Error("to-core roadmap must name /product_family/cutsheet")
	}
}

// TestGatedNamesIntermediateBlocker pins the second blocker path: a record that meets
// core and every full requirement but is missing one STANDARD row achieves core, and
// full (its own rows all met) is gated by standard, so the gated message names standard
// (not core) as the grade to reach.
func TestGatedNamesIntermediateBlocker(t *testing.T) {
	rec := fullBase()
	delete(rec["photometry"].(map[string]any), "maximum_intensity_cd") // a standard rule
	if got := AchievedLevel(rec); got != LevelCore {
		t.Fatalf("achieved = %s, want core", got)
	}
	report := findings.NewReport()
	Report(rec, report)
	report.Finalize()
	found := false
	for _, g := range findingsFor(report, findings.CodeConformanceGradeGated) {
		if strings.Contains(g.Message, `"full"`) {
			found = true
			if !strings.Contains(g.Message, `unlocked once "standard" is reached`) {
				t.Errorf("full gated message should name standard as the blocker: %q", g.Message)
			}
		}
	}
	if !found {
		t.Error("expected full to be gated (by standard) when only a standard row is missing")
	}
}

// TestGatedBlockerIsHighestDelta pins the multi-delta case: when an incomplete record
// has BOTH a core gap and a standard gap while every full requirement is met, full is
// gated and its blocker must be the HIGHEST outstanding delta grade (standard), not the
// lowest (core). Reaching core alone leaves standard's gap open, so full stays locked;
// naming core would mislead.
func TestGatedBlockerIsHighestDelta(t *testing.T) {
	rec := fullBase()
	delete(rec["product_family"].(map[string]any), "cutsheet")         // a core rule
	delete(rec["photometry"].(map[string]any), "maximum_intensity_cd") // a standard rule
	if got := AchievedLevel(rec); got != LevelIncomplete {
		t.Fatalf("achieved = %s, want incomplete", got)
	}
	report := findings.NewReport()
	Report(rec, report)
	report.Finalize()
	found := false
	for _, g := range findingsFor(report, findings.CodeConformanceGradeGated) {
		if strings.Contains(g.Message, `"full"`) {
			found = true
			if !strings.Contains(g.Message, `unlocked once "standard" is reached`) {
				t.Errorf("full's blocker should be standard (the highest delta grade), got: %q", g.Message)
			}
		}
	}
	if !found {
		t.Error("expected full to be gated when core+standard carry deltas but full's own rows are met")
	}
}

// TestRoadmapPerTierToFull confirms the per-grade decomposition: a core record emits
// both a to-standard and a to-full roadmap whose deltas are disjoint (no path repeats);
// a standard record emits only a to-full roadmap.
func TestRoadmapPerTierToFull(t *testing.T) {
	report := findings.NewReport()
	Report(coreBase(), report)
	report.Finalize()
	std, full := map[string]bool{}, map[string]bool{}
	for _, g := range findingsFor(report, findings.CodeConformanceGap) {
		switch g.NextConformanceLevel {
		case "standard":
			std[g.Path] = true
		case "full":
			full[g.Path] = true
		default:
			t.Errorf("core record gap at unexpected grade %q (%q)", g.NextConformanceLevel, g.Path)
		}
	}
	if len(std) == 0 || len(full) == 0 {
		t.Fatalf("core record should emit both to-standard (%d) and to-full (%d) deltas", len(std), len(full))
	}
	for p := range std {
		if full[p] {
			t.Errorf("path %q appears in BOTH to-standard and to-full deltas; must be disjoint", p)
		}
	}

	report2 := findings.NewReport()
	Report(standardBase(), report2)
	report2.Finalize()
	for _, g := range findingsFor(report2, findings.CodeConformanceGap) {
		if g.NextConformanceLevel != "full" {
			t.Errorf("standard record should emit only to-full gaps, got %q at %q", g.NextConformanceLevel, g.Path)
		}
	}
}

// TestZeroDocumentRecordGradesIncomplete pins the floor's worked case: an identity-only
// record (no photometry, no electrical, no documents) grades incomplete, emits only INFO
// findings, and carries a to-core roadmap.
func TestZeroDocumentRecordGradesIncomplete(t *testing.T) {
	rec := map[string]any{
		"product_family": map[string]any{
			"family_id":     "acme-orbit",
			"manufacturer":  map[string]any{"slug": "acme", "display_name": "Acme Lighting"},
			"catalog_model": "Orbit 1200",
		},
		"source_files": []any{},
	}
	report := findings.NewReport()
	if got := Report(rec, report); got != LevelIncomplete {
		t.Fatalf("identity-only record = %s, want incomplete", got)
	}
	report.Finalize()
	if report.Summary.Errors != 0 || report.Summary.Warnings != 0 {
		t.Errorf("floor record should be INFO-only, got %d errors, %d warnings", report.Summary.Errors, report.Summary.Warnings)
	}
	if gaps := findingsFor(report, findings.CodeConformanceGap); len(gaps) == 0 {
		t.Error("identity-only record must carry a to-core roadmap")
	}
}

// TestObservationsAtCoreAndAbove is the three-way split guard for the reclassified
// depth band: at core, the enrichment rows (thermal, Duv) now emit under
// conformance/enrichment with byte-identical messages, while the retained notes
// (sustainability, legacy cutoff) stay under conformance/observation; an incomplete
// record emits ZERO of BOTH codes (including from the two direct note emitters),
// pinning the single core gate.
func TestObservationsAtCoreAndAbove(t *testing.T) {
	// Core white-light record.
	report := findings.NewReport()
	Report(coreBase(), report)
	report.Finalize()

	// The reclassified rows emit as enrichment, with the exact prior message text.
	wantEnrichment := map[string]string{
		"/thermal_derating": "thermal derating not disclosed",
		"/colorimetry/duv":  "Duv (distance from the Planckian locus) not disclosed",
	}
	for path, msg := range wantEnrichment {
		f, ok := enrichmentAt(report, path)
		if !ok {
			t.Errorf("core record: expected enrichment at %s; enrichment: %+v",
				path, findingsFor(report, findings.CodeConformanceEnrichment))
			continue
		}
		if f.Message != msg {
			t.Errorf("enrichment %s message = %q, want byte-identical %q", path, f.Message, msg)
		}
		if hasObservationAt(report, path) {
			t.Errorf("reclassified row %s must no longer emit conformance/observation", path)
		}
	}

	// The retained note (sustainability) stays an observation.
	if !hasObservationAt(report, "/sustainability_declaration") {
		t.Errorf("core record: sustainability must stay an observation; observations: %+v",
			findingsFor(report, findings.CodeConformanceObservation))
	}
	if _, ok := enrichmentAt(report, "/sustainability_declaration"); ok {
		t.Error("sustainability must NOT appear on the enrichment roadmap")
	}

	// Incomplete record: ZERO enrichment AND zero observations. This pins the single
	// core gate covering the rubric rows AND the two direct note emitters (the
	// attestation-coverage note would otherwise leak on an incomplete record).
	inc := coreBase()
	delete(inc["product_family"].(map[string]any), "shared_attestations")
	report2 := findings.NewReport()
	if got := Report(inc, report2); got != LevelIncomplete {
		t.Fatalf("achieved = %s, want incomplete", got)
	}
	report2.Finalize()
	if enr := findingsFor(report2, findings.CodeConformanceEnrichment); len(enr) != 0 {
		t.Errorf("incomplete record emitted %d enrichment findings, want 0: %+v", len(enr), enr)
	}
	if obs := findingsFor(report2, findings.CodeConformanceObservation); len(obs) != 0 {
		t.Errorf("incomplete record emitted %d observations, want 0 (note emitters must sit inside the core gate): %+v", len(obs), obs)
	}

	// JSON always carries the enrichment block at core+ regardless of Verbose.
	jbuf := &bytes.Buffer{}
	jreport := findings.NewReport()
	Report(coreBase(), jreport)
	jreport.Finalize()
	if err := jreport.WriteJSON(jbuf); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	if !strings.Contains(jbuf.String(), "conformance/enrichment") {
		t.Errorf("JSON must always carry the enrichment block at core+:\n%s", jbuf.String())
	}
}

// TestEnrichmentRowsFireOnPresentParentOnly pins the blockPresent-gated enrichment
// rows: a sub-field nudge fires only when its parent block is genuinely present
// (non-empty) and the sub-field absent; a hollow ({}) or absent parent draws no
// sub-field spam.
func TestEnrichmentRowsFireOnPresentParentOnly(t *testing.T) {
	emit := func(rec map[string]any) *findings.Report {
		r := findings.NewReport()
		Report(rec, r)
		r.Finalize()
		return r
	}

	// coreBase has a populated photometry block missing beam_family -> fires.
	if _, ok := enrichmentAt(emit(coreBase()), "/photometry/beam_family"); !ok {
		t.Error("beam_family enrichment should fire: photometry present, beam_family absent")
	}

	// test_conditions absent on coreBase -> no test-condition sub-field rows.
	base := emit(coreBase())
	for _, p := range []string{"/test_conditions/photometry_method", "/test_conditions/stabilization_method"} {
		if _, ok := enrichmentAt(base, p); ok {
			t.Errorf("%s must not fire when test_conditions is absent", p)
		}
	}

	// A hollow test_conditions:{} still draws no sub-field spam (blockPresent excludes it).
	hollow := coreBase()
	hollow["test_conditions"] = map[string]any{}
	rH := emit(hollow)
	for _, p := range []string{"/test_conditions/photometry_method", "/test_conditions/file_generation_type"} {
		if _, ok := enrichmentAt(rH, p); ok {
			t.Errorf("%s must not fire on a hollow test_conditions:{}", p)
		}
	}

	// A present, non-empty test_conditions missing the sub-field DOES fire.
	present := coreBase()
	present["test_conditions"] = map[string]any{"photometry_basis": "absolute"}
	if _, ok := enrichmentAt(emit(present), "/test_conditions/photometry_method"); !ok {
		t.Error("photometry_method enrichment should fire on a present test_conditions missing it")
	}

	// A test_conditions that HAS the sub-field does NOT fire it.
	filled := coreBase()
	filled["test_conditions"] = map[string]any{"photometry_method": "goniophotometer"}
	if _, ok := enrichmentAt(emit(filled), "/test_conditions/photometry_method"); ok {
		t.Error("photometry_method enrichment must not fire when the field is present")
	}
}

// TestArrItemEnrichmentAnyEntry pins arrItemHas semantics: ANY array entry carrying
// the field satisfies the row (the nudge is "start disclosing this dimension"). A
// lumen_maintenance_package whose one entry carries tested_product_type but not
// flux_maintenance_quantity fires the flux row and not the tested-product-type row.
func TestArrItemEnrichmentAnyEntry(t *testing.T) {
	rec := standardBase()
	rec["lumen_maintenance_package"] = []any{
		map[string]any{"tested_product_type": "led_package"},
	}
	report := findings.NewReport()
	Report(rec, report)
	report.Finalize()
	if _, ok := enrichmentAt(report, "/lumen_maintenance_package[]/flux_maintenance_quantity"); !ok {
		t.Error("flux_maintenance_quantity enrichment should fire: package present, field absent in every entry")
	}
	if _, ok := enrichmentAt(report, "/lumen_maintenance_package[]/tested_product_type"); ok {
		t.Error("tested_product_type enrichment must NOT fire: an entry carries it")
	}
}

// TestOperatingPointEnrichmentWindow pins the operating_point sub-field window: the
// dut_operating_mode / monitoring-point rows fire only when operating_point is present
// (hasOperatingPoint) but the sub-field absent. hasOperatingPoint counts
// dut_operating_mode itself as a satisfying qualifier, so the fixture makes
// operating_point present via a DIFFERENT qualifier (drive_current_ma) while
// dut_operating_mode is absent.
func TestOperatingPointEnrichmentWindow(t *testing.T) {
	rec := coreBase()
	rec["operating_point"] = map[string]any{
		"drive_current_ma": map[string]any{"value": float64(350)},
	}
	report := findings.NewReport()
	Report(rec, report)
	report.Finalize()
	for _, p := range []string{"/operating_point/dut_operating_mode", "/operating_point/case_temperature_monitoring_point"} {
		if _, ok := enrichmentAt(report, p); !ok {
			t.Errorf("%s enrichment should fire: operating_point present via drive_current_ma, sub-field absent", p)
		}
	}

	// operating_point absent -> neither fires (no hasOperatingPoint).
	report2 := findings.NewReport()
	Report(coreBase(), report2)
	report2.Finalize()
	if _, ok := enrichmentAt(report2, "/operating_point/dut_operating_mode"); ok {
		t.Error("dut_operating_mode must not fire when operating_point is absent")
	}
}

// TestAdaptiveLightingModeControllableDriver pins the controllableDriver applicability
// with a positive AND a negative fixture, so an empty or wrong token set is caught
// rather than silently no-op: a controllable driver_protocol (0-10v) fires the
// AdaptiveLightingMode row; non_dimming waives it; declared modes suppress it.
func TestAdaptiveLightingModeControllableDriver(t *testing.T) {
	fires := func(rec map[string]any) bool {
		report := findings.NewReport()
		Report(rec, report)
		report.Finalize()
		_, ok := enrichmentAt(report, "/electrical/adaptive_lighting_modes")
		return ok
	}
	// Positive: coreBase carries a controllable 0-10v driver and no adaptive modes.
	if !fires(coreBase()) {
		t.Error("adaptive_lighting_modes enrichment should fire for a controllable (0-10v) driver")
	}
	// Negative: a non_dimming driver waives the row.
	nd := coreBase()
	nd["electrical"].(map[string]any)["driver_protocol"] = "non_dimming"
	if fires(nd) {
		t.Error("adaptive_lighting_modes enrichment must NOT fire for a non_dimming driver")
	}
	// Declared modes suppress the nudge even for a controllable driver.
	filled := coreBase()
	filled["electrical"].(map[string]any)["adaptive_lighting_modes"] = []any{"integrated_photocell"}
	if fires(filled) {
		t.Error("adaptive_lighting_modes enrichment must NOT fire when modes are declared")
	}
}

// TestPvfCodeCompoundApplicability pins the pvf_code row's AND predicate: it fires only
// when the fixture is white-light-primary AND a tm_30 block is present (but pvf_code
// absent). A present pvf_code, an absent tm_30 block, or a color-mixing fixture waives it.
func TestPvfCodeCompoundApplicability(t *testing.T) {
	fires := func(rec map[string]any) bool {
		report := findings.NewReport()
		Report(rec, report)
		report.Finalize()
		_, ok := enrichmentAt(report, "/colorimetry/tm_30/pvf_code")
		return ok
	}
	// White-light fixture with a tm_30 block (rf) but no pvf_code -> fires.
	rec := coreBase()
	rec["colorimetry"].(map[string]any)["tm_30"] = map[string]any{"rf": map[string]any{"value": float64(90)}}
	if !fires(rec) {
		t.Error("pvf_code enrichment should fire: white-light-primary, tm_30 present, pvf_code absent")
	}
	// pvf_code present -> suppressed.
	rec2 := coreBase()
	rec2["colorimetry"].(map[string]any)["tm_30"] = map[string]any{"pvf_code": "P2"}
	if fires(rec2) {
		t.Error("pvf_code enrichment must not fire when pvf_code is present")
	}
	// No tm_30 block -> waived (blockPresent false).
	if fires(coreBase()) {
		t.Error("pvf_code enrichment must not fire when tm_30 is absent")
	}
	// Color-mixing fixture (rgb) with a tm_30 block -> waived (not white-light-primary).
	rgb := coreBase()
	rgb["configuration"].(map[string]any)["tested_axes"].(map[string]any)["color_tunability"] = "rgb"
	delete(rgb["colorimetry"].(map[string]any), "nominal_cct_k")
	delete(rgb["colorimetry"].(map[string]any), "cri_ra")
	rgb["colorimetry"].(map[string]any)["tm_30"] = map[string]any{"rf": map[string]any{"value": float64(90)}}
	if fires(rgb) {
		t.Error("pvf_code enrichment must not fire for a color-mixing (rgb) fixture")
	}
}

// TestEnrichmentFieldsDoNotGate is the level-invariance guard: populating any
// enrichment-only field or block on a graded record must NOT change AchievedLevel.
// This pins the release's central compatibility promise (grades and
// index.conformance_level do not move) at the field level, and catches a future edit
// that accidentally moved an enrichment field into the gating walk or gave a row a
// gating level. It complements TestPredicatesReadOnlyCoreFields, which only proves the
// gating predicates read core fields (not that a populated enrichment field can lift a
// grade).
func TestEnrichmentFieldsDoNotGate(t *testing.T) {
	pf := func(r map[string]any) map[string]any { return r["product_family"].(map[string]any) }
	phot := func(r map[string]any) map[string]any { return r["photometry"].(map[string]any) }
	el := func(r map[string]any) map[string]any { return r["electrical"].(map[string]any) }
	col := func(r map[string]any) map[string]any { return r["colorimetry"].(map[string]any) }
	// setTM adds a key to colorimetry.tm_30 without clobbering an existing block (fullBase
	// carries a real tm_30 that the level depends on).
	setTM := func(r map[string]any, k string, v any) {
		c := col(r)
		tm, ok := c["tm_30"].(map[string]any)
		if !ok {
			tm = map[string]any{}
			c["tm_30"] = tm
		}
		tm[k] = v
	}

	mutations := []struct {
		name string
		add  func(map[string]any)
	}{
		// The 5 additive Phase-D optional fields.
		{"orientation", func(r map[string]any) { pf(r)["orientation"] = "downward" }},
		{"optical_radiation_band", func(r map[string]any) { phot(r)["optical_radiation_band"] = "visible" }},
		{"adaptive_lighting_modes", func(r map[string]any) { el(r)["adaptive_lighting_modes"] = []any{"networked_control"} }},
		{"photometry_format", func(r map[string]any) {
			r["source_files"] = []any{map[string]any{"file_type": "ies", "photometry_format": "lm63_2019"}}
		}},
		{"reference_illuminant_type", func(r map[string]any) { setTM(r, "reference_illuminant_type", "planckian") }},
		{"pvf_code", func(r map[string]any) { setTM(r, "pvf_code", "P2") }},
		// Representative reclassified + Bucket-B enrichment fields/blocks across blocks.
		{"power_factor", func(r map[string]any) { el(r)["power_factor"] = map[string]any{"value": float64(0.95)} }},
		{"thermal_derating", func(r map[string]any) { r["thermal_derating"] = map[string]any{"thermal_control_method": "active_fan"} }},
		{"flicker_measurements", func(r map[string]any) { r["flicker_measurements"] = map[string]any{"risk_level": "no_risk"} }},
		{"chromaticity_shift_projection", func(r map[string]any) { r["chromaticity_shift_projection"] = map[string]any{"shift_metric": "duv"} }},
		{"dimming_curve", func(r map[string]any) { el(r)["dimming_curve"] = "logarithmic" }},
		{"beam_family", func(r map[string]any) { phot(r)["beam_family"] = "spot" }},
		{"duv", func(r map[string]any) { col(r)["duv"] = map[string]any{"value": float64(0.001)} }},
		{"compatible_accessories", func(r map[string]any) {
			r["compatible_accessories"] = []any{map[string]any{"accessory_type": "louver"}}
		}},
	}

	bases := []struct {
		name string
		make func() map[string]any
	}{
		{"core", coreBase},
		{"standard", standardBase},
		{"full", fullBase},
	}
	for _, b := range bases {
		want := AchievedLevel(b.make())
		for _, m := range mutations {
			rec := b.make()
			m.add(rec)
			if got := AchievedLevel(rec); got != want {
				t.Errorf("%s base + enrichment mutation %q changed level %s -> %s (enrichment must never gate)", b.name, m.name, want, got)
			}
		}
	}
}

// TestComputeContract exercises the Compute() entry point directly (not only through
// Report): it is pure and UNCONDITIONAL (an incomplete record still yields the full
// Enrichment slice, while render gates emission behind core), its TierRoadmap carries
// real NextLevel values whose messages match the emitted conformance/gap findings (the
// shared roadmapMessage helper), and Result.Observations holds only the rubric
// observation rows (not the two note emitters).
func TestComputeContract(t *testing.T) {
	inc := coreBase()
	delete(inc["product_family"].(map[string]any), "shared_attestations")

	res := Compute(inc)
	if res.Level != LevelIncomplete {
		t.Fatalf("Compute(incomplete).Level = %s, want incomplete", res.Level)
	}
	if len(res.Enrichment) == 0 {
		t.Error("Compute is ungated: an incomplete record must still yield Enrichment items")
	}
	if len(res.TierRoadmap) == 0 {
		t.Fatal("Compute(incomplete).TierRoadmap must carry the to-core (and beyond) gaps")
	}

	// Report/render gates the same incomplete record: zero enrichment emitted.
	report := findings.NewReport()
	Report(inc, report)
	report.Finalize()
	if n := len(findingsFor(report, findings.CodeConformanceEnrichment)); n != 0 {
		t.Errorf("Report(incomplete) emitted %d enrichment findings, want 0 (render gates behind core)", n)
	}

	// Every TierRoadmap item carries a real NextLevel, and its message is byte-identical
	// to the emitted conformance/gap finding for the same (level, path).
	gapMsg := map[string]string{}
	for _, f := range findingsFor(report, findings.CodeConformanceGap) {
		gapMsg[f.NextConformanceLevel+"|"+f.Path] = f.Message
	}
	for _, it := range res.TierRoadmap {
		if it.NextLevel != LevelCore && it.NextLevel != LevelStandard && it.NextLevel != LevelFull {
			t.Errorf("TierRoadmap item %q has NextLevel %s, want a real tier", it.Path, it.NextLevel)
		}
		key := it.NextLevel.String() + "|" + it.Path
		if gapMsg[key] != it.Message {
			t.Errorf("TierRoadmap message for %s diverges from the emitted gap: %q vs %q", key, it.Message, gapMsg[key])
		}
	}

	// Result.Observations carries ONLY rubric observation rows (the two note emitters are
	// appended by render, not part of the slice).
	core := Compute(coreBase())
	for _, it := range core.Observations {
		if it.Path != "/sustainability_declaration" && it.Path != "/outdoor_classification/legacy_cutoff" {
			t.Errorf("Result.Observations contains unexpected path %q (rubric observation rows only)", it.Path)
		}
	}
}

// TestReclassifiedEnrichmentMessagesByteIdentical pins ALL 20 rows reclassified from
// observation to enrichment against their exact message text: the reclassification
// changed only the finding code, and the CHANGELOG promises byte-identical wording, so
// a typo introduced into any of the 20 during the move must fail here.
func TestReclassifiedEnrichmentMessagesByteIdentical(t *testing.T) {
	want := map[string]string{
		"/electrical/power_factor":                                  "full records commonly disclose power factor; absent here",
		"/product_family/shared_warranty/term_years":                "warranty term not disclosed",
		"/photometry/luminous_opening_shape":                        "luminous opening shape not disclosed",
		"/photometry/emission_face":                                 "emission face not disclosed",
		"/colorimetry/duv":                                          "Duv (distance from the Planckian locus) not disclosed",
		"/colorimetry/chromaticity_x":                               "chromaticity x not disclosed",
		"/colorimetry/chromaticity_y":                               "chromaticity y not disclosed",
		"/product_family/shared_mechanical/ambient_operating_range": "ambient operating range not disclosed",
		"/compatible_accessories":                                   "compatible accessories not listed",
		"/thermal_derating":                                         "thermal derating not disclosed",
		"/flicker_measurements":                                     "flicker measurements not disclosed",
		"/alpha_opic_metrics":                                       "alpha-opic (circadian) metrics not disclosed",
		"/chromaticity_shift_projection":                            "chromaticity-shift projection not disclosed",
		"/photometry/field_angle_deg":                               "field angle not disclosed",
		"/photometry/cutoff_angle_from_horizontal_deg":              "cutoff angle not disclosed",
		"/photometry/spacing_criterion":                             "spacing criterion not disclosed",
		"/photometry/ugr_4h_8h":                                     "UGR not disclosed",
		"/outdoor_classification/lcs_zonal_lumens":                  "LCS zonal lumens not disclosed",
		"/product_family/shared_mechanical/ik_rating":               "impact (IK) rating not disclosed",
		"/product_family/physical_dimensions/epa":                   "EPA (effective projected area) not disclosed",
	}
	if len(want) != 20 {
		t.Fatalf("test table has %d rows, want 20", len(want))
	}
	seen := map[string]bool{}
	for _, ru := range rubric {
		if ru.level != LevelEnrichment {
			continue
		}
		if w, ok := want[ru.path]; ok {
			seen[ru.path] = true
			if ru.message != w {
				t.Errorf("reclassified row %s message = %q, want byte-identical %q", ru.path, ru.message, w)
			}
		}
	}
	for path := range want {
		if !seen[path] {
			t.Errorf("reclassified path %s is not present as a LevelEnrichment rubric row", path)
		}
	}
}

// TestPhotometryFormatEnrichment pins the Phase-D PhotometryFormat row and its
// hasPhotometrySourceFile applicability: it fires when a photometric source file
// (ies/ldt/tm33) lacks a format, is waived when only a non-photometric file (or none)
// is present, and is suppressed once any source file carries a format.
func TestPhotometryFormatEnrichment(t *testing.T) {
	fires := func(rec map[string]any) bool {
		report := findings.NewReport()
		Report(rec, report)
		report.Finalize()
		_, ok := enrichmentAt(report, "/source_files[]/photometry_format")
		return ok
	}
	for _, ft := range []string{"ies", "ldt", "tm33"} {
		r := coreBase()
		r["source_files"] = []any{map[string]any{"file_type": ft}}
		if !fires(r) {
			t.Errorf("photometry_format should fire for a %s source file lacking a format", ft)
		}
	}
	noPhot := coreBase()
	noPhot["source_files"] = []any{map[string]any{"file_type": "datasheet_pdf"}}
	if fires(noPhot) {
		t.Error("photometry_format must be waived when no ies/ldt/tm33 source file is present")
	}
	absent := coreBase()
	delete(absent, "source_files")
	if fires(absent) {
		t.Error("photometry_format must be waived when source_files is absent")
	}
	filled := coreBase()
	filled["source_files"] = []any{map[string]any{"file_type": "ies", "photometry_format": "lm63_2019"}}
	if fires(filled) {
		t.Error("photometry_format must not fire once a source file carries a format")
	}
}

// TestCie97LmfTableEnrichment pins the block-level AmbientCleanliness row and its
// hasCie97LmfTable closure: it fires when a luminaire framework is present without a
// populated CIE 97 grid, is waived when no luminaire framework exists, is suppressed
// once a real grid is present, and still fires on a hollow (empty-grid) table.
func TestCie97LmfTableEnrichment(t *testing.T) {
	fires := func(rec map[string]any) bool {
		report := findings.NewReport()
		Report(rec, report)
		report.Finalize()
		_, ok := enrichmentAt(report, "/lumen_maintenance_luminaire/cie_97_lmf_table")
		return ok
	}
	if !fires(standardBase()) {
		t.Error("cie_97_lmf_table should fire: luminaire framework present, no CIE 97 table")
	}
	if fires(coreBase()) {
		t.Error("cie_97_lmf_table must be waived when no luminaire framework is present")
	}
	populated := standardBase()
	populated["lumen_maintenance_luminaire"].(map[string]any)["cie_97_lmf_table"] = map[string]any{
		"llmf_by_hours": []any{map[string]any{"hours": float64(6000), "llmf": float64(0.95)}},
	}
	if fires(populated) {
		t.Error("cie_97_lmf_table must not fire once a populated grid is present")
	}
	hollow := standardBase()
	hollow["lumen_maintenance_luminaire"].(map[string]any)["cie_97_lmf_table"] = map[string]any{"llmf_by_hours": []any{}}
	if !fires(hollow) {
		t.Error("cie_97_lmf_table should still fire on a hollow table (empty grids read as absent)")
	}
}

// TestHeadlineProvenanceObservation confirms a record whose headline photometry is
// not "measured" surfaces the non-measured-headline note, and that the derived
// efficacy value is excluded.
func TestHeadlineProvenanceObservation(t *testing.T) {
	rec := coreBase()
	phot := rec["photometry"].(map[string]any)
	phot["total_luminous_flux_lm"].(map[string]any)["value_type"] = "rated"
	// efficacy is rated too, but it must NOT surface (derived quantity).
	phot["luminaire_efficacy_lm_per_w"].(map[string]any)["value_type"] = "rated"
	report := findings.NewReport()
	Report(rec, report)
	report.Finalize()
	if !hasObservationAt(report, "/photometry/total_luminous_flux_lm/value_type") {
		t.Errorf("expected non-measured headline note on total_luminous_flux_lm; observations: %+v",
			findingsFor(report, findings.CodeConformanceObservation))
	}
	if hasObservationAt(report, "/photometry/luminaire_efficacy_lm_per_w/value_type") {
		t.Error("efficacy value_type must be excluded from the non-measured headline note")
	}
}

// TestAttestationCoverageObservation confirms the attestation-coverage note lists
// the record's programs, or notes none.
func TestAttestationCoverageObservation(t *testing.T) {
	report := findings.NewReport()
	Report(coreBase(), report)
	report.Finalize()
	var found bool
	for _, f := range findingsFor(report, findings.CodeConformanceObservation) {
		if f.Path == "/attestations" && strings.Contains(f.Message, "ul_listed") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected attestation-coverage note listing ul_listed; observations: %+v",
			findingsFor(report, findings.CodeConformanceObservation))
	}
}

// --- determinism + panic-safety ---

// TestPredicatesReadOnlyCoreFields asserts the GATING-row applicability predicates
// read only core fields: stripping every standard/full field leaves each predicate's
// value unchanged. coreBase and fullBase share identical core fields, so every gating
// row's predicate must agree across them, for a neutral, an outdoor-site, and a
// directional fixture. Rubric-driven so it automatically covers every gating row and
// naturally EXEMPTS enrichment/observation rows (which may read parent-block presence
// because they never affect the level; see the applicability-predicates note in
// completeness.go). This replaces the former hand-enumerated predicate map.
func TestPredicatesReadOnlyCoreFields(t *testing.T) {
	check := func(t *testing.T, core, full map[string]any) {
		for _, ru := range rubric {
			switch ru.level {
			case LevelCore, LevelStandard, LevelFull:
			default:
				continue // enrichment / observation rows are non-gating and exempt
			}
			if ru.applicable == nil {
				continue // universal
			}
			if ru.applicable(core) != ru.applicable(full) {
				t.Errorf("gating row (level %s, path %q) applicability differs between core and full fixtures (reads a non-core field): core=%v full=%v",
					ru.level, ru.path, ru.applicable(core), ru.applicable(full))
			}
		}
	}
	t.Run("neutral", func(t *testing.T) { check(t, coreBase(), fullBase()) })
	t.Run("outdoor-site", func(t *testing.T) {
		oc := coreBase()
		oc["product_family"].(map[string]any)["primary_category"] = "flood_area_site"
		oc["product_family"].(map[string]any)["indoor_outdoor"] = "outdoor"
		of := fullBase()
		of["product_family"].(map[string]any)["primary_category"] = "flood_area_site"
		of["product_family"].(map[string]any)["indoor_outdoor"] = "outdoor"
		check(t, oc, of)
	})
	t.Run("directional", func(t *testing.T) {
		dc := coreBase()
		dc["product_family"].(map[string]any)["primary_category"] = "downlight"
		df := fullBase()
		df["product_family"].(map[string]any)["primary_category"] = "downlight"
		check(t, dc, df)
	})

	// v0.10.0 class pairs (§2.10): the neutral troffer makes every class predicate false
	// on both sides, so it cannot catch a class predicate that wrongly reads a non-core
	// field. Each pair below is a §5.2 ladder's own core-vs-full rung, so the full side
	// carries the class's standard+full fields and a predicate reading one genuinely
	// flips. The internally-illuminated and emergency-luminaire pairs author the emergency
	// block (integral_battery) on BOTH sides; the three unpowered-mode sign pairs author
	// no emergency block. The neutral bases are left untouched (bolting a block on would
	// fire the enrichment nudges on every neutral-fixture test).
	t.Run("sign-internally-illuminated", func(t *testing.T) { check(t, comboSignCore(), comboSignFull()) })
	t.Run("sign-externally-illuminated", func(t *testing.T) { check(t, externalSignCore(), externalSignFull()) })
	// P1 regression (§2.9): an external sign whose full side ALSO carries a battery block.
	// power_source is non-core for an external sign, so the battery-trio gate must read only
	// the power-source-core class. Before the fix, hasIntegralBattery differed across this
	// pair and the battery-trio gate's applicability flipped, failing this check.
	t.Run("sign-external-combo-battery", func(t *testing.T) { check(t, externalSignCore(), externalComboSignFull()) })
	t.Run("sign-photoluminescent", func(t *testing.T) { check(t, photoSignCore(), photoSignFull()) })
	t.Run("sign-self-luminous", func(t *testing.T) { check(t, tritiumSignCore(), tritiumSignFull()) })
	t.Run("emergency-luminaire", func(t *testing.T) { check(t, emgLuminaireCore(), emgLuminaireFull()) })

	// Non-vacuity (§2.10 req 3): each new gating predicate must be TRUE on at least one
	// pair's CORE side. A predicate false on every core side is a silent vacuity bug that
	// the applicable(core)==applicable(full) equality alone would not catch.
	t.Run("non-vacuity", func(t *testing.T) {
		checks := []struct {
			level Level
			path  string
			rec   map[string]any
		}{
			{LevelCore, "/emergency/power_source", comboSignCore()},
			{LevelCore, "/emergency/power_source", emgLuminaireCore()},
			{LevelStandard, "/emergency/battery_duration_min", comboSignCore()},
			{LevelStandard, "/emergency/battery_duration_min", emgLuminaireCore()},
			{LevelStandard, "/electrical/input_power_w", comboSignCore()},
			{LevelCore, "/exit_sign/illumination_mode", comboSignCore()},
			{LevelCore, "/exit_sign/legend_color", externalSignCore()},
			{LevelStandard, "/exit_sign/legend_height", externalSignCore()},
			{LevelStandard, "/exit_sign/face_illuminance_lx", externalSignCore()},
			{LevelStandard, "/exit_sign/contrast_ratio", externalSignCore()},
			{LevelStandard, "/exit_sign/sign_face_luminance_cd_per_m2", photoSignCore()},
			{LevelStandard, "/exit_sign/min_charging_illuminance_lx", photoSignCore()},
			{LevelStandard, "/exit_sign/sign_face_luminance_cd_per_m2", tritiumSignCore()},
			{LevelStandard, "/exit_sign/tritium_rated_life_years", tritiumSignCore()},
			{LevelFull, "test-report-backed sign-face luminance", comboSignCore()},
			{LevelFull, "test-report-backed sign-face luminance", photoSignCore()},
			{LevelFull, "test-report-backed sign-face luminance", tritiumSignCore()},
			{LevelFull, "test-report-backed face illuminance", externalSignCore()},
			{LevelCore, "UL 924 listing", comboSignCore()},              // naDedicatedClass on an NA sign
			{LevelCore, "/photometry/luminaire_efficacy_lm_per_w", coreBase()}, // notDedicatedClass on a troffer
			{LevelCore, "/product_family/secondary_function", emgLuminaireCore()}, // notExitSign on an emergency luminaire
		}
		for _, c := range checks {
			if !applicableTo(t, c.level, c.path, c.rec) {
				t.Errorf("non-vacuity: row (level %s, path %q) is NOT applicable to its intended core-side fixture (silent vacuity bug)", c.level, c.path)
			}
		}
	})
}

// TestGradeHostileInput substitutes wrong-typed values at every path the grader
// reads and asserts AchievedLevel and Report never panic. The grader runs before
// full schema validation on the from-sheet path, so it must tolerate any shape.
func TestGradeHostileInput(t *testing.T) {
	hostiles := []any{map[string]any{}, []any{}, nil, "x", 1.0, []any{1.0, 2.0}}
	// (parent-path, leaf-key) pairs. An empty parent means a top-level key.
	targets := []struct {
		parent []string
		key    string
	}{
		{nil, "product_family"}, {[]string{"product_family"}, "shared_mechanical"},
		{[]string{"product_family"}, "shared_warranty"}, {[]string{"product_family", "shared_warranty"}, "term_years"},
		{[]string{"product_family"}, "physical_dimensions"}, {[]string{"product_family"}, "mounting_types"},
		{[]string{"product_family"}, "manufacturer"}, {nil, "electrical"},
		{[]string{"electrical"}, "dimming_range_percent"}, {nil, "configuration"},
		{[]string{"configuration"}, "tested_axes"}, {nil, "colorimetry"},
		{[]string{"colorimetry"}, "tm_30"}, {nil, "photometry"}, {[]string{"photometry"}, "zonal_lumens"},
		{nil, "corrections_applied"}, {nil, "instrumentation"}, {nil, "outdoor_classification"},
		{nil, "uncertainty"}, {nil, "operating_point"}, {nil, "lumen_maintenance_package"},
		{nil, "lumen_maintenance_luminaire"}, {nil, "attestations"}, {nil, "compatible_accessories"},
		{nil, "test_conditions"}, {nil, "thermal_derating"}, {nil, "flicker_measurements"},
		{nil, "alpha_opic_metrics"}, {nil, "chromaticity_shift_projection"}, {nil, "sustainability_declaration"},
		// New Phase C array / sub-block parents the enrichment closures descend into.
		{[]string{"thermal_derating"}, "curves"},
		{[]string{"lumen_maintenance_luminaire"}, "tm_28"},
		{[]string{"lumen_maintenance_luminaire"}, "manufacturer_rated_claim"},
		{[]string{"lumen_maintenance_luminaire"}, "cie_97_lmf_table"},
		// Phase D array parent (PhotometryFormat closure descends into source_files).
		{nil, "source_files"},
	}
	for _, tgt := range targets {
		for _, h := range hostiles {
			tgt, h := tgt, h
			label := fmt.Sprintf("%s.%s=%T", strings.Join(tgt.parent, "."), tgt.key, h)
			t.Run(label, func(t *testing.T) {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("panic on %s: %v", label, r)
					}
				}()
				rec := fullBase()
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
					return // parent already replaced by a hostile in a prior subtest path
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

// (The order-independent build-parity test, index.Build(record)["conformance_level"]
// == AchievedLevel(record).String(), lives in the index package, which can import
// both index and grade without a cycle.)

// --- helpers ---

func loadRecord(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	normalized, err := normalizeForTest(raw)
	if err != nil {
		t.Fatalf("normalize %s: %v", path, err)
	}
	m, ok := normalized.(map[string]any)
	if !ok {
		t.Fatalf("%s: top-level JSON is not an object", path)
	}
	return m
}

// normalizeForTest mirrors main.normalizeNumbers: json.Number -> int64 when
// integral, float64 otherwise, so the grader sees the same shapes the CLI does.
func normalizeForTest(v any) (any, error) {
	switch n := v.(type) {
	case map[string]any:
		for k, child := range n {
			fixed, err := normalizeForTest(child)
			if err != nil {
				return nil, err
			}
			n[k] = fixed
		}
		return n, nil
	case []any:
		for i, child := range n {
			fixed, err := normalizeForTest(child)
			if err != nil {
				return nil, err
			}
			n[i] = fixed
		}
		return n, nil
	case json.Number:
		s := n.String()
		isInt := true
		for _, r := range s {
			if r == '.' || r == 'e' || r == 'E' {
				isInt = false
				break
			}
		}
		if isInt {
			if i, err := n.Int64(); err == nil {
				return i, nil
			}
		}
		f, err := n.Float64()
		if err != nil {
			return nil, fmt.Errorf("invalid number %q: %w", s, err)
		}
		return f, nil
	default:
		return v, nil
	}
}
