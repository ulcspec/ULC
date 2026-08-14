package sheet

import (
	"strings"
	"testing"
)

// wantDualUnit asserts both leaves of a dual-unit object at a dotted path,
// comparing numerically so an integer-preserved leaf and a float leaf are both
// accepted at the same value.
func wantDualUnit(t *testing.T, rec map[string]any, path, siLeaf string, si float64, companionLeaf string, companion float64) {
	t.Helper()
	v, ok := getPath(rec, path)
	if !ok {
		t.Errorf("%s is absent", path)
		return
	}
	obj, ok := v.(map[string]any)
	if !ok {
		t.Errorf("%s = %T, want a dual-unit object", path, v)
		return
	}
	if got := numeric(t, path+"."+siLeaf, obj[siLeaf]); got != si {
		t.Errorf("%s.%s = %v, want %v", path, siLeaf, got, si)
	}
	if got := numeric(t, path+"."+companionLeaf, obj[companionLeaf]); got != companion {
		t.Errorf("%s.%s = %v, want %v", path, companionLeaf, got, companion)
	}
}

func numeric(t *testing.T, label string, v any) float64 {
	t.Helper()
	switch n := v.(type) {
	case int64:
		return float64(n)
	case float64:
		return n
	default:
		t.Errorf("%s = %T, want a number", label, v)
		return 0
	}
}

// TestConvertImperialEntry authors the Imperial side of three dual-unit fields
// and asserts the authored leaf lands verbatim, the SI leaf is computed, and the
// provenance override columns work on the new headers exactly as on the SI ones.
func TestConvertImperialEntry(t *testing.T) {
	dir := bundleWithColumns(t, map[string]string{
		"overall_diameter_in":             "4.45",
		"overall_diameter_in__value_type": "nominal",
		"luminaire_mass_lb":               "25",
		"linear_mass_per_foot_lb_per_ft":  "0.75",
	})

	rec := convertOneRecord(t, dir, Options{}).Record

	wantDualUnit(t, rec, "product_family.physical_dimensions.overall_diameter", "mm", 113.03, "in", 4.45)
	wantDualUnit(t, rec, "product_family.physical_dimensions.luminaire_mass", "kg", 11.3398, "lb", 25)
	wantDualUnit(t, rec, "product_family.physical_dimensions.linear_mass_per_foot", "kg_per_m", 1.1161, "lb_per_ft", 0.75)

	// value_type comes from the override where given, from the column default
	// otherwise.
	wantString(t, rec, "product_family.physical_dimensions.overall_diameter.value_type", "nominal")
	wantString(t, rec, "product_family.physical_dimensions.luminaire_mass.value_type", "rated")

	// The column's provenance defaults apply to the Imperial header unchanged.
	wantString(t, rec, "product_family.physical_dimensions.luminaire_mass.provenance.source", "datasheet_pdf")
	wantString(t, rec, "product_family.physical_dimensions.luminaire_mass.provenance.method", "extracted")

	wantSchemaValid(t, rec)
}

// TestConvertRejectsBothUnitSides pins the both-sides-authored refusal: a row
// filling one field's SI and Imperial columns is a contradiction in the source
// data, and the error names both columns.
func TestConvertRejectsBothUnitSides(t *testing.T) {
	dir := bundleWithColumns(t, map[string]string{
		"overall_diameter_mm": "113",
		"overall_diameter_in": "4.45",
	})

	_, err := Convert(dir, Options{})
	if err == nil {
		t.Fatal("expected an error when both sides of a dual-unit field are authored")
	}
	msg := err.Error()
	if !strings.Contains(msg, "overall_diameter_in") || !strings.Contains(msg, "overall_diameter_mm") {
		t.Errorf("error must name both columns, got: %s", msg)
	}
}
