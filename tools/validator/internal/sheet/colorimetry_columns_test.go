package sheet

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
	"github.com/ulcspec/ULC/tools/validator/internal/index"
	"github.com/ulcspec/ULC/tools/validator/internal/validate"
)

type colourColumnExpectation struct {
	header, path, unit string
	kind               Kind
}

var measuredColourColumns = []colourColumnExpectation{
	{"measured_cct_k", "colorimetry.measured_cct_k", "K", KindProvNumber},
	{"chromaticity_x", "colorimetry.chromaticity_x", "", KindProvNumber},
	{"chromaticity_y", "colorimetry.chromaticity_y", "", KindProvNumber},
	{"cri_r9", "colorimetry.cri_r9", "", KindProvNumber},
	{"tm_30_rf", "colorimetry.tm_30.rf", "", KindProvNumber},
	{"tm_30_rg", "colorimetry.tm_30.rg", "", KindProvNumber},
	{"tm_30_reference_illuminant_type", "colorimetry.tm_30.reference_illuminant_type", "", KindEnum},
	{"tm_30_pvf_code", "colorimetry.tm_30.pvf_code", "", KindString},
}

var colourNumericHeaders = []string{
	"measured_cct_k", "chromaticity_x", "chromaticity_y", "cri_r9", "tm_30_rf", "tm_30_rg",
}

const expectedRecordsTemplateHeaders = "record_id,cutsheet_file,cutsheet_file__revision_label,cutsheet_file__revision_date,ulc_version,record_status,record_status_as_of,superseded_by_record_id,superseded_by_record_sha256,superseded_by_catalog_number,superseded_by_catalog_model,discontinued_at,family_id,family_display_name,family_description,manufacturer_slug,manufacturer_display_name,catalog_line,catalog_model,primary_category,secondary_function,indoor_outdoor,technical_region,markets,mounting_types,environment_rating,shape,housing_material,lens_material,reflector_material,finish_color_options,ip_rating,ik_rating,warranty_term_years,warranty_term_basis,warranty_scope,warranty_conditions_file,warranty_conditions_file__revision_label,warranty_conditions_file__revision_date,overall_diameter_mm,overall_diameter_in,ceiling_aperture_mm,ceiling_aperture_in,recess_depth_mm,recess_depth_in,connection_cable_length_mm,connection_cable_length_in,overall_length_mm,overall_length_in,overall_width_mm,overall_width_in,overall_height_mm,overall_height_in,luminaire_mass_kg,luminaire_mass_lb,linear_mass_per_foot_kg_per_m,linear_mass_per_foot_lb_per_ft,photometric_scenario_id,catalog_number,scenario_label,source_ies_ref,distribution_manufacturer_label,distribution_type,outdoor_distribution_type_axis,light_engine_variant,output_tier_manufacturer_label,output_tier_meaning,cri_tier,color_tunability,nominal_cct_at_test,input_voltage_at_test,mounting_at_test,input_power_w,input_voltage_v,input_voltage_class,power_factor,power_factor_bound_operator,thd_percent,thd_percent_bound_operator,driver_protocol,dimming_method,dimming_range_min_percent,dimming_range_max_percent,dimming_curve,control_gear_type,led_module_power_w,total_luminous_flux_lm,luminaire_efficacy_lm_per_w,maximum_intensity_cd,beam_angle_deg,field_angle_deg,ugr_4h_8h,ugr_4h_8h_bound_operator,max_surface_luminance_cd_per_m2,max_surface_luminance_bound_operator,beam_family,distribution_type_photometry,symmetry_type,photometric_coordinate_system,luminous_opening_shape,emission_face,lumens_per_foot,watts_per_foot,reference_length_mm,reference_length_in,operating_input_voltage_v,operating_input_frequency_hz,nominal_cct_k,cri_ra,duv,sdcm_step,measured_cct_k,measured_cct_k__value_type,measured_cct_k__prov_source,measured_cct_k__prov_method,measured_cct_k__extension_method,measured_cct_k__base_attestation_ref,measured_cct_k__attestation_ref,chromaticity_x,chromaticity_x__value_type,chromaticity_x__prov_source,chromaticity_x__prov_method,chromaticity_x__extension_method,chromaticity_x__base_attestation_ref,chromaticity_x__attestation_ref,chromaticity_y,chromaticity_y__value_type,chromaticity_y__prov_source,chromaticity_y__prov_method,chromaticity_y__extension_method,chromaticity_y__base_attestation_ref,chromaticity_y__attestation_ref,cri_r9,cri_r9__value_type,cri_r9__prov_source,cri_r9__prov_method,cri_r9__extension_method,cri_r9__base_attestation_ref,cri_r9__attestation_ref,tm_30_rf,tm_30_rf__value_type,tm_30_rf__prov_source,tm_30_rf__prov_method,tm_30_rf__extension_method,tm_30_rf__base_attestation_ref,tm_30_rf__attestation_ref,tm_30_rg,tm_30_rg__value_type,tm_30_rg__prov_source,tm_30_rg__prov_method,tm_30_rg__extension_method,tm_30_rg__base_attestation_ref,tm_30_rg__attestation_ref,tm_30_reference_illuminant_type,tm_30_pvf_code,bug_b,bug_u,bug_g,outdoor_distribution_type,longitudinal_distribution_range,photometry_basis,measurement_regime,lm_declaration_framework,lm_claim_type,lm_claimed_hours,lm_claim_basis,sustainability_declaration_type,sustainability_document_id,sustainability_issue_date,sustainability_expiration_date,final_assembly_location,life_expectancy_years,recyclable_percent,end_of_life_options,lbc_criteria_compliance,voc_content,interior_performance,responsible_sourcing,extensions_json,extensions_slug,applicable_catalog_pattern,fixed_axes,applicable_sku_count_estimate"

func colourBundle(t *testing.T, cells map[string]string) string {
	t.Helper()
	bundle := writeTemplateHeaderBundle(t)
	for header, value := range cells {
		setClaimsCell(t, bundle, "records", header, value)
	}
	return bundle
}

func isolatedColourBundle(t *testing.T, cells map[string]string) string {
	t.Helper()
	all := map[string]string{}
	for header, value := range cells {
		all[header] = value
	}
	for _, header := range []string{
		"input_power_w", "total_luminous_flux_lm", "luminaire_efficacy_lm_per_w",
		"maximum_intensity_cd", "beam_angle_deg", "cri_ra",
	} {
		all[header+"__value_type"] = "rated"
	}
	return bundleWithColumns(t, all)
}

func bundleRecordID(t *testing.T, bundle string) string {
	t.Helper()
	rows := readCSVRows(t, filepath.Join(bundle, "records.csv"))
	for i, header := range rows[0] {
		if header == "record_id" {
			return rows[1][i]
		}
	}
	t.Fatal("records fixture has no record_id header")
	return ""
}

func setColourAttestations(t *testing.T, bundle string, rows ...map[string]string) {
	t.Helper()
	path := filepath.Join(bundle, "attestations.csv")
	current := readCSVRows(t, path)
	header := current[0]
	out := [][]string{header}
	for _, fields := range rows {
		if fields["record_id"] == "" {
			fields["record_id"] = bundleRecordID(t, bundle)
		}
		row := make([]string, len(header))
		for i, name := range header {
			row[i] = fields[name]
		}
		out = append(out, row)
	}
	writeCSVRows(t, path, out)
}

func colourAttestation(id, program string) map[string]string {
	return map[string]string{
		"attestation_id":    id,
		"program":           program,
		"status":            "verified",
		"value_type":        "measured",
		"verification_type": "unconditional",
	}
}

func provenancedNumberAt(t *testing.T, record map[string]any, path string) map[string]any {
	t.Helper()
	value, ok := getPath(record, path)
	if !ok {
		t.Fatalf("%s is absent", path)
	}
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %T, want map[string]any", path, value)
	}
	return object
}

func colourSchemaErrors(t *testing.T, input string) string {
	t.Helper()
	results, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	record := results[0].Record
	record["index"] = index.Build(record)
	validator, err := validate.NewValidator(schemaDir(t))
	if err != nil {
		t.Fatal(err)
	}
	report := findings.NewReport()
	validator.Validate(numberTree(t, record), report)
	report.Finalize()
	if !report.HasErrors() {
		t.Fatal("invalid measured-colour value passed schema validation")
	}
	var output bytes.Buffer
	if err := report.WriteText(&output, "record"); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestMeasuredColourColumnTableIsExact(t *testing.T) {
	if len(baseRecordColumns) != 125 {
		t.Fatalf("baseRecordColumns count = %d, want 125", len(baseRecordColumns))
	}
	if len(recordColumns) != 135 {
		t.Fatalf("recordColumns count = %d, want 135", len(recordColumns))
	}
	start := -1
	for i, column := range baseRecordColumns {
		if column.Header == "measured_cct_k" {
			start = i
			break
		}
	}
	if start < 1 || baseRecordColumns[start-1].Header != "sdcm_step" {
		t.Fatalf("measured-colour group does not immediately follow sdcm_step")
	}
	if got := baseRecordColumns[start+len(measuredColourColumns)].Header; got != "bug_b" {
		t.Fatalf("column after measured-colour group = %q, want bug_b", got)
	}
	for i, want := range measuredColourColumns {
		got := baseRecordColumns[start+i]
		if got.Header != want.header || got.Path != want.path || got.Kind != want.kind || got.Unit != want.unit {
			t.Errorf("column %d = %#v, want header=%q path=%q kind=%v unit=%q", i, got, want.header, want.path, want.kind, want.unit)
		}
		if want.kind == KindProvNumber {
			if got.ProvValueType != "measured" || got.ProvSource != "test_report" || got.ProvMethod != "transcribed" {
				t.Errorf("%s defaults = %s/%s/%s, want measured/test_report/transcribed", got.Header, got.ProvValueType, got.ProvSource, got.ProvMethod)
			}
		} else if got.Provenanced() {
			t.Errorf("plain column %s unexpectedly reports provenance support", got.Header)
		}
	}
}

func TestMeasuredColourTemplateHeaderContract(t *testing.T) {
	template := filepath.Join(filepath.Dir(schemaDir(t)), "templates", "workbook", "records.csv")
	file, err := os.Open(template)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("records template rows = %d, want header only", len(rows))
	}
	header := rows[0]
	want := strings.Split(expectedRecordsTemplateHeaders, ",")
	if !reflect.DeepEqual(header, want) {
		t.Fatalf("records template header order changed\n got: %s\nwant: %s", strings.Join(header, ","), expectedRecordsTemplateHeaders)
	}
	if len(header) != 183 {
		t.Fatalf("records template headers = %d, want 183", len(header))
	}
	seen := map[string]bool{}
	for _, name := range header {
		if seen[name] {
			t.Errorf("duplicate records template header %q", name)
		}
		seen[name] = true
	}
	companions := CompanionHeaderSuffixes[CompanionFamilyProvenance]
	for _, base := range colourNumericHeaders {
		position := -1
		for i, name := range header {
			if name == base {
				position = i
				break
			}
		}
		if position < 0 {
			t.Fatalf("template lacks %s", base)
		}
		for i, suffix := range companions {
			if got := header[position+i+1]; got != base+suffix {
				t.Errorf("header after %s at offset %d = %q, want %q", base, i+1, got, base+suffix)
			}
		}
	}
	for _, base := range []string{"tm_30_reference_illuminant_type", "tm_30_pvf_code"} {
		for _, suffix := range companions {
			if seen[base+suffix] {
				t.Errorf("plain field %s exposes companion %s", base, suffix)
			}
		}
	}
}

func TestMeasuredColourColumnsAcrossReaders(t *testing.T) {
	bundle := colourBundle(t, map[string]string{
		"measured_cct_k":                  "3012.25",
		"chromaticity_x":                  "0.312345678901234",
		"chromaticity_y":                  "0.329876543210987",
		"cri_r9":                          "-4.5",
		"tm_30_rf":                        "87.25",
		"tm_30_rg":                        "101.75",
		"tm_30_reference_illuminant_type": "planckian",
		"tm_30_pvf_code":                  "P2",
		"duv":                             "-0.0012",
	})
	wantNumbers := map[string]float64{
		"colorimetry.measured_cct_k": 3012.25,
		"colorimetry.chromaticity_x": 0.312345678901234,
		"colorimetry.chromaticity_y": 0.329876543210987,
		"colorimetry.cri_r9":         -4.5,
		"colorimetry.tm_30.rf":       87.25,
		"colorimetry.tm_30.rg":       101.75,
	}
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			record := convertOneRecord(t, input, Options{}).Record
			for path, want := range wantNumbers {
				object := provenancedNumberAt(t, record, path)
				if got := asFloat(t, object["value"]); got != want {
					t.Errorf("%s value = %.15g, want %.15g", path, got, want)
				}
				if object["value_type"] != "measured" {
					t.Errorf("%s value_type = %v", path, object["value_type"])
				}
				provenance := object["provenance"].(map[string]any)
				if provenance["source"] != "test_report" || provenance["method"] != "transcribed" || provenance["attestation_ref"] != "lm79_lumos_skyline_sr_ho" {
					t.Errorf("%s provenance = %v", path, provenance)
				}
				if path == "colorimetry.measured_cct_k" {
					if object["unit"] != "K" {
						t.Errorf("measured CCT unit = %v, want K", object["unit"])
					}
				} else if _, exists := object["unit"]; exists {
					t.Errorf("dimensionless %s has unit %v", path, object["unit"])
				}
			}
			if got, _ := getPath(record, "colorimetry.nominal_cct_k"); got != "3000" {
				t.Errorf("nominal CCT = %v, want 3000", got)
			}
			if got := asFloat(t, provenancedNumberAt(t, record, "colorimetry.cri_ra")["value"]); got != 80 {
				t.Errorf("existing CRI Ra = %v, want 80", got)
			}
			if got := asFloat(t, provenancedNumberAt(t, record, "colorimetry.duv")["value"]); got != -0.0012 {
				t.Errorf("existing Duv = %v, want -0.0012", got)
			}
			if got := asFloat(t, provenancedNumberAt(t, record, "colorimetry.sdcm_step")["value"]); got != 3 {
				t.Errorf("existing SDCM = %v, want 3", got)
			}
			if _, exists := getPath(record, "colorimetry.spectral_power_distribution"); !exists {
				t.Error("existing spectral power distribution was dropped")
			}
			if got, _ := getPath(record, "colorimetry.tm_30.reference_illuminant_type"); got != "planckian" {
				t.Errorf("reference illuminant = %v", got)
			}
			if got, _ := getPath(record, "colorimetry.tm_30.pvf_code"); got != "P2" {
				t.Errorf("PVF code = %v", got)
			}
			wantSchemaValid(t, record)
		})
	}
}

func TestMeasuredColourCompanionsAndIndependentTM30AcrossReaders(t *testing.T) {
	bundle := isolatedColourBundle(t, map[string]string{
		"tm_30_rf":                             "88",
		"tm_30_rf__attestation_ref":            "colour-a",
		"tm_30_rg":                             "102",
		"tm_30_rg__prov_method":                "extracted",
		"tm_30_rg__attestation_ref":            "colour-b",
		"chromaticity_y":                       "0.33",
		"chromaticity_y__value_type":           "rated",
		"chromaticity_y__prov_source":          "datasheet_pdf",
		"chromaticity_y__prov_method":          "scaled",
		"chromaticity_y__extension_method":     "cct_multiplier",
		"chromaticity_y__base_attestation_ref": "colour-b",
		"chromaticity_y__attestation_ref":      "colour-a",
	})
	setColourAttestations(t, bundle,
		colourAttestation("colour-a", "lm_79_19"),
		colourAttestation("colour-b", "lm_79_24"),
	)
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			record := convertOneRecord(t, input, Options{}).Record
			rf := provenancedNumberAt(t, record, "colorimetry.tm_30.rf")
			rg := provenancedNumberAt(t, record, "colorimetry.tm_30.rg")
			if rf["provenance"].(map[string]any)["attestation_ref"] != "colour-a" {
				t.Errorf("Rf provenance = %v", rf["provenance"])
			}
			rgProvenance := rg["provenance"].(map[string]any)
			if rgProvenance["attestation_ref"] != "colour-b" || rgProvenance["method"] != "extracted" {
				t.Errorf("Rg provenance = %v", rgProvenance)
			}
			y := provenancedNumberAt(t, record, "colorimetry.chromaticity_y")
			provenance := y["provenance"].(map[string]any)
			if y["value_type"] != "rated" || provenance["source"] != "datasheet_pdf" || provenance["method"] != "scaled" || provenance["extension_method"] != "cct_multiplier" || provenance["base_attestation_ref"] != "colour-b" || provenance["attestation_ref"] != "colour-a" {
				t.Errorf("companion overrides = value_type %v provenance %v", y["value_type"], provenance)
			}
			wantSchemaValid(t, record)
		})
	}
}

func TestTM30SparseAndBlankCasesAcrossReaders(t *testing.T) {
	tests := []struct {
		name     string
		cells    map[string]string
		wantPath string
	}{
		{"Rf only", map[string]string{"tm_30_rf": "90"}, "colorimetry.tm_30.rf"},
		{"Rg only", map[string]string{"tm_30_rg": "100"}, "colorimetry.tm_30.rg"},
		{"illuminant only", map[string]string{"tm_30_reference_illuminant_type": "cie_d_series"}, "colorimetry.tm_30.reference_illuminant_type"},
		{"PVF only", map[string]string{"tm_30_pvf_code": "V1"}, "colorimetry.tm_30.pvf_code"},
		{"all blank", map[string]string{"tm_30_rf": "", "tm_30_rg": "", "tm_30_reference_illuminant_type": "", "tm_30_pvf_code": ""}, ""},
		{"no automatic derivation", map[string]string{"measured_cct_k": "4200"}, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := colourBundle(t, test.cells)
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					record := convertOneRecord(t, input, Options{}).Record
					_, hasTM30 := getPath(record, "colorimetry.tm_30")
					if test.wantPath == "" {
						if hasTM30 {
							t.Errorf("blank or unrelated cells emitted tm_30")
						}
						return
					}
					if _, exists := getPath(record, test.wantPath); !exists {
						t.Errorf("%s is absent", test.wantPath)
					}
					wantSchemaValid(t, record)
				})
			}
		})
	}
}

func TestMeasuredColourNumericRefusalsAcrossReaders(t *testing.T) {
	badValues := []struct{ name, value, reason string }{
		{"text", "not-a-number", "invalid number"},
		{"NaN", "NaN", "non-finite number"},
		{"infinity", "Inf", "non-finite number"},
		{"overflow", "1e999", "invalid number"},
	}
	for _, header := range colourNumericHeaders {
		for _, bad := range badValues {
			t.Run(header+"/"+bad.name, func(t *testing.T) {
				bundle := colourBundle(t, map[string]string{header: bad.value})
				for reader, input := range supplementaryInputs(t, bundle) {
					t.Run(reader, func(t *testing.T) {
						_, err := Convert(input, Options{})
						if err == nil {
							t.Fatal("malformed numeric cell converted")
						}
						for _, want := range []string{"record \"lumos-skyline-sr-ho-3000k\"", "column \"" + header + "\"", bad.reason} {
							if !strings.Contains(err.Error(), want) {
								t.Errorf("error %q lacks %q", err, want)
							}
						}
					})
				}
			})
		}
	}
}

func TestMeasuredColourHeaderPreflightAcrossReaders(t *testing.T) {
	for _, header := range []string{
		"measured_cct_k__prov_sorce",
		"tm_30_rf__source",
		"tm_30_pvf_code__value_type",
		"tm_30_reference_illuminant_type__prov_source",
	} {
		t.Run(header, func(t *testing.T) {
			bundle := colourBundle(t, map[string]string{header: "", "record_id": ""})
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					_, err := Convert(input, Options{})
					if err == nil {
						t.Fatal("unsupported blank companion header converted")
					}
					if !strings.Contains(err.Error(), "sheet \"records\"") || !strings.Contains(err.Error(), header) {
						t.Errorf("preflight error %q does not identify the header", err)
					}
					if strings.Contains(err.Error(), "missing record_id") {
						t.Errorf("record assembly ran before companion preflight: %v", err)
					}
				})
			}
		})
	}
}

func TestMeasuredColourReferenceRefusalsAcrossReaders(t *testing.T) {
	tests := []struct {
		name         string
		cells        map[string]string
		attestations []map[string]string
		want         []string
	}{
		{"missing anchor", map[string]string{"measured_cct_k": "3100"}, nil, []string{"no photometric lm_79"}},
		{"ambiguous anchor", map[string]string{"measured_cct_k": "3100"}, []map[string]string{colourAttestation("a", "lm_79_19"), colourAttestation("b", "lm_79_24")}, []string{"declares 2 photometric lm_79", "disambiguate"}},
		{"nonexistent reference", map[string]string{"measured_cct_k": "3100", "measured_cct_k__attestation_ref": "missing"}, []map[string]string{colourAttestation("a", "lm_79_19")}, []string{"no attestation", "missing"}},
		{"duplicate reference", map[string]string{"measured_cct_k": "3100", "measured_cct_k__attestation_ref": "same"}, []map[string]string{colourAttestation("same", "lm_79_19"), colourAttestation("same", "lm_79_24")}, []string{"declared by 2 attestations"}},
		{"wrong family", map[string]string{"measured_cct_k": "3100", "measured_cct_k__attestation_ref": "maintenance"}, []map[string]string{colourAttestation("maintenance", "lm_80_21")}, []string{"different evidence family", "photometric lm_79"}},
		{"confirmation required", map[string]string{"measured_cct_k": "3100", "measured_cct_k__attestation_ref": "case"}, []map[string]string{func() map[string]string {
			a := colourAttestation("case", "lm_79_24")
			a["value_type"] = "rated"
			a["verification_type"] = "requires_manufacturer_confirmation"
			return a
		}()}, []string{"requires manufacturer confirmation"}},
		{"derived requires rated", map[string]string{"measured_cct_k": "3100", "measured_cct_k__prov_method": "scaled"}, []map[string]string{colourAttestation("a", "lm_79_19")}, []string{"derived method", "requires value_type=rated"}},
		{"derived requires base", map[string]string{"measured_cct_k": "3100", "measured_cct_k__value_type": "rated", "measured_cct_k__prov_method": "scaled"}, nil, []string{"base_attestation_ref", "no photometric lm_79"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := isolatedColourBundle(t, test.cells)
			recordID := bundleRecordID(t, bundle)
			setColourAttestations(t, bundle, test.attestations...)
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					_, err := Convert(input, Options{})
					if err == nil {
						t.Fatal("invalid provenance reference converted")
					}
					for _, want := range append([]string{"record \"" + recordID + "\"", "measured_cct_k"}, test.want...) {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("error %q lacks %q", err, want)
						}
					}
				})
			}
		})
	}
}

func TestMeasuredColourExplicitDisambiguationAndValueTypeOverridesAcrossReaders(t *testing.T) {
	t.Run("explicit disambiguation", func(t *testing.T) {
		bundle := isolatedColourBundle(t, map[string]string{"cri_r9": "-2", "cri_r9__attestation_ref": "selected"})
		setColourAttestations(t, bundle, colourAttestation("other", "lm_79_19"), colourAttestation("selected", "lm_79_24"))
		for reader, input := range supplementaryInputs(t, bundle) {
			t.Run(reader, func(t *testing.T) {
				record := convertOneRecord(t, input, Options{}).Record
				provenance := provenancedNumberAt(t, record, "colorimetry.cri_r9")["provenance"].(map[string]any)
				if provenance["attestation_ref"] != "selected" {
					t.Errorf("attestation_ref = %v", provenance["attestation_ref"])
				}
			})
		}
	})
	for _, valueType := range []string{"rated", "nominal"} {
		t.Run(valueType, func(t *testing.T) {
			bundle := isolatedColourBundle(t, map[string]string{"chromaticity_x": "0.31", "chromaticity_x__value_type": valueType})
			setColourAttestations(t, bundle)
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					record := convertOneRecord(t, input, Options{}).Record
					object := provenancedNumberAt(t, record, "colorimetry.chromaticity_x")
					if object["value_type"] != valueType {
						t.Errorf("value_type = %v, want %s", object["value_type"], valueType)
					}
					if _, exists := object["provenance"].(map[string]any)["attestation_ref"]; exists {
						t.Error("non-measured override gained attestation_ref")
					}
					wantSchemaValid(t, record)
				})
			}
		})
	}
}

func TestMeasuredColourSchemaRefusalsAcrossReaders(t *testing.T) {
	tests := []struct {
		name, location string
		cells          map[string]string
	}{
		{"illuminant", "/colorimetry/tm_30/reference_illuminant_type", map[string]string{"tm_30_reference_illuminant_type": "moonlight"}},
		{"PVF", "/colorimetry/tm_30/pvf_code", map[string]string{"tm_30_pvf_code": "X9"}},
		{"value type", "/colorimetry/measured_cct_k/value_type", map[string]string{"measured_cct_k": "3000", "measured_cct_k__value_type": "estimated"}},
		{"source", "/colorimetry/measured_cct_k/provenance/source", map[string]string{"measured_cct_k": "3000", "measured_cct_k__prov_source": "unknown_source"}},
		{"method", "/colorimetry/measured_cct_k/provenance/method", map[string]string{"measured_cct_k": "3000", "measured_cct_k__prov_method": "unknown_method"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := colourBundle(t, test.cells)
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					output := colourSchemaErrors(t, input)
					if !strings.Contains(output, test.location) {
						t.Errorf("schema report lacks %q:\n%s", test.location, output)
					}
				})
			}
		})
	}
}

func TestExistingBundlesOmitMeasuredColourColumnsAcrossReaders(t *testing.T) {
	paths := []string{
		"colorimetry.measured_cct_k", "colorimetry.chromaticity_x", "colorimetry.chromaticity_y",
		"colorimetry.cri_r9", "colorimetry.tm_30.rf", "colorimetry.tm_30.rg",
		"colorimetry.tm_30.reference_illuminant_type", "colorimetry.tm_30.pvf_code",
	}
	for _, name := range []string{"bundle", "bundle-b", "bundle-c", "bundle-d"} {
		t.Run(name, func(t *testing.T) {
			bundle := t.TempDir()
			copyBundle(t, filepath.Join("testdata", name), bundle)
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					results, err := Convert(input, Options{})
					if err != nil {
						t.Fatalf("Convert: %v", err)
					}
					for _, result := range results {
						for _, path := range paths {
							if value, exists := getPath(result.Record, path); exists {
								t.Errorf("old %s input emitted %s = %v", reader, path, value)
							}
						}
					}
				})
			}
		})
	}
}
