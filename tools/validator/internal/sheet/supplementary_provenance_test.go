package sheet

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/completeness"
	"github.com/ulcspec/ULC/tools/validator/internal/findings"
	"github.com/ulcspec/ULC/tools/validator/internal/index"
	"github.com/ulcspec/ULC/tools/validator/internal/validate"
)

func TestSupplementaryValueColumnTableIsExact(t *testing.T) {
	want := map[supplementaryValueKey]struct {
		unit, unitColumn string
		family           attestationFamily
	}{
		{sheet: "alpha_opic", field: "melanopic_der"}:                         {unit: "ratio", family: attestationFamilyMelanopic},
		{sheet: "alpha_opic", field: "efficacy"}:                              {unit: "ratio", family: attestationFamilyMelanopic},
		{sheet: "flicker_metrics", field: "value"}:                            {unit: "ratio", unitColumn: "unit", family: attestationFamilyFlicker},
		{sheet: "lumen_maintenance_package", field: "tm_21_projection_hours"}: {unit: "h", family: attestationFamilyMaintenance},
		{sheet: "lumen_maintenance_package", field: "test_hours"}:             {unit: "h", family: attestationFamilyMaintenance},
		{sheet: "lumen_maintenance_package", field: "drive_current_ma"}:       {unit: "mA", family: attestationFamilyMaintenance},
		{sheet: "zonal_lumens", field: "lumens"}:                              {unit: "lm", family: attestationFamilyPhotometric},
		{sheet: "lcs_zonal_lumens", field: "lumens"}:                          {unit: "lm", family: attestationFamilyPhotometric},
	}
	if len(supplementaryValueColumns) != len(want) {
		t.Fatalf("supplementary value table has %d rows, want %d", len(supplementaryValueColumns), len(want))
	}
	for key, expected := range want {
		got, ok := supplementaryValueColumns[key]
		if !ok {
			t.Errorf("supplementary value table missing %#v", key)
			continue
		}
		if got.unit != expected.unit || got.unitColumn != expected.unitColumn || got.defaults.family != expected.family {
			t.Errorf("supplementary value table %#v = unit %q, unit column %q, family %q; want %q, %q, %q", key, got.unit, got.unitColumn, got.defaults.family, expected.unit, expected.unitColumn, expected.family)
		}
	}
}

// TestSupplementaryProvenanceDefaults pins the exact defaults that predate the
// shared resolver. The resolver refactor must preserve these values.
func TestSupplementaryProvenanceDefaults(t *testing.T) {
	record := convertOne(t, filepath.Join("testdata", "bundle-b"), PatternB, completeness.LevelStandard)

	melDER, _ := getPath(record, "alpha_opic_metrics.melanopic_der")
	assertProvenanceDefaults(t, "alpha_opic.melanopic_der", melDER, "ratio", "rated", "datasheet_pdf", "extracted", "")
	channels := arrayAt(t, record, "alpha_opic_metrics.per_channel")
	channel, _ := channels[0].(map[string]any)
	assertProvenanceDefaults(t, "alpha_opic.efficacy", channel["efficacy"], "ratio", "rated", "datasheet_pdf", "extracted", "")

	metrics := arrayAt(t, record, "flicker_measurements.metrics")
	for i, value := range metrics {
		metric, _ := value.(map[string]any)
		assertProvenanceDefaults(t, "flicker_metrics.value", metric["value"], "ratio", "rated", "datasheet_pdf", "extracted", "")
		if metric["bound_operator"] != "lte" {
			t.Errorf("flicker metric %d bound_operator = %v, want lte", i, metric["bound_operator"])
		}
	}

	packages := arrayAt(t, record, "lumen_maintenance_package")
	pkg, _ := packages[0].(map[string]any)
	assertProvenanceDefaults(t, "lumen_maintenance_package.tm_21_projection_hours", pkg["tm_21_projection_hours"], "h", "rated", "manufacturer_direct", "transcribed", "")

	zones := arrayAt(t, record, "photometry.zonal_lumens")
	zone, _ := zones[0].(map[string]any)
	assertProvenanceDefaults(t, "zonal_lumens.lumens", zone["lumens"], "lm", "measured", "ies", "extracted", "lm79_lumos_skyline_sr_ho")
	lcsZones := arrayAt(t, record, "outdoor_classification.lcs_zonal_lumens")
	lcsZone, _ := lcsZones[0].(map[string]any)
	assertProvenanceDefaults(t, "lcs_zonal_lumens.lumens", lcsZone["lumens"], "lm", "measured", "ies", "extracted", "lm79_lumos_skyline_sr_ho")
}

func assertProvenanceDefaults(t *testing.T, name string, value any, unit, valueType, source, method, attestationRef string) {
	t.Helper()
	obj, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %T, want ProvenancedNumber", name, value)
	}
	if got := obj["unit"]; got != unit {
		t.Errorf("%s unit = %v, want %q", name, got, unit)
	}
	if got := obj["value_type"]; got != valueType {
		t.Errorf("%s value_type = %v, want %q", name, got, valueType)
	}
	prov, ok := obj["provenance"].(map[string]any)
	if !ok {
		t.Fatalf("%s provenance = %T, want object", name, obj["provenance"])
	}
	if got := prov["source"]; got != source {
		t.Errorf("%s provenance.source = %v, want %q", name, got, source)
	}
	if got := prov["method"]; got != method {
		t.Errorf("%s provenance.method = %v, want %q", name, got, method)
	}
	if got, _ := prov["attestation_ref"].(string); got != attestationRef {
		t.Errorf("%s provenance.attestation_ref = %q, want %q", name, got, attestationRef)
	}
}

func supplementaryBundleWithColumns(t *testing.T, sheet string, columns map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	copyBundle(t, filepath.Join("testdata", "bundle-b"), dir)
	path := filepath.Join(dir, sheet+".csv")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	names := make([]string, 0, len(columns))
	for name := range columns {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if strings.ContainsAny(columns[name], ",\"") {
			t.Fatalf("test column %q has unsafe raw CSV value %q", name, columns[name])
		}
		lines[0] += "," + name
		for i := 1; i < len(lines); i++ {
			lines[i] += "," + columns[name]
		}
	}
	writeFile(t, path, strings.Join(lines, "\n")+"\n")
	return dir
}

func supplementaryInputs(t *testing.T, bundle string) map[string]string {
	t.Helper()
	xlsx := filepath.Join(bundle, "supplementary.xlsx")
	buildXLSX(t, xlsx, bundleToXLSXSheets(t, bundle))
	return map[string]string{"CSV": bundle, "XLSX": xlsx}
}

func supplementaryTestValue(t *testing.T, record map[string]any, sheet string) any {
	t.Helper()
	switch sheet {
	case "alpha_opic":
		value, _ := getPath(record, "alpha_opic_metrics.melanopic_der")
		return value
	case "flicker_metrics":
		metric, _ := arrayAt(t, record, "flicker_measurements.metrics")[0].(map[string]any)
		return metric["value"]
	case "lumen_maintenance_package":
		pkg, _ := arrayAt(t, record, "lumen_maintenance_package")[0].(map[string]any)
		return pkg["tm_21_projection_hours"]
	default:
		t.Fatalf("unknown test sheet %q", sheet)
		return nil
	}
}

func TestSupplementaryProvenanceOverridesAcrossReaders(t *testing.T) {
	tests := []struct {
		sheet, field, source, method string
	}{
		{sheet: "alpha_opic", field: "melanopic_der", source: "manufacturer_direct", method: "transcribed"},
		{sheet: "flicker_metrics", field: "value", source: "manufacturer_direct", method: "transcribed"},
		{sheet: "lumen_maintenance_package", field: "tm_21_projection_hours", source: "datasheet_pdf", method: "extracted"},
	}
	for _, test := range tests {
		bundle := supplementaryBundleWithColumns(t, test.sheet, map[string]string{
			test.field + "__value_type":  "nominal",
			test.field + "__prov_source": test.source,
			test.field + "__prov_method": test.method,
		})
		for reader, input := range supplementaryInputs(t, bundle) {
			t.Run(test.sheet+"/"+reader, func(t *testing.T) {
				record := convertOne(t, input, PatternB, completeness.LevelStandard)
				assertProvenanceDefaults(t, test.sheet, supplementaryTestValue(t, record, test.sheet),
					supplementaryValueColumns[supplementaryValueKey{sheet: test.sheet, field: test.field}].unit,
					"nominal", test.source, test.method, "")
			})
		}
	}
}

func TestFlickerBoundOperatorSurvivesBothReaders(t *testing.T) {
	bundle := supplementaryBundleWithColumns(t, "flicker_metrics", map[string]string{})
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			record := convertOne(t, input, PatternB, completeness.LevelStandard)
			for i, value := range arrayAt(t, record, "flicker_measurements.metrics") {
				metric, _ := value.(map[string]any)
				if metric["bound_operator"] != "lte" {
					t.Errorf("metric %d bound_operator = %v, want authored lte", i, metric["bound_operator"])
				}
			}
		})
	}
}

func TestUnsupportedSupplementaryProvenanceStillFailsSchemaValidation(t *testing.T) {
	const unsupported = "not_a_provenance_source"
	bundle := supplementaryBundleWithColumns(t, "flicker_metrics", map[string]string{"value__prov_source": unsupported})
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			results, err := Convert(input, Options{})
			if err != nil {
				t.Fatalf("converter copied the enum domain instead of passing the token to schema validation: %v", err)
			}
			record := results[0].Record
			metric, _ := arrayAt(t, record, "flicker_measurements.metrics")[0].(map[string]any)
			value, _ := metric["value"].(map[string]any)
			provenance, _ := value["provenance"].(map[string]any)
			if got := provenance["source"]; got != unsupported {
				t.Fatalf("converter changed unsupported provenance source to %v", got)
			}
			record["index"] = index.Build(record)
			validator, err := validate.NewValidator(schemaDir(t))
			if err != nil {
				t.Fatal(err)
			}
			report := findings.NewReport()
			validator.Validate(numberTree(t, record), report)
			report.Finalize()
			if !report.HasErrors() {
				t.Fatal("unsupported supplementary provenance token passed schema validation")
			}
			var output bytes.Buffer
			if err := report.WriteText(&output, "record"); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"/flicker_measurements/metrics/0/value/provenance/source"} {
				if !strings.Contains(output.String(), want) {
					t.Errorf("schema report does not contain %q:\n%s", want, output.String())
				}
			}
		})
	}
}
