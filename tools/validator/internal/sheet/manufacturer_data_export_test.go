package sheet

import (
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/completeness"
	"github.com/ulcspec/ULC/tools/validator/internal/findings"
	"github.com/ulcspec/ULC/tools/validator/internal/index"
	"github.com/ulcspec/ULC/tools/validator/internal/validate"
)

const (
	exportToken    = "manufacturer_data_export"
	exportFilename = "published-performance-data.csv"
)

// TestManufacturerDataExportAcrossWorkbookFormats proves that both workbook
// readers preserve the new source-file and provenance semantics. Each leg
// copies the shared bundle before changing it so no test can alter the fixture
// seen by another package.
func TestManufacturerDataExportAcrossWorkbookFormats(t *testing.T) {
	type semantics struct {
		Reference  map[string]any
		Provenance map[string]any
		Grade      completeness.Level
	}

	got := map[string]semantics{}
	for _, shape := range []string{"csv", "xlsx"} {
		t.Run(shape, func(t *testing.T) {
			dir := t.TempDir()
			writeFixtureCopy(t, dir)

			baselineInput := dir
			if shape == "xlsx" {
				baselineInput = filepath.Join(dir, "baseline.xlsx")
				buildXLSX(t, baselineInput, bundleToXLSXSheets(t, dir))
			}
			baseline, err := Convert(baselineInput, Options{})
			if err != nil {
				t.Fatalf("convert baseline %s fixture: %v", shape, err)
			}
			if len(baseline) != 1 {
				t.Fatalf("baseline %s results = %d, want 1", shape, len(baseline))
			}
			baselineGrade := completeness.AchievedLevel(baseline[0].Record)

			appendCSVRow(t, filepath.Join(dir, "source_files.csv"), []string{
				"acme-orbit-1200-4000k",
				exportToken,
				exportFilename,
				"2026-09-13",
			})
			appendCSVColumn(t, filepath.Join(dir, "records.csv"), "input_power_w__prov_source", exportToken)

			exportBytes := []byte("synthetic published performance data\n")
			exportPath := filepath.Join(dir, exportFilename)
			if err := os.WriteFile(exportPath, exportBytes, 0o644); err != nil {
				t.Fatalf("write runtime export: %v", err)
			}

			input := dir
			if shape == "xlsx" {
				input = filepath.Join(dir, "with-export.xlsx")
				buildXLSX(t, input, bundleToXLSXSheets(t, dir))
			}
			results, err := Convert(input, Options{})
			if err != nil {
				t.Fatalf("convert %s fixture with export: %v", shape, err)
			}
			if len(results) != 1 {
				t.Fatalf("%s results = %d, want 1", shape, len(results))
			}
			record := results[0].Record
			if gotGrade := completeness.AchievedLevel(record); gotGrade != baselineGrade {
				t.Fatalf("%s grade moved from %s to %s after adding export provenance", shape, baselineGrade, gotGrade)
			}

			exportRef, exportIndex := findSourceFileReference(t, record, exportToken)
			sum := sha256.Sum256(exportBytes)
			if gotSum := exportRef["sha256"]; gotSum != hex.EncodeToString(sum[:]) {
				t.Errorf("%s export sha256 = %v, want converter hash of runtime bytes", shape, gotSum)
			}
			if gotName := exportRef["filename"]; gotName != exportFilename {
				t.Errorf("%s export filename = %v, want %q", shape, gotName, exportFilename)
			}

			electrical, _ := record["electrical"].(map[string]any)
			power, _ := electrical["input_power_w"].(map[string]any)
			provenance, _ := power["provenance"].(map[string]any)
			if gotSource := provenance["source"]; gotSource != exportToken {
				t.Errorf("%s input power provenance source = %v, want %q", shape, gotSource, exportToken)
			}

			// Withholding happens after conversion, so the record retains the real
			// converter-generated hash while the publication bytes are absent.
			if err := os.Remove(exportPath); err != nil {
				t.Fatalf("remove runtime export: %v", err)
			}
			record["index"] = index.Build(record)
			validator, err := validate.NewValidator(schemaDir(t))
			if err != nil {
				t.Fatalf("NewValidator: %v", err)
			}
			report := findings.NewReport()
			validator.Validate(numberTree(t, record), report)
			validate.VerifyFileReferences(dir, record, validate.VerifyOptions{}, report)
			report.Finalize()
			if report.HasErrors() {
				t.Fatalf("%s withheld-export record must validate: %v", shape, report.Findings)
			}
			if len(report.Findings) != 1 {
				t.Fatalf("%s withheld-export findings = %d, want exactly 1: %v", shape, len(report.Findings), report.Findings)
			}
			finding := report.Findings[0]
			wantPath := "/source_files/" + strconv.Itoa(exportIndex) + "/reference"
			if finding.Level != findings.LevelInfo || finding.Code != findings.CodeSourceFileNotFound || finding.Path != wantPath {
				t.Errorf("%s withheld-export finding = %+v, want INFO %s at %s", shape, finding, findings.CodeSourceFileNotFound, wantPath)
			}
			built, _ := record["index"].(map[string]any)
			if grade := built["conformance_level"]; grade != baselineGrade.String() {
				t.Errorf("%s built grade = %v, want unchanged %s", shape, grade, baselineGrade)
			}

			got[shape] = semantics{Reference: exportRef, Provenance: provenance, Grade: baselineGrade}
		})
	}

	if !reflect.DeepEqual(got["csv"], got["xlsx"]) {
		t.Errorf("export semantics differ by workbook format:\nCSV:  %#v\nXLSX: %#v", got["csv"], got["xlsx"])
	}
}

func appendCSVRow(t *testing.T, path string, row []string) {
	t.Helper()
	rows := readCSVRows(t, path)
	rows = append(rows, row)
	writeCSVRows(t, path, rows)
}

func appendCSVColumn(t *testing.T, path, header, value string) {
	t.Helper()
	rows := readCSVRows(t, path)
	if len(rows) != 2 {
		t.Fatalf("%s rows = %d, want one header and one fixture row", path, len(rows))
	}
	rows[0] = append(rows[0], header)
	rows[1] = append(rows[1], value)
	writeCSVRows(t, path, rows)
}

func readCSVRows(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return rows
}

func writeCSVRows(t *testing.T, path string, rows [][]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	w := csv.NewWriter(f)
	if err := w.WriteAll(rows); err != nil {
		_ = f.Close()
		t.Fatalf("write %s: %v", path, err)
	}
	w.Flush()
	if err := w.Error(); err != nil {
		_ = f.Close()
		t.Fatalf("flush %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}

func findSourceFileReference(t *testing.T, record map[string]any, fileType string) (map[string]any, int) {
	t.Helper()
	files, _ := record["source_files"].([]any)
	for i, raw := range files {
		entry, _ := raw.(map[string]any)
		if entry["file_type"] == fileType {
			ref, _ := entry["reference"].(map[string]any)
			return ref, i
		}
	}
	t.Fatalf("no source_files entry with file_type %q", fileType)
	return nil, -1
}
