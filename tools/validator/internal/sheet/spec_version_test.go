package sheet

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// recordColumnHeaders is the pinned sorted header list of the records-sheet
// column spec. It is a sorted LIST rather than a count on purpose: a rename, or
// an add paired with a removal, nets out to the same count and would otherwise
// pass silently, and header renames are a contemplated edit shape on this
// surface.
var recordColumnHeaders = []string{
	"beam_angle_deg",
	"beam_family",
	"bug_b",
	"bug_g",
	"bug_u",
	"catalog_line",
	"catalog_model",
	"catalog_number",
	"ceiling_aperture_in",
	"ceiling_aperture_mm",
	"color_tunability",
	"connection_cable_length_in",
	"connection_cable_length_mm",
	"control_gear_type",
	"cri_ra",
	"cri_tier",
	"dimming_curve",
	"dimming_method",
	"dimming_range_max_percent",
	"dimming_range_min_percent",
	"discontinued_at",
	"distribution_manufacturer_label",
	"distribution_type",
	"distribution_type_photometry",
	"driver_protocol",
	"duv",
	"emission_face",
	"end_of_life_options",
	"environment_rating",
	"family_description",
	"family_display_name",
	"family_id",
	"field_angle_deg",
	"final_assembly_location",
	"finish_color_options",
	"housing_material",
	"ik_rating",
	"indoor_outdoor",
	"input_power_w",
	"input_voltage_at_test",
	"input_voltage_class",
	"input_voltage_v",
	"interior_performance",
	"ip_rating",
	"lbc_criteria_compliance",
	"led_module_power_w",
	"lens_material",
	"life_expectancy_years",
	"light_engine_variant",
	"linear_mass_per_foot_kg_per_m",
	"linear_mass_per_foot_lb_per_ft",
	"lm_claim_type",
	"lm_claimed_hours",
	"lm_declaration_framework",
	"longitudinal_distribution_range",
	"lumens_per_foot",
	"luminaire_efficacy_lm_per_w",
	"luminaire_mass_kg",
	"luminaire_mass_lb",
	"luminous_opening_shape",
	"manufacturer_display_name",
	"manufacturer_slug",
	"maximum_intensity_cd",
	"measurement_regime",
	"mounting_at_test",
	"mounting_types",
	"nominal_cct_at_test",
	"nominal_cct_k",
	"operating_input_frequency_hz",
	"operating_input_voltage_v",
	"outdoor_distribution_type",
	"outdoor_distribution_type_axis",
	"output_tier_manufacturer_label",
	"output_tier_meaning",
	"overall_diameter_in",
	"overall_diameter_mm",
	"overall_height_in",
	"overall_height_mm",
	"overall_length_in",
	"overall_length_mm",
	"overall_width_in",
	"overall_width_mm",
	"photometric_coordinate_system",
	"photometric_scenario_id",
	"photometry_basis",
	"primary_category",
	"recess_depth_in",
	"recess_depth_mm",
	"record_status",
	"record_status_as_of",
	"recyclable_percent",
	"reference_length_in",
	"reference_length_mm",
	"reflector_material",
	"responsible_sourcing",
	"scenario_label",
	"sdcm_step",
	"secondary_function",
	"shape",
	"source_ies_ref",
	"superseded_by_catalog_model",
	"superseded_by_catalog_number",
	"superseded_by_record_id",
	"superseded_by_record_sha256",
	"sustainability_declaration_type",
	"sustainability_document_id",
	"sustainability_expiration_date",
	"sustainability_issue_date",
	"symmetry_type",
	"technical_region",
	"total_luminous_flux_lm",
	"ugr_4h_8h",
	"ugr_4h_8h_bound_operator",
	"ulc_version",
	"voc_content",
	"warranty_scope",
	"warranty_term_basis",
	"warranty_term_years",
	"watts_per_foot",
}

// TestFromSheetDefaultVersionGuard guards the from-sheet ulc_version default in
// the two ways it can go wrong. First, the version the converter actually
// stamps must be one this repository documents: it has to appear as a dated
// CHANGELOG heading, so a typo or a never-documented version fails. That is a
// changelogged-version check, not a released-version check: a CHANGELOG heading
// proves the version is spelled correctly and is one this repo documents, not
// that it is tagged.
//
// Second, the records-sheet column set is pinned, so a release that adds
// authorable columns cannot silently forget to re-decide the default. The
// tripwire fires on any header-set change and cannot judge whether a bump is
// due; that stays a human decision this red test forces.
func TestFromSheetDefaultVersionGuard(t *testing.T) {
	// Read the default the converter actually stamps, rather than the literal
	// in convert.go, so the production constant stays independently pinned by
	// TestConvertPatternA.
	res := convertOneRecord(t, bundleWithColumns(t, map[string]string{}), Options{})
	stamped, _ := res.Record["ulc_version"].(string)
	if stamped == "" {
		t.Fatal("a converted record carries no ulc_version")
	}

	repoRoot := filepath.Dir(schemaDir(t))
	changelog, err := os.ReadFile(filepath.Join(repoRoot, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("read CHANGELOG.md: %v", err)
	}
	heading := regexp.MustCompile(`(?m)^## ` + regexp.QuoteMeta(stamped) + ` \(\d{4}-\d{2}-\d{2}\)$`)
	if !heading.Match(changelog) {
		t.Errorf("the from-sheet ulc_version default is %q, which has no dated `## %s (YYYY-MM-DD)` section in CHANGELOG.md; "+
			"the default must name a version this repository documents", stamped, stamped)
	}

	got := make([]string, 0, len(recordColumns))
	for _, c := range recordColumns {
		got = append(got, c.Header)
	}
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(recordColumnHeaders, "\n") {
		t.Errorf("the records-sheet column set changed; re-decide the from-sheet ulc_version default in convert.go "+
			"(bump it when the new columns author fields introduced after the current default) and update this pin.\n"+
			"current sorted headers:\n\t%s", strings.Join(got, "\n\t"))
	}
}
