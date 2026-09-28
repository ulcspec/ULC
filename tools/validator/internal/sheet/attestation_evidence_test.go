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

var attestationEvidenceColumns = []string{
	"valid_until", "listing_number", "test_laboratory",
	"verification_contact_reference", "verification_notes",
}

func attestationEvidenceBundle(t *testing.T, sheet string, cells map[string]string) string {
	t.Helper()
	bundle := t.TempDir()
	copyBundle(t, filepath.Join("testdata", "bundle-b"), bundle)
	path := filepath.Join(bundle, sheet+".csv")
	rows := readCSVRows(t, path)
	rows[0] = append(rows[0], attestationEvidenceColumns...)
	for i := 1; i < len(rows); i++ {
		rows[i] = append(rows[i], make([]string, len(attestationEvidenceColumns))...)
	}
	values := map[string]string{
		"record_id":      rows[1][0],
		"attestation_id": "evidence-test",
		"program":        "baa",
		"status":         "claimed",
		"value_type":     "rated",
	}
	for key, value := range cells {
		values[key] = value
	}
	added := make([]string, len(rows[0]))
	for i, header := range rows[0] {
		added[i] = values[header]
	}
	writeCSVRows(t, path, append(rows, added))
	return bundle
}

func evidenceAttestation(t *testing.T, record map[string]any, sheet string) map[string]any {
	t.Helper()
	path := "attestations"
	if sheet == "shared_attestations" {
		path = "product_family.shared_attestations"
	}
	for _, item := range arrayAt(t, record, path) {
		att := item.(map[string]any)
		if att["attestation_id"] == "evidence-test" {
			return att
		}
	}
	t.Fatalf("%s has no evidence-test attestation", path)
	return nil
}

func TestAttestationEvidenceAcrossReaders(t *testing.T) {
	for _, sheet := range []string{"attestations", "shared_attestations"} {
		t.Run(sheet, func(t *testing.T) {
			bundle := attestationEvidenceBundle(t, sheet, map[string]string{
				"valid_until":                    "2030-12-31",
				"listing_number":                 "LIST-123",
				"test_laboratory":                "Independent Test Lab",
				"verification_type":              "requires_manufacturer_confirmation",
				"verification_contact_reference": "certification desk",
				"verification_notes":             "Confirm each project.",
			})
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					record := convertOne(t, input, PatternB, completeness.LevelStandard)
					att := evidenceAttestation(t, record, sheet)
					for key, want := range map[string]string{
						"valid_until": "2030-12-31", "listing_number": "LIST-123", "test_laboratory": "Independent Test Lab",
					} {
						if got := att[key]; got != want {
							t.Errorf("%s = %v, want %q", key, got, want)
						}
					}
					verification := att["verification"].(map[string]any)
					for key, want := range map[string]string{
						"type": "requires_manufacturer_confirmation", "contact_reference": "certification desk", "notes": "Confirm each project.",
					} {
						if got := verification[key]; got != want {
							t.Errorf("verification.%s = %v, want %q", key, got, want)
						}
					}
				})
			}
		})
	}
}

func TestAttestationEvidenceNotesDefaultAcrossReaders(t *testing.T) {
	for _, sheet := range []string{"attestations", "shared_attestations"} {
		t.Run(sheet, func(t *testing.T) {
			bundle := attestationEvidenceBundle(t, sheet, map[string]string{"verification_notes": "Review supporting evidence."})
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					record := convertOne(t, input, PatternB, completeness.LevelStandard)
					verification := evidenceAttestation(t, record, sheet)["verification"].(map[string]any)
					if verification["type"] != "unconditional" || verification["notes"] != "Review supporting evidence." {
						t.Errorf("verification = %v", verification)
					}
					if _, exists := verification["contact_reference"]; exists {
						t.Errorf("blank contact_reference was emitted: %v", verification)
					}
				})
			}
		})
	}
}

func evidenceSchemaErrors(t *testing.T, input string) string {
	t.Helper()
	results, err := Convert(input, Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	record := results[0].Record
	record["index"] = index.Build(record)
	v, err := validate.NewValidator(schemaDir(t))
	if err != nil {
		t.Fatal(err)
	}
	report := findings.NewReport()
	v.Validate(numberTree(t, record), report)
	report.Finalize()
	if !report.HasErrors() {
		t.Fatal("malformed valid_until passed schema validation")
	}
	var output bytes.Buffer
	if err := report.WriteText(&output, "record"); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestAttestationEvidenceInvalidDateAcrossReaders(t *testing.T) {
	for _, sheet := range []string{"attestations", "shared_attestations"} {
		t.Run(sheet, func(t *testing.T) {
			bundle := attestationEvidenceBundle(t, sheet, map[string]string{"valid_until": "2030-99-99"})
			location := "/attestations/1/valid_until"
			if sheet == "shared_attestations" {
				location = "/product_family/shared_attestations/2/valid_until"
			}
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					output := evidenceSchemaErrors(t, input)
					if !strings.Contains(output, location) {
						t.Errorf("schema report lacks %q:\n%s", location, output)
					}
				})
			}
		})
	}
}

func TestAttestationEvidenceAbsentAcrossReaders(t *testing.T) {
	for _, sheet := range []string{"attestations", "shared_attestations"} {
		t.Run(sheet, func(t *testing.T) {
			bundle := attestationEvidenceBundle(t, sheet, nil)
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					record := convertOne(t, input, PatternB, completeness.LevelStandard)
					att := evidenceAttestation(t, record, sheet)
					for _, key := range []string{"valid_until", "listing_number", "test_laboratory"} {
						if _, exists := att[key]; exists {
							t.Errorf("blank %s was emitted", key)
						}
					}
					verification := att["verification"].(map[string]any)
					if len(verification) != 1 || verification["type"] != "unconditional" {
						t.Errorf("blank verification columns emitted: %v", verification)
					}
				})
			}
		})
	}
}
