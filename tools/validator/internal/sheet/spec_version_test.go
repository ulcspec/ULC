package sheet

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
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

type datedChangelogVersion struct {
	raw                 string
	major, minor, patch int
}

func datedChangelogVersions(t *testing.T) []datedChangelogVersion {
	t.Helper()
	repoRoot := filepath.Dir(schemaDir(t))
	changelog, err := os.ReadFile(filepath.Join(repoRoot, "CHANGELOG.md"))
	if err != nil {
		t.Fatalf("read CHANGELOG.md: %v", err)
	}
	heading := regexp.MustCompile(`(?m)^## (\d+)\.(\d+)\.(\d+) \(\d{4}-\d{2}-\d{2}\)$`)
	matches := heading.FindAllStringSubmatch(string(changelog), -1)
	if len(matches) == 0 {
		t.Fatal("CHANGELOG.md has no dated release headings")
	}
	versions := make([]datedChangelogVersion, 0, len(matches))
	for _, match := range matches {
		major, _ := strconv.Atoi(match[1])
		minor, _ := strconv.Atoi(match[2])
		patch, _ := strconv.Atoi(match[3])
		versions = append(versions, datedChangelogVersion{
			raw: match[1] + "." + match[2] + "." + match[3], major: major, minor: minor, patch: patch,
		})
	}
	return versions
}

func assertCurrentDocumentedPatch(t *testing.T, stamped string) {
	t.Helper()
	parts := regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`).FindStringSubmatch(stamped)
	if parts == nil {
		t.Fatalf("stamped ulc_version %q is not three dot-separated integers", stamped)
	}
	major, _ := strconv.Atoi(parts[1])
	minor, _ := strconv.Atoi(parts[2])
	patch, _ := strconv.Atoi(parts[3])
	found := false
	for _, version := range datedChangelogVersions(t) {
		if version.raw == stamped {
			found = true
		}
		if version.major == major && version.minor == minor && version.patch > patch {
			t.Errorf("from-sheet stamps %s, but CHANGELOG.md documents newer patch %s on the same %d.%d line", stamped, version.raw, major, minor)
		}
	}
	if !found {
		t.Errorf("from-sheet stamps %s, which has no dated CHANGELOG.md release heading", stamped)
	}
}

// TestFromSheetDefaultVersionGuard requires the converter's actual default to
// be a documented release and the newest patch on its own major.minor line.
func TestFromSheetDefaultVersionGuard(t *testing.T) {
	res := convertOneRecord(t, bundleWithColumns(t, map[string]string{}), Options{})
	stamped, _ := res.Record["ulc_version"].(string)
	if stamped == "" {
		t.Fatal("a converted record carries no ulc_version")
	}
	assertCurrentDocumentedPatch(t, stamped)
}

// TestRecordsSheetHeadersMatchTemplateContract pins the sorted records-sheet
// header set so converter and workbook-template edits cannot drift silently.
func TestRecordsSheetHeadersMatchTemplateContract(t *testing.T) {
	got := make([]string, 0, len(recordColumns))
	for _, c := range recordColumns {
		got = append(got, c.Header)
	}
	sort.Strings(got)
	if strings.Join(got, "\n") != strings.Join(recordColumnHeaders, "\n") {
		t.Errorf("the records-sheet column set changed; reconcile converter and workbook-template column parity, then update this sorted pin.\n"+
			"current sorted headers:\n\t%s", strings.Join(got, "\n\t"))
	}
}
