package sheet

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/completeness"
	"github.com/ulcspec/ULC/tools/validator/internal/findings"
	"github.com/ulcspec/ULC/tools/validator/internal/index"
	"github.com/ulcspec/ULC/tools/validator/internal/validate"
)

func opennessBundle(t *testing.T, edit func([][]string)) string {
	t.Helper()
	bundle := t.TempDir()
	copyBundle(t, filepath.Join("testdata", "bundle-b"), bundle)
	if edit != nil {
		path := filepath.Join(bundle, "customization_openness.csv")
		rows := readCSVRows(t, path)
		edit(rows)
		writeCSVRows(t, path, rows)
	}
	return bundle
}

func TestCustomizationOpennessAcrossReaders(t *testing.T) {
	bundle := opennessBundle(t, nil)
	want := []any{
		map[string]any{"axis": "finish", "statement": "Custom finishes on request.", "published_in_ref": "lumos-skyline-area-ss.pdf"},
		map[string]any{"axis": "dimensions", "statement": "Custom lengths on request.", "contact_reference": "factory engineering desk"},
		map[string]any{"axis": "other", "axis_label": "packaging", "statement": "Project packaging on request.", "contact_reference": "factory engineering desk"},
	}
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			record := convertOne(t, input, PatternB, completeness.LevelStandard)
			got, ok := getPath(record, "product_family.customization_openness")
			if !ok || !reflect.DeepEqual(got, want) {
				t.Errorf("customization_openness = %#v, want %#v", got, want)
			}
		})
	}
}

func TestCustomizationOpennessRowRefusalsAcrossReaders(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func([][]string)
		want []string
	}{
		{"missing axis", func(rows [][]string) { rows[1][1] = "" }, []string{"customization_openness", "row 1", "lumos-skyline-sr-ho-3000k", "missing axis"}},
		{"duplicate finish", func(rows [][]string) { rows[2][1] = "finish" }, []string{"customization_openness", "rows 1 and 2", "lumos-skyline-sr-ho-3000k", "duplicate axis"}},
		{"duplicate other label", func(rows [][]string) { rows[2][1], rows[2][2] = "other", " Packaging " }, []string{"customization_openness", "rows 2 and 3", "lumos-skyline-sr-ho-3000k", "duplicate other axis_label"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := opennessBundle(t, test.edit)
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					_, err := Convert(input, Options{})
					if err == nil {
						t.Fatal("invalid openness row converted")
					}
					for _, part := range test.want {
						if !strings.Contains(err.Error(), part) {
							t.Errorf("error %q lacks %q", err, part)
						}
					}
				})
			}
		})
	}
}

func opennessSchemaErrors(t *testing.T, input string) string {
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
		t.Fatal("invalid openness entry passed schema validation")
	}
	var output bytes.Buffer
	if err := report.WriteText(&output, "record"); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestCustomizationOpennessSchemaRefusalsAcrossReaders(t *testing.T) {
	for _, test := range []struct {
		name     string
		edit     func([][]string)
		location string
	}{
		{"unknown axis", func(rows [][]string) { rows[1][1] = "unknown_axis" }, "/product_family/customization_openness/0/axis"},
		{"no reference", func(rows [][]string) { rows[1][4], rows[1][5] = "", "" }, "/product_family/customization_openness/0"},
		{"other without label", func(rows [][]string) { rows[3][2] = "" }, "/product_family/customization_openness/2"},
		{"label too long", func(rows [][]string) { rows[3][2] = strings.Repeat("x", 81) }, "/product_family/customization_openness/2/axis_label"},
		{"statement too long", func(rows [][]string) { rows[1][3] = strings.Repeat("x", 201) }, "/product_family/customization_openness/0/statement"},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := opennessBundle(t, test.edit)
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					output := opennessSchemaErrors(t, input)
					if !strings.Contains(output, test.location) {
						t.Errorf("schema report lacks %q:\n%s", test.location, output)
					}
				})
			}
		})
	}
}

func TestPreviousWorkbookWithoutCustomizationOpennessAcrossReaders(t *testing.T) {
	bundle := opennessBundle(t, nil)
	if err := os.Remove(filepath.Join(bundle, "customization_openness.csv")); err != nil {
		t.Fatal(err)
	}
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			record := convertOne(t, input, PatternB, completeness.LevelStandard)
			if _, exists := getPath(record, "product_family.customization_openness"); exists {
				t.Error("older workbook gained customization_openness")
			}
		})
	}
}
