package sheet

import (
	"path/filepath"
	"reflect"
	"strings"
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

func TestPowerInputKindColumn(t *testing.T) {
	record := convertOneRecord(t, bundleWithColumns(t, map[string]string{"power_input_kind": "poe"}), Options{}).Record
	wantString(t, record, "electrical.power_input_kind", "poe")
	wantSchemaValid(t, record)
	wantString(t, record, "index.power_input_kind", "poe")
	t.Log("converted electrical.power_input_kind=poe index.power_input_kind=poe")
	got := domesticSchemaErrors(t, bundleWithColumns(t, map[string]string{"power_input_kind": "battery"}))
	for _, path := range []string{"/electrical/power_input_kind", "/index/power_input_kind"} {
		want := "at '" + path + "': value must be one of 'ac_mains', 'dc_low_voltage', 'poe', 'solar_pv'"
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	t.Log(got)
}

func TestFieldSelectableAxesColumn(t *testing.T) {
	record := convertOneRecord(t, bundleWithColumns(t, map[string]string{"field_selectable_axes": "output;cct"}), Options{}).Record
	value, ok := getPath(record, "applicability.field_selectable_axes")
	if !ok || !reflect.DeepEqual(value, []any{"output", "cct"}) {
		t.Fatalf("authored axes = %#v", value)
	}
	wantSchemaValid(t, record)
	value, ok = getPath(record, "index.field_selectable_axes")
	if !ok || !reflect.DeepEqual(value, []any{"cct", "output"}) {
		t.Fatalf("indexed axes = %#v", value)
	}
	t.Log("converted applicability.field_selectable_axes=[output cct] index.field_selectable_axes=[cct output]")
	got := domesticSchemaErrors(t, bundleWithColumns(t, map[string]string{"field_selectable_axes": "cct;cct"}))
	want := "at '/applicability/field_selectable_axes': items at 0 and 1 are equal"
	if !strings.Contains(got, want) {
		t.Fatalf("missing %q in %s", want, got)
	}
	t.Log(got)
}
