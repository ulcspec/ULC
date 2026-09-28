package sheet

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/completeness"
	"github.com/ulcspec/ULC/tools/validator/internal/findings"
	"github.com/ulcspec/ULC/tools/validator/internal/index"
	"github.com/ulcspec/ULC/tools/validator/internal/validate"
)

func claimsBundle(t *testing.T) string {
	t.Helper()
	bundle := t.TempDir()
	copyBundle(t, filepath.Join("testdata", "bundle-b"), bundle)
	return bundle
}

func setClaimsCell(t *testing.T, bundle, sheet, column, value string) {
	t.Helper()
	path := filepath.Join(bundle, sheet+".csv")
	rows := readCSVRows(t, path)
	position := -1
	for i, header := range rows[0] {
		if header == column {
			position = i
			break
		}
	}
	if position < 0 {
		rows[0] = append(rows[0], column)
		position = len(rows[0]) - 1
		for i := 1; i < len(rows); i++ {
			rows[i] = append(rows[i], "")
		}
	}
	rows[1][position] = value
	writeCSVRows(t, path, rows)
}

func claimsSchemaErrors(t *testing.T, input string) string {
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
		t.Fatal("invalid claim or bound passed schema validation")
	}
	var output bytes.Buffer
	if err := report.WriteText(&output, "record"); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestClaimsAndBoundsAcrossReaders(t *testing.T) {
	bundle := claimsBundle(t)
	setClaimsCell(t, bundle, "additional_rated_claims", "failure_percent", "0.1")
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			record := convertOne(t, input, PatternB, completeness.LevelStandard)
			for _, expected := range []struct {
				path     string
				value    float64
				unit     string
				operator string
			}{
				{"electrical.power_factor", 0.9, "", "gt"},
				{"electrical.thd_percent", 20, "percent", "lt"},
				{"photometry.max_surface_luminance_cd_per_m2", 1600, "cd/m2", "lt"},
			} {
				value, ok := getPath(record, expected.path)
				if !ok {
					t.Errorf("%s is missing", expected.path)
					continue
				}
				obj := value.(map[string]any)
				if asFloat(t, obj["value"]) != expected.value || obj["value_type"] != "rated" {
					t.Errorf("%s = %v", expected.path, obj)
				}
				if expected.unit == "" {
					if _, hasUnit := obj["unit"]; hasUnit {
						t.Errorf("%s has unexpected unit %v", expected.path, obj["unit"])
					}
				} else if obj["unit"] != expected.unit {
					t.Errorf("%s unit = %v, want %s", expected.path, obj["unit"], expected.unit)
				}
				operatorPath := expected.path + "_bound_operator"
				if expected.path == "photometry.max_surface_luminance_cd_per_m2" {
					operatorPath = "photometry.max_surface_luminance_bound_operator"
				}
				if operator, _ := getPath(record, operatorPath); operator != expected.operator {
					t.Errorf("%s = %v, want %s", operatorPath, operator, expected.operator)
				}
			}
			if headline, _ := getPath(record, "lumen_maintenance_luminaire.manufacturer_rated_claim.claim_type"); headline != "L90" {
				t.Errorf("headline claim type = %v, want L90", headline)
			}
			if basis, _ := getPath(record, "lumen_maintenance_luminaire.manufacturer_rated_claim.basis"); basis != "tm_21_reported" {
				t.Errorf("headline basis = %v", basis)
			}
			claims := arrayAt(t, record, "lumen_maintenance_luminaire.additional_rated_claims")
			if len(claims) != 1 {
				t.Fatalf("claims count = %d, want 1", len(claims))
			}
			claim := claims[0].(map[string]any)
			if claim["claim_type"] != "L70" || claim["basis"] != "tm_21_calculated" {
				t.Errorf("additional claim = %v", claim)
			}
			hours := claim["claimed_hours"].(map[string]any)
			if asFloat(t, hours["value"]) != 115000 || hours["unit"] != "h" || hours["value_type"] != "rated" {
				t.Errorf("claimed hours = %v", hours)
			}
			failure := claim["failure_percent"].(map[string]any)
			if asFloat(t, failure["value"]) != 0.1 || failure["unit"] != "percent" {
				t.Errorf("failure percent = %v", failure)
			}
		})
	}
}

func TestAdditionalClaimRowRefusalsAcrossReaders(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*testing.T, string)
		want []string
	}{
		{"missing claim type", func(t *testing.T, b string) { setClaimsCell(t, b, "additional_rated_claims", "claim_type", "") }, []string{"claim_type"}},
		{"L50 rated claim", func(t *testing.T, b string) { setClaimsCell(t, b, "additional_rated_claims", "claim_type", "L50") }, []string{"claim_type", "L50", "threshold crossed experimentally", "extended LM-80", "measured", "lumen_maintenance_package"}},
		{"missing claimed hours", func(t *testing.T, b string) { setClaimsCell(t, b, "additional_rated_claims", "claimed_hours", "") }, []string{"claimed_hours"}},
		{"measured claimed hours", func(t *testing.T, b string) {
			setClaimsCell(t, b, "additional_rated_claims", "claimed_hours__value_type", "measured")
		}, []string{"claimed_hours", "requires value_type=rated"}},
		{"missing headline", func(t *testing.T, b string) {
			setClaimsCell(t, b, "records", "lm_claim_type", "")
			setClaimsCell(t, b, "records", "lm_claimed_hours", "")
		}, []string{"lm_claim_type", "lm_claimed_hours"}},
		{"wrong family reference", func(t *testing.T, b string) {
			setClaimsCell(t, b, "additional_rated_claims", "claimed_hours__attestation_ref", "lm79_lumos_skyline_sr_ho")
		}, []string{"claimed_hours", "different evidence family"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := claimsBundle(t)
			test.edit(t, bundle)
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					_, err := Convert(input, Options{})
					if err == nil {
						t.Fatal("invalid additional claim converted")
					}
					for _, want := range append([]string{"additional_rated_claims", "row 1", "lumos-skyline-sr-ho-3000k"}, test.want...) {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("error %q does not contain %q", err, want)
						}
					}
				})
			}
		})
	}
}

func TestClaimBasisSchemaRefusalsAcrossReaders(t *testing.T) {
	for _, test := range []struct{ name, basis, want string }{
		{"missing", "", "basis"},
		{"unknown", "not_a_basis", "basis"},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := claimsBundle(t)
			setClaimsCell(t, bundle, "additional_rated_claims", "basis", test.basis)
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					output := claimsSchemaErrors(t, input)
					for _, want := range []string{"/lumen_maintenance_luminaire/additional_rated_claims/0", test.want} {
						if !strings.Contains(output, want) {
							t.Errorf("schema report does not contain %q:\n%s", want, output)
						}
					}
				})
			}
		})
	}
}

func TestBoundOperatorSchemaRefusalsAcrossReaders(t *testing.T) {
	for _, test := range []struct{ number, operator, location string }{
		{"power_factor", "power_factor_bound_operator", "/electrical"},
		{"thd_percent", "thd_percent_bound_operator", "/electrical"},
		{"max_surface_luminance_cd_per_m2", "max_surface_luminance_bound_operator", "/photometry"},
	} {
		t.Run(test.operator, func(t *testing.T) {
			bundle := claimsBundle(t)
			setClaimsCell(t, bundle, "records", test.number, "")
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					output := claimsSchemaErrors(t, input)
					for _, want := range []string{test.location, test.operator, test.number} {
						if !strings.Contains(output, want) {
							t.Errorf("schema report does not contain %q:\n%s", want, output)
						}
					}
				})
			}
		})
	}
}

func TestBoundNumbersWithoutOperatorsArePointEstimatesAcrossReaders(t *testing.T) {
	bundle := claimsBundle(t)
	for _, operator := range []string{
		"power_factor_bound_operator",
		"thd_percent_bound_operator",
		"max_surface_luminance_bound_operator",
	} {
		setClaimsCell(t, bundle, "records", operator, "")
	}
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			record := convertOne(t, input, PatternB, completeness.LevelStandard)
			for _, pair := range []struct{ number, operator string }{
				{"electrical.power_factor", "electrical.power_factor_bound_operator"},
				{"electrical.thd_percent", "electrical.thd_percent_bound_operator"},
				{"photometry.max_surface_luminance_cd_per_m2", "photometry.max_surface_luminance_bound_operator"},
			} {
				if _, present := getPath(record, pair.number); !present {
					t.Errorf("point estimate %s is missing", pair.number)
				}
				if value, present := getPath(record, pair.operator); present {
					t.Errorf("blank operator emitted %s = %v", pair.operator, value)
				}
			}
		})
	}
}

func TestUGRLowerBoundStillFailsSchemaAcrossReaders(t *testing.T) {
	bundle := claimsBundle(t)
	setClaimsCell(t, bundle, "records", "ugr_4h_8h", "10")
	setClaimsCell(t, bundle, "records", "ugr_4h_8h_bound_operator", "gt")
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			output := claimsSchemaErrors(t, input)
			if !strings.Contains(output, "/photometry/ugr_4h_8h_bound_operator") {
				t.Errorf("schema report lacks UGR operator location:\n%s", output)
			}
		})
	}
}

func TestOlderBundleOmitsNewFieldsAcrossReaders(t *testing.T) {
	bundle := claimsBundle(t)
	for _, name := range []string{"additional_rated_claims.csv", "spectral_power_distribution.csv"} {
		if err := os.Remove(filepath.Join(bundle, name)); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(bundle, "records.csv")
	rows := readCSVRows(t, path)
	newColumns := map[string]bool{
		"power_factor": true, "power_factor_bound_operator": true,
		"thd_percent": true, "thd_percent_bound_operator": true,
		"max_surface_luminance_cd_per_m2": true, "max_surface_luminance_bound_operator": true,
		"lm_claim_basis": true,
	}
	without := make([][]string, len(rows))
	for i, row := range rows {
		for j, cell := range row {
			if !newColumns[rows[0][j]] {
				without[i] = append(without[i], cell)
			}
		}
	}
	writeCSVRows(t, path, without)
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			record := convertOne(t, input, PatternB, completeness.LevelStandard)
			for _, path := range []string{
				"electrical.power_factor", "electrical.power_factor_bound_operator",
				"electrical.thd_percent", "electrical.thd_percent_bound_operator",
				"photometry.max_surface_luminance_cd_per_m2", "photometry.max_surface_luminance_bound_operator",
				"lumen_maintenance_luminaire.manufacturer_rated_claim.basis",
				"lumen_maintenance_luminaire.additional_rated_claims",
				"colorimetry.spectral_power_distribution",
			} {
				if value, present := getPath(record, path); present {
					t.Errorf("older bundle emitted %s = %v", path, value)
				}
			}
		})
	}
}
