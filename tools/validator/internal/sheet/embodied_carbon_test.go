package sheet

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
	"github.com/ulcspec/ULC/tools/validator/internal/index"
	"github.com/ulcspec/ULC/tools/validator/internal/validate"
)

func TestEmbodiedCarbonColumnsOnTheAttestationsSheet(t *testing.T) {
	bundle := func(cells map[string]string) string {
		dir := bundleWithColumns(t, map[string]string{})
		path := filepath.Join(dir, "attestations.csv")
		rows := readCSVRows(t, path)
		rows[0] = append(rows[0], "embodied_carbon_kgco2e", "embodied_carbon_scope", "embodied_carbon_functional_unit", "method_variant")
		writeCSVRows(t, path, rows[:1])
		row := map[string]string{"attestation_id": "tm65-carbon", "program": "tm65", "status": "claimed", "value_type": "rated"}
		for key, value := range cells {
			row[key] = value
		}
		setColourAttestations(t, dir, colourAttestation("photometry-test", "lm_79"), row)
		return dir
	}
	attestation := func(record map[string]any) map[string]any {
		for _, item := range record["attestations"].([]any) {
			att := item.(map[string]any)
			if att["attestation_id"] == "tm65-carbon" {
				return att
			}
		}
		t.Fatal("TM65 attestation missing")
		return nil
	}
	t.Run("complete figure", func(t *testing.T) {
		dir := bundle(map[string]string{"embodied_carbon_kgco2e": "43.82", "embodied_carbon_scope": "a1_a4_b3_c2_c4", "embodied_carbon_functional_unit": "one luminaire", "method_variant": "TM65NA"})
		for reader, input := range supplementaryInputs(t, dir) {
			t.Run(reader, func(t *testing.T) {
				record := convertOneRecord(t, input, Options{}).Record
				want := map[string]any{"embodied_carbon_kgco2e": 43.82, "embodied_carbon_scope": "a1_a4_b3_c2_c4", "embodied_carbon_functional_unit": "one luminaire", "method_variant": "TM65NA"}
				if got := attestation(record)["sustainability_metric"]; !reflect.DeepEqual(got, want) {
					t.Fatalf("metric = %#v, want %#v", got, want)
				}
				wantSchemaValid(t, record)
				themeValue, ok := getPath(record, "index.achievements.themes.embodied_carbon")
				if !ok {
					t.Fatal("embodied carbon theme missing")
				}
				theme := themeValue.(map[string]any)
				if theme["state"] != "claimed" || theme["best_metric_ref"] != "tm65-carbon" {
					t.Fatalf("embodied carbon theme = %v", theme)
				}
			})
		}
	})
	t.Run("figure needs companions", func(t *testing.T) {
		record := convertOneRecord(t, bundle(map[string]string{"embodied_carbon_kgco2e": "43.82"}), Options{}).Record
		record["index"] = index.Build(record)
		v, err := validate.NewValidator(schemaDir(t))
		if err != nil {
			t.Fatal(err)
		}
		report := findings.NewReport()
		v.Validate(numberTree(t, record), report)
		var output bytes.Buffer
		if err := report.WriteText(&output, "record"); err != nil {
			t.Fatal(err)
		}
		want := "missing properties 'embodied_carbon_scope', 'embodied_carbon_functional_unit'"
		if !report.HasErrors() || !strings.Contains(output.String(), want) {
			t.Fatalf("schema findings = %s, want %q", output.String(), want)
		}
	})
	t.Run("invalid figure refused", func(t *testing.T) {
		_, err := Convert(bundle(map[string]string{"embodied_carbon_kgco2e": "about 44"}), Options{})
		want := "embodied_carbon_kgco2e: invalid number \"about 44\""
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("Convert error = %v, want %q", err, want)
		}
	})
	t.Run("blank cells omitted", func(t *testing.T) {
		record := convertOneRecord(t, bundle(map[string]string{}), Options{}).Record
		if got, exists := attestation(record)["sustainability_metric"]; exists {
			t.Fatalf("blank cells emitted metric: %v", got)
		}
	})
}
