package sheet

import (
	"path/filepath"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/completeness"
)

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
