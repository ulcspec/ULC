package grade

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
// (tools/validator/internal/grade -> four levels up).
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

// TestObservationsAtCoreAndAbove confirms a core record emits observation findings
// (including Duv for a white-light fixture) while an incomplete record emits none.
func TestObservationsAtCoreAndAbove(t *testing.T) {
	// Core white-light record: observations fire (thermal, sustainability, Duv).
	report := findings.NewReport()
	Report(coreBase(), report)
	report.Finalize()
	for _, p := range []string{"/thermal_derating", "/sustainability_declaration", "/colorimetry/duv"} {
		if !hasObservationAt(report, p) {
			t.Errorf("core record: expected observation at %s; observations: %+v",
				p, findingsFor(report, findings.CodeConformanceObservation))
		}
	}
	// Incomplete record: no observations, only the to-core roadmap.
	inc := coreBase()
	delete(inc["product_family"].(map[string]any), "shared_attestations")
	report2 := findings.NewReport()
	if got := Report(inc, report2); got != LevelIncomplete {
		t.Fatalf("achieved = %s, want incomplete", got)
	}
	report2.Finalize()
	if obs := findingsFor(report2, findings.CodeConformanceObservation); len(obs) != 0 {
		t.Errorf("incomplete record emitted %d observations, want 0: %+v", len(obs), obs)
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

// TestPredicatesReadOnlyCoreFields asserts the applicability predicates read only
// core fields: stripping every standard/full field leaves each predicate's value
// unchanged. coreBase and fullBase share identical core fields, so each predicate
// must agree across them, for a neutral fixture and an outdoor-site fixture.
func TestPredicatesReadOnlyCoreFields(t *testing.T) {
	preds := map[string]predicate{
		"hasWhitePoint": hasWhitePoint, "isWhiteLightPrimary": isWhiteLightPrimary,
		"directional": directional, "outdoorSite": outdoorSite, "linear": linear,
		"requiresDimmingDetail": requiresDimmingDetail, "wetOrExposed": wetOrExposed, "impactPublic": impactPublic,
		"poleMounted": poleMounted, "hasMarketSafetyListing": hasMarketSafetyListing,
	}
	check := func(t *testing.T, core, full map[string]any) {
		for name, p := range preds {
			if p(core) != p(full) {
				t.Errorf("predicate %s differs between core and full fixtures (reads a non-core field): core=%v full=%v",
					name, p(core), p(full))
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
