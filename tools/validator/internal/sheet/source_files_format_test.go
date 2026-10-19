package sheet

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	embedded "github.com/ulcspec/ULC/schema"
	"github.com/ulcspec/ULC/tools/validator/internal/completeness"
)

func formatBundle(t *testing.T, fileType, filename, format string) string {
	t.Helper()
	bundle := t.TempDir()
	copyBundle(t, filepath.Join("testdata", "bundle"), bundle)
	path := filepath.Join(bundle, "source_files.csv")
	rows := readCSVRows(t, path)
	rows[0] = append(rows[0], "photometry_format")
	for i := 1; i < len(rows); i++ {
		rows[i] = append(rows[i], "")
	}
	if fileType == "" {
		rows[1][len(rows[0])-1] = format
	} else {
		writeFile(t, filepath.Join(bundle, filename), "Synthetic source file.\n")
		row := make([]string, len(rows[0]))
		for i, header := range rows[0] {
			row[i] = map[string]string{"record_id": "acme-orbit-1200-4000k", "file_type": fileType, "filename": filename, "photometry_format": format}[header]
		}
		rows = append(rows, row)
	}
	writeCSVRows(t, path, rows)
	return bundle
}

func TestSourceFilesPhotometryFormatAcrossReaders(t *testing.T) {
	for _, tc := range []struct{ fileType, filename, format string }{
		{"", "", "lm63_2002"}, {"", "", "lm63_2019"},
		{"ldt", "acme-orbit-1200-4000k.ldt", "eulumdat"},
		{"tm33", "acme-orbit-1200-4000k.xml", "tm33_2023"},
	} {
		t.Run(tc.format, func(t *testing.T) {
			bundle := formatBundle(t, tc.fileType, tc.filename, tc.format)
			var first map[string]any
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					results, err := Convert(input, Options{})
					if err != nil {
						t.Fatal(err)
					}
					rec := results[0].Record
					found := false
					for _, item := range rec["source_files"].([]any) {
						entry := item.(map[string]any)
						if entry["photometry_format"] == tc.format {
							found = true
						}
					}
					if !found {
						t.Fatalf("format %q absent", tc.format)
					}
					for _, item := range completeness.Compute(rec).Enrichment {
						if item.Path == "/source_files[]/photometry_format" {
							t.Fatalf("format enrichment gap remains: %+v", item)
						}
					}
					wantSchemaValid(t, rec)
					if first == nil {
						first = rec
					} else if !reflect.DeepEqual(first, rec) {
						t.Fatal("CSV and XLSX records differ")
					}
				})
			}
		})
	}
}

func TestSourceFilesBlankPhotometryFormatIsAbsent(t *testing.T) {
	bundle := formatBundle(t, "", "", "")
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			results, err := Convert(input, Options{})
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range results[0].Record["source_files"].([]any) {
				if _, exists := item.(map[string]any)["photometry_format"]; exists {
					t.Fatal("blank format must be absent")
				}
			}
		})
	}
}

func TestSourceFilesPhotometryFormatRefusalsAcrossReaders(t *testing.T) {
	for _, tc := range []struct{ fileType, filename, format, want string }{
		{"supplementary_pdf", "acme-orbit-notes.pdf", "lm63_2002", `source_files row "acme-orbit-notes.pdf": photometry_format "lm63_2002" is allowed only on file_type ies, ldt or tm33, not "supplementary_pdf"`},
		{"", "", "lm63_2003", `source_files row "acme-orbit-1200-4000k.ies": photometry_format "lm63_2003" is not a PhotometryFormat token`},
		{"", "", "IESNA:LM-63-2002", `source_files row "acme-orbit-1200-4000k.ies": photometry_format "IESNA:LM-63-2002" is not a PhotometryFormat token`},
		{"ldt", "acme-orbit-1200-4000k.ldt", "lm63_2002", `source_files row "acme-orbit-1200-4000k.ldt": photometry_format "lm63_2002" belongs to file_type ies, not "ldt"`},
		{"tm33", "acme-orbit-1200-4000k.xml", "eulumdat", `source_files row "acme-orbit-1200-4000k.xml": photometry_format "eulumdat" belongs to file_type ldt, not "tm33"`},
		{"", "", "tm33_2023", `source_files row "acme-orbit-1200-4000k.ies": photometry_format "tm33_2023" belongs to file_type tm33, not "ies"`},
	} {
		t.Run(tc.format+tc.fileType, func(t *testing.T) {
			for reader, input := range supplementaryInputs(t, formatBundle(t, tc.fileType, tc.filename, tc.format)) {
				t.Run(reader, func(t *testing.T) {
					_, err := Convert(input, Options{})
					if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), `record "acme-orbit-1200-4000k"`) {
						t.Fatalf("refusal = %v, want %s and record id", err, tc.want)
					}
				})
			}
		})
	}
}

func TestPhotometryFormatFileTypesMatchSchema(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal(embedded.ULCSchemaJSON, &schema); err != nil {
		t.Fatal(err)
	}
	source := schema["$defs"].(map[string]any)["SourceFile"].(map[string]any)
	properties := source["then"].(map[string]any)["properties"].(map[string]any)
	enum := properties["file_type"].(map[string]any)["enum"].([]any)
	want := map[string]bool{}
	for _, token := range enum {
		want[fmt.Sprint(token)] = true
	}
	if !reflect.DeepEqual(photometryFormatFileTypes, want) {
		t.Fatalf("format file types = %v, schema = %v", photometryFormatFileTypes, want)
	}
}

func TestPhotometryFormatTokensMatchFileTypeFamilies(t *testing.T) {
	var taxonomy struct {
		Defs map[string]struct {
			Enum []string `json:"enum"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(embedded.TaxonomySchemaJSON, &taxonomy); err != nil {
		t.Fatal(err)
	}
	tokens := taxonomy.Defs["PhotometryFormat"].Enum
	if len(tokens) == 0 {
		t.Fatal("PhotometryFormat enum is empty")
	}
	families := map[string]bool{}
	for _, token := range tokens {
		fileType := photometryFormatFileType(token)
		if !photometryFormatFileTypes[fileType] {
			t.Errorf("PhotometryFormat token %q has no unique supported file-type family", token)
		}
		families[fileType] = true
	}
	if !reflect.DeepEqual(families, photometryFormatFileTypes) {
		t.Fatalf("format families = %v, want all supported types %v", families, photometryFormatFileTypes)
	}
}
