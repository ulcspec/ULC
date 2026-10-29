package sheet

import (
	"path/filepath"
	"strings"
	"testing"
)

func originCountryBundle(t *testing.T, sheet, program, country string) string {
	t.Helper()
	bundle := domesticBundle(t, sheet, map[string]string{"program": program})
	path := filepath.Join(bundle, sheet+".csv")
	rows := readCSVRows(t, path)
	rows[0] = append(rows[0], "origin_country")
	for i := 1; i < len(rows); i++ {
		value := ""
		if i == len(rows)-1 {
			value = country
		}
		rows[i] = append(rows[i], value)
	}
	writeCSVRows(t, path, rows)
	return bundle
}

func TestOriginCountryColumnOnBothAttestationSheets(t *testing.T) {
	for _, test := range []struct{ sheet, program, country string }{
		{"attestations", "country_of_origin", "MX"},
		{"shared_attestations", "made_in_usa", "US"},
		{"attestations", "country_of_origin", ""},
		{"shared_attestations", "made_in_canada", ""},
	} {
		t.Run(test.sheet+"/"+test.program+"/"+test.country, func(t *testing.T) {
			record := convertOneRecord(t, originCountryBundle(t, test.sheet, test.program, test.country), Options{}).Record
			path := "attestations"
			if test.sheet == "shared_attestations" {
				path = "product_family.shared_attestations"
			}
			found := false
			for _, item := range arrayAt(t, record, path) {
				att := item.(map[string]any)
				if att["attestation_id"] != "baa-domestic-test" {
					continue
				}
				found = true
				country, present := att["origin_country"]
				if test.country == "" {
					if present {
						t.Errorf("blank country emitted %v", country)
					}
				} else if country != test.country {
					t.Errorf("country = %v, want %s", country, test.country)
				}
				t.Logf("converted %s: program=%s origin_country=%v present=%t", path, test.program, country, present)
			}
			if !found {
				t.Fatal("missing synthetic attestation")
			}
			wantSchemaValid(t, record)
		})
	}
}

func TestOriginCountryRefusals(t *testing.T) {
	for _, test := range []struct{ program, country, want string }{
		{"country_of_origin", "us", "'us' does not match pattern '^[A-Z]{2}$'"},
		{"made_in_canada", "US", "value must be 'CA'"},
	} {
		t.Run(test.program, func(t *testing.T) {
			got := domesticSchemaErrors(t, originCountryBundle(t, "attestations", test.program, test.country))
			if !strings.Contains(got, "/origin_country': "+test.want) {
				t.Fatalf("refusal missing %q: %s", test.want, got)
			}
			t.Log(got)
		})
	}
}
