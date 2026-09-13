package sheet

import (
	"errors"
	"os"
	"os/exec"
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

func TestSpecificationVersionGreaterComparesComponents(t *testing.T) {
	tests := []struct {
		candidate string
		bound     string
		want      bool
	}{
		{candidate: "1.9.0", bound: "1.10.0", want: false},
		{candidate: "1.10.0", bound: "1.9.0", want: true},
		{candidate: "1.9.0", bound: "1.9.0", want: false},
	}
	for _, test := range tests {
		got, err := specificationVersionGreater(test.candidate, test.bound)
		if err != nil {
			t.Fatalf("specificationVersionGreater(%q, %q): %v", test.candidate, test.bound, err)
		}
		if got != test.want {
			t.Errorf("specificationVersionGreater(%q, %q) = %t, want %t", test.candidate, test.bound, got, test.want)
		}
	}
}

func TestFromSheetVersionCellBound(t *testing.T) {
	for _, malformed := range []string{"1.8", "v1.8.0", "1.8.0.1", "1.x.0"} {
		_, err := Convert(bundleWithColumns(t, map[string]string{"ulc_version": malformed}), Options{})
		if err == nil {
			t.Errorf("ulc_version %q converted, want malformed-cell refusal", malformed)
			continue
		}
		for _, want := range []string{"records", "ulc_version cell", malformed, "three dot-separated integers"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("ulc_version %q error %q does not contain %q", malformed, err, want)
			}
		}
	}

	future := "2.0.0"
	_, err := Convert(bundleWithColumns(t, map[string]string{"ulc_version": future}), Options{})
	if err == nil {
		t.Fatal("future ulc_version converted, want refusal")
	}
	for _, want := range []string{future, SpecVersion, "newer"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("future-version error %q does not contain %q", err, want)
		}
	}

	older := "1.7.0"
	res := convertOneRecord(t, bundleWithColumns(t, map[string]string{"ulc_version": older}), Options{})
	if got := res.Record["ulc_version"]; got != older {
		t.Errorf("older authored ulc_version = %v, want %s", got, older)
	}
}

func TestFromSheetBlankVersionCellUsesSpecVersionAcrossReaders(t *testing.T) {
	bundle := bundleWithColumns(t, map[string]string{"ulc_version": ""})
	xlsx := filepath.Join(bundle, "blank-version.xlsx")
	buildXLSX(t, xlsx, bundleToXLSXSheets(t, bundle))

	for name, input := range map[string]string{"CSV": bundle, "XLSX": xlsx} {
		t.Run(name, func(t *testing.T) {
			res := convertOneRecord(t, input, Options{})
			if got := res.Record["ulc_version"]; got != SpecVersion {
				t.Errorf("blank ulc_version cell stamped %v, want %s", got, SpecVersion)
			}
		})
	}
}

func TestReadmeCurrentReleaseMatchesSpecVersion(t *testing.T) {
	readme, err := os.ReadFile(filepath.Join(filepath.Dir(schemaDir(t)), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	matches := regexp.MustCompile(`(?m)^The current release is `+"`"+`([0-9]+\.[0-9]+\.[0-9]+)`+"`"+`\.`).FindAllStringSubmatch(string(readme), -1)
	if len(matches) != 1 {
		t.Fatalf("README current-release statements = %d, want 1", len(matches))
	}
	if got := matches[0][1]; got != SpecVersion {
		t.Errorf("README current release = %s, converter SpecVersion = %s", got, SpecVersion)
	}
}

func TestCheckSpecVersionScript(t *testing.T) {
	repoRoot := filepath.Dir(schemaDir(t))
	script := filepath.Join(repoRoot, "tools", "validator", "check-spec-version.sh")
	testRoot := t.TempDir()
	constantDir := filepath.Join(testRoot, "tools", "validator", "internal", "sheet")
	if err := os.MkdirAll(constantDir, 0o755); err != nil {
		t.Fatal(err)
	}
	constantPath := filepath.Join(constantDir, "specversion.go")
	writeConstant := func(t *testing.T, body string) {
		t.Helper()
		if err := os.WriteFile(constantPath, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) (string, int) {
		cmd := exec.Command("sh", append([]string{script}, args...)...)
		cmd.Dir = testRoot
		output, err := cmd.CombinedOutput()
		if err == nil {
			return string(output), 0
		}
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("run version guard: %v", err)
		}
		return string(output), exitErr.ExitCode()
	}

	t.Run("usage", func(t *testing.T) {
		output, code := run()
		if code != 2 || !strings.Contains(output, "usage:") {
			t.Fatalf("exit %d, output %q; want usage exit 2", code, output)
		}
	})
	t.Run("missing constant", func(t *testing.T) {
		if err := os.Remove(constantPath); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		output, code := run(SpecVersion)
		if code != 1 || !strings.Contains(output, "could not read exactly one version") {
			t.Fatalf("exit %d, output %q; want missing-constant refusal", code, output)
		}
	})
	t.Run("duplicate constant", func(t *testing.T) {
		writeConstant(t, "const SpecVersion = \"1.9.0\"\nconst SpecVersion = \"1.9.0\"\n")
		output, code := run(SpecVersion)
		if code != 1 || !strings.Contains(output, "could not read exactly one version") {
			t.Fatalf("exit %d, output %q; want duplicate-constant refusal", code, output)
		}
	})
	t.Run("mismatch", func(t *testing.T) {
		writeConstant(t, "const SpecVersion = \"1.9.0\"\n")
		output, code := run("1.8.0")
		if code != 1 || !strings.Contains(output, "SpecVersion mismatch") {
			t.Fatalf("exit %d, output %q; want mismatch refusal", code, output)
		}
	})
	t.Run("match", func(t *testing.T) {
		writeConstant(t, "const SpecVersion = \"1.9.0\"\n")
		output, code := run(SpecVersion)
		if code != 0 || !strings.Contains(output, "matches release version") {
			t.Fatalf("exit %d, output %q; want matching success", code, output)
		}
	})
}
