package sheet

import (
	"path/filepath"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/completeness"
	"github.com/ulcspec/ULC/tools/validator/internal/index"
)

// lineCardBundle changes an existing master-row cell in the reference bundle.
func lineCardBundle(t *testing.T, column, value string) string {
	t.Helper()
	dir := t.TempDir()
	copyBundle(t, filepath.Join("testdata", "bundle"), dir)
	path := filepath.Join(dir, "records.csv")
	rows := readCSVRows(t, path)
	found := false
	for i, header := range rows[0] {
		if header == column {
			rows[1][i] = value
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("fixture missing column %s", column)
	}
	writeCSVRows(t, path, rows)
	return dir
}

func TestLigatureResistantConvertsAtTheSameGrade(t *testing.T) {
	base := convertOneRecord(t, filepath.Join("testdata", "bundle"), Options{}).Record
	record := convertOneRecord(t, lineCardBundle(t, "environment_rating", "ligature_resistant"), Options{}).Record
	wantString(t, record, "product_family.environment_rating", "ligature_resistant")
	record["index"] = index.Build(record)
	wantString(t, record, "index.environment_rating", "ligature_resistant")
	want := completeness.AchievedLevel(base)
	if want != completeness.LevelCore || completeness.AchievedLevel(record) != want {
		t.Fatalf("grade = %s, base = %s, want core", completeness.AchievedLevel(record), want)
	}
	wantSchemaValid(t, record)
	t.Log("converted product_family.environment_rating=ligature_resistant index.environment_rating=ligature_resistant conformance_level=core")
}
