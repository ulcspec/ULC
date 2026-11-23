package sheet

import (
	"reflect"
	"strings"
	"testing"
)

func TestOperatingAmbientTemperatureColumn(t *testing.T) {
	for _, header := range []string{"operating_ambient_temperature_c", "operating_ambient_temperature_f"} {
		t.Run(header, func(t *testing.T) {
			value := "25"
			if header == "operating_ambient_temperature_f" {
				value = "77"
			}
			record := convertOneRecord(t, bundleWithColumns(t, map[string]string{header: value}), Options{}).Record
			wantDualUnit(t, record, "operating_point.ambient_temperature", "c", 25, "f", 77)
			got, ok := getPath(record, "operating_point.ambient_temperature")
			if !ok {
				t.Fatal("ambient temperature missing")
			}
			payload := got.(map[string]any)
			if len(payload) != 4 || payload["value_type"] != "rated" || !reflect.DeepEqual(payload["provenance"], map[string]any{"source": "datasheet_pdf", "method": "extracted"}) {
				t.Fatalf("ambient payload = %#v, want two leaves and rated datasheet defaults", payload)
			}
			wantSchemaValid(t, record)
		})
	}
	t.Run("both sides refused", func(t *testing.T) {
		_, err := Convert(bundleWithColumns(t, map[string]string{"operating_ambient_temperature_c": "25", "operating_ambient_temperature_f": "77"}), Options{})
		want := "the SI companion column \"operating_ambient_temperature_c\" is also authored for this row"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("Convert error = %v, want %q", err, want)
		}
	})
	t.Run("blank omitted", func(t *testing.T) {
		record := convertOneRecord(t, bundleWithColumns(t, map[string]string{"operating_ambient_temperature_c": "", "operating_ambient_temperature_f": ""}), Options{}).Record
		if _, exists := record["operating_point"]; exists {
			t.Fatalf("blank ambient emitted operating_point: %v", record["operating_point"])
		}
	})
}
