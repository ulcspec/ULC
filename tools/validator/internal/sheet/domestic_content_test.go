package sheet

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/completeness"
	"github.com/ulcspec/ULC/tools/validator/internal/findings"
	"github.com/ulcspec/ULC/tools/validator/internal/index"
	"github.com/ulcspec/ULC/tools/validator/internal/validate"
)

var domesticColumns = []string{
	"domestic_content_us_cost_share_percent",
	"domestic_content_foreign_cost_share_percent",
	"domestic_content_threshold_percent",
	"domestic_content_threshold_effective_date",
	"domestic_content_basis",
	"domestic_content__prov_source",
	"domestic_content__prov_method",
}

func domesticBundle(t *testing.T, sheet string, cells map[string]string) string {
	t.Helper()
	bundle := t.TempDir()
	copyBundle(t, filepath.Join("testdata", "bundle-b"), bundle)
	path := filepath.Join(bundle, sheet+".csv")
	rows := readCSVRows(t, path)
	rows[0] = append(rows[0], domesticColumns...)
	for i := 1; i < len(rows); i++ {
		rows[i] = append(rows[i], make([]string, len(domesticColumns))...)
	}
	added := make([]string, len(rows[0]))
	values := map[string]string{
		"record_id":         rows[1][0],
		"attestation_id":    "baa-domestic-test",
		"program":           "baa",
		"status":            "claimed",
		"value_type":        "rated",
		"verification_type": "unconditional",
	}
	for key, value := range cells {
		values[key] = value
	}
	for i, header := range rows[0] {
		added[i] = values[header]
	}
	rows = append(rows, added)
	writeCSVRows(t, path, rows)
	return bundle
}

func domesticPayload(t *testing.T, record map[string]any, sheet string) map[string]any {
	t.Helper()
	path := "attestations"
	if sheet == "shared_attestations" {
		path = "product_family.shared_attestations"
	}
	for _, item := range arrayAt(t, record, path) {
		attestation := item.(map[string]any)
		if attestation["attestation_id"] == "baa-domestic-test" {
			payload, _ := attestation["domestic_content"].(map[string]any)
			return payload
		}
	}
	t.Fatalf("%s has no baa-domestic-test attestation", path)
	return nil
}

func TestDomesticContentAcrossReaders(t *testing.T) {
	for _, sheet := range []string{"attestations", "shared_attestations"} {
		for _, override := range []bool{false, true} {
			name := sheet + "/default"
			if override {
				name = sheet + "/override"
			}
			t.Run(name, func(t *testing.T) {
				cells := map[string]string{
					"domestic_content_us_cost_share_percent":      "88.50",
					"domestic_content_foreign_cost_share_percent": "11.50",
					"domestic_content_threshold_percent":          "65",
					"domestic_content_threshold_effective_date":   "2024-01-01",
					"domestic_content_basis":                      "manufacturing_cost",
				}
				source, method := "manufacturer_direct", "transcribed"
				if override {
					cells["domestic_content__prov_source"] = "manufacturer_data_export"
					cells["domestic_content__prov_method"] = "extracted"
					source, method = "manufacturer_data_export", "extracted"
				}
				bundle := domesticBundle(t, sheet, cells)
				for reader, input := range supplementaryInputs(t, bundle) {
					t.Run(reader, func(t *testing.T) {
						record := convertOne(t, input, PatternB, completeness.LevelStandard)
						payload := domesticPayload(t, record, sheet)
						if asFloat(t, payload["us_cost_share_percent"]) != 88.5 || asFloat(t, payload["foreign_cost_share_percent"]) != 11.5 || asFloat(t, payload["threshold_percent"]) != 65 {
							t.Errorf("cost shares and threshold = %v", payload)
						}
						if payload["threshold_effective_date"] != "2024-01-01" || payload["basis"] != "manufacturing_cost" {
							t.Errorf("date or basis = %v", payload)
						}
						if _, present := payload["value_type"]; present {
							t.Errorf("domestic payload has a value_type: %v", payload)
						}
						provenance := payload["provenance"].(map[string]any)
						if provenance["source"] != source || provenance["method"] != method {
							t.Errorf("provenance = %v, want %s and %s", provenance, source, method)
						}
					})
				}
			})
		}
	}
}

func domesticSchemaErrors(t *testing.T, input string) string {
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
		t.Fatal("invalid domestic payload passed schema validation")
	}
	var output bytes.Buffer
	if err := report.WriteText(&output, "record"); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestDomesticContentSchemaRefusalsAcrossReaders(t *testing.T) {
	for _, test := range []struct {
		name, sheet string
		cells       map[string]string
		want        string
	}{
		{"incomplete", "attestations", map[string]string{"domestic_content_us_cost_share_percent": "88.50", "domestic_content_basis": "manufacturing_cost"}, "threshold_percent"},
		{"over 100", "shared_attestations", map[string]string{"domestic_content_us_cost_share_percent": "101", "domestic_content_threshold_percent": "65", "domestic_content_basis": "manufacturing_cost"}, "us_cost_share_percent"},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := domesticBundle(t, test.sheet, test.cells)
			location := "/attestations/1/domestic_content"
			if test.sheet == "shared_attestations" {
				location = "/product_family/shared_attestations/2/domestic_content"
			}
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					output := domesticSchemaErrors(t, input)
					for _, want := range []string{location, test.want} {
						if !strings.Contains(output, want) {
							t.Errorf("schema report does not contain %q:\n%s", want, output)
						}
					}
				})
			}
		})
	}
}

func TestDomesticContentRejectsValueTypeCompanionAcrossReaders(t *testing.T) {
	for _, sheet := range []string{"attestations", "shared_attestations"} {
		bundle := domesticBundle(t, sheet, nil)
		path := filepath.Join(bundle, sheet+".csv")
		rows := readCSVRows(t, path)
		rows[0] = append(rows[0], "domestic_content__value_type")
		for i := 1; i < len(rows); i++ {
			rows[i] = append(rows[i], "")
		}
		writeCSVRows(t, path, rows)
		for reader, input := range supplementaryInputs(t, bundle) {
			t.Run(sheet+"/"+reader, func(t *testing.T) {
				_, err := Convert(input, Options{})
				if err == nil {
					t.Fatal("unsupported domestic_content__value_type companion was accepted")
				}
				for _, want := range []string{sheet, "domestic_content__value_type", "unsupported suffix"} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
			})
		}
	}
}

func TestDomesticContentOmittedForBlankColumnsAcrossReaders(t *testing.T) {
	for _, sheet := range []string{"attestations", "shared_attestations"} {
		bundle := domesticBundle(t, sheet, nil)
		for reader, input := range supplementaryInputs(t, bundle) {
			t.Run(sheet+"/"+reader, func(t *testing.T) {
				record := convertOne(t, input, PatternB, completeness.LevelStandard)
				if payload := domesticPayload(t, record, sheet); payload != nil {
					t.Errorf("blank domestic columns emitted payload %v", payload)
				}
			})
		}
	}
}
