package sheet

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCompanionHeaderSuffixTableIsExact(t *testing.T) {
	want := map[string][]string{
		CompanionFamilyProvenance: {
			"__value_type", "__prov_source", "__prov_method", "__extension_method", "__base_attestation_ref", "__attestation_ref",
		},
		CompanionFamilyRevision: {"__revision_label", "__revision_date"},
	}
	if !reflect.DeepEqual(CompanionHeaderSuffixes, want) {
		t.Errorf("companion suffix table = %#v, want %#v", CompanionHeaderSuffixes, want)
	}
}

func TestBlankUnsupportedCompanionHeaderIsRefusedAcrossReaders(t *testing.T) {
	tests := []struct {
		name, header string
		want         []string
	}{
		{
			name:   "known base, unsupported suffix",
			header: "value__prov_sorce",
			want:   []string{"flicker_metrics", "value__prov_sorce", "value", "__prov_source"},
		},
		{
			name:   "unknown base",
			header: "valu__prov_source",
			want:   []string{"flicker_metrics", "valu__prov_source", "legal bases", "value"},
		},
	}
	for _, test := range tests {
		bundle := supplementaryBundleWithColumns(t, "flicker_metrics", map[string]string{test.header: ""})
		for reader, input := range supplementaryInputs(t, bundle) {
			t.Run(test.name+"/"+reader, func(t *testing.T) {
				_, err := Convert(input, Options{})
				if err == nil {
					t.Fatalf("blank unsupported companion header %q was ignored", test.header)
				}
				for _, want := range test.want {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
			})
		}
	}
}

func TestImperialProvenanceCompanionIsLegalAcrossReaders(t *testing.T) {
	bundle := bundleWithColumns(t, map[string]string{"overall_diameter_in__prov_source": ""})
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			if _, err := Convert(input, Options{}); err != nil {
				t.Errorf("Imperial provenance companion was refused: %v", err)
			}
		})
	}
}

func writeTemplateHeaderBundle(t *testing.T) string {
	t.Helper()
	repoRoot := filepath.Dir(schemaDir(t))
	templates, err := ReadCSVBundle(filepath.Join(repoRoot, "templates", "workbook"))
	if err != nil {
		t.Fatalf("read workbook templates: %v", err)
	}
	if len(templates.Headers) != 16 {
		t.Fatalf("workbook template carries %d CSV sheets, want 16", len(templates.Headers))
	}
	sourceDir := filepath.Join("testdata", "bundle-b")
	source, err := ReadCSVBundle(sourceDir)
	if err != nil {
		t.Fatalf("read source fixture: %v", err)
	}
	out := t.TempDir()
	copyBundle(t, sourceDir, out)
	for sheet, header := range templates.Headers {
		path := filepath.Join(out, sheet+".csv")
		file, err := os.Create(path)
		if err != nil {
			t.Fatalf("create %s: %v", path, err)
		}
		writer := csv.NewWriter(file)
		if err := writer.Write(header); err != nil {
			file.Close()
			t.Fatalf("write %s header: %v", sheet, err)
		}
		for _, row := range source.Rows[sheet] {
			values := make([]string, len(header))
			for i, column := range header {
				values[i] = row[column]
			}
			if err := writer.Write(values); err != nil {
				file.Close()
				t.Fatalf("write %s row: %v", sheet, err)
			}
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			file.Close()
			t.Fatalf("flush %s: %v", sheet, err)
		}
		if err := file.Close(); err != nil {
			t.Fatalf("close %s: %v", sheet, err)
		}
	}
	return out
}

func TestAllShippedTemplateHeadersConvert(t *testing.T) {
	bundle := writeTemplateHeaderBundle(t)
	workbook, err := ReadCSVBundle(bundle)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("shipped template headers checked: %d sheets", len(workbook.Headers))
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			if _, err := Convert(input, Options{}); err != nil {
				t.Fatalf("bundle carrying all shipped template headers did not convert: %v", err)
			}
		})
	}
}

func TestEveryConverterTestBundlePassesCompanionHeaderCheck(t *testing.T) {
	for _, name := range []string{"bundle", "bundle-b", "bundle-c", "bundle-d"} {
		t.Run(name+"/CSV", func(t *testing.T) {
			if _, err := Convert(filepath.Join("testdata", name), Options{}); err != nil {
				t.Fatalf("Convert: %v", err)
			}
		})
		t.Run(name+"/XLSX", func(t *testing.T) {
			copy := t.TempDir()
			copyBundle(t, filepath.Join("testdata", name), copy)
			xlsx := filepath.Join(copy, "bundle.xlsx")
			buildXLSX(t, xlsx, bundleToXLSXSheets(t, copy))
			if _, err := Convert(xlsx, Options{}); err != nil {
				t.Fatalf("Convert: %v", err)
			}
		})
	}
}
