package sheet

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/completeness"
)

func writeHueBinRows(t *testing.T, bundle string, edit func([][]string) [][]string) {
	t.Helper()
	path := filepath.Join(bundle, "tm_30_hue_bins.csv")
	header := strings.Split("record_id,bin,rf_h,conflict_notes,rf_h__value_type,rf_h__prov_source,rf_h__prov_method,rf_h__extension_method,rf_h__base_attestation_ref,rf_h__attestation_ref", ",")
	if _, err := os.Stat(path); err == nil {
		header = readCSVRows(t, path)[0]
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	rows := [][]string{header}
	id := bundleRecordID(t, bundle)
	for b := 1; b <= 16; b++ {
		row := make([]string, len(header))
		row[0], row[1], row[2] = id, strconv.Itoa(b), strconv.Itoa(70+b)+".5"
		rows = append(rows, row)
	}
	if edit != nil {
		rows = edit(rows)
	}
	writeCSVRows(t, path, rows)
}

func hueBinBundle(t *testing.T, records map[string]string, edit func([][]string) [][]string) string {
	t.Helper()
	if records == nil {
		records = map[string]string{"tm_30_rf": "87"}
	}
	bundle := colourBundle(t, records)
	writeHueBinRows(t, bundle, edit)
	return bundle
}

func TestTM30HueBinsConvertSixteenEntriesAcrossReaders(t *testing.T) {
	bundle := hueBinBundle(t, nil, func(rows [][]string) [][]string {
		for i, j := 1, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
		return rows
	})
	var first []any
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			record := convertOneRecord(t, input, Options{}).Record
			entries := arrayAt(t, record, "colorimetry.tm_30.rf_h_per_bin")
			if len(entries) != 16 {
				t.Fatalf("entries = %d, want 16", len(entries))
			}
			for i, raw := range entries {
				entry := raw.(map[string]any)
				if entry["bin"] != int64(i+1) {
					t.Errorf("bin = %v, want %d", entry["bin"], i+1)
				}
				value := entry["rf_h"].(map[string]any)
				if asFloat(t, value["value"]) != float64(71+i)+0.5 || value["value_type"] != "measured" {
					t.Errorf("unexpected rf_h: %v", value)
				}
				want := map[string]any{"source": "test_report", "method": "transcribed", "attestation_ref": "lm79_lumos_skyline_sr_ho"}
				if !reflect.DeepEqual(value["provenance"], want) {
					t.Errorf("provenance = %v, want %v", value["provenance"], want)
				}
				if _, ok := value["unit"]; ok {
					t.Errorf("rf_h has unit: %v", value)
				}
			}
			if first == nil {
				first = entries
			} else if !reflect.DeepEqual(first, entries) {
				t.Errorf("CSV and XLSX hue bins differ: %v versus %v", first, entries)
			}
			for _, item := range completeness.Compute(record).TierRoadmap {
				if item.Path == "/colorimetry/tm_30/rf_h_per_bin" {
					t.Error("sixteen hue bins leave the Full item unmet")
				}
			}
		})
	}
	control := hueBinBundle(t, nil, func(rows [][]string) [][]string { return rows[:1] })
	for reader, input := range supplementaryInputs(t, control) {
		t.Run(reader+"/header only", func(t *testing.T) {
			record := convertOneRecord(t, input, Options{}).Record
			for _, item := range completeness.Compute(record).TierRoadmap {
				if item.Path == "/colorimetry/tm_30/rf_h_per_bin" {
					return
				}
			}
			t.Error("header-only sheet lacks the Full gap control")
		})
	}
}

func TestTM30HueBinsRefusalsAcrossReaders(t *testing.T) {
	tests := []struct {
		name    string
		records map[string]string
		edit    func([][]string) [][]string
		want    string
	}{
		{"bin 17", nil, func(r [][]string) [][]string { r[1][1] = "17"; return r }, "bin 17 is outside 1 to 16"},
		{"bin 0", nil, func(r [][]string) [][]string { r[1][1] = "0"; return r }, "bin 0 is outside 1 to 16"},
		{"repeated bin", nil, func(r [][]string) [][]string { r[16][1] = "1"; return r }, "bin 1 repeats row 1"},
		{"fractional bin", nil, func(r [][]string) [][]string { r[1][1] = "2.5"; return r }, "expected an integer"},
		{"text bin", nil, func(r [][]string) [][]string { r[1][1] = "x"; return r }, "invalid integer"},
		{"blank bin", nil, func(r [][]string) [][]string { r[1][1] = ""; return r }, "missing bin"},
		{"text rf_h", nil, func(r [][]string) [][]string { r[1][2] = "oops"; return r }, "invalid rf_h \"oops\""},
		{"NaN rf_h", nil, func(r [][]string) [][]string { r[1][2] = "NaN"; return r }, "non-finite number"},
		{"blank rf_h", nil, func(r [][]string) [][]string { r[1][2] = ""; return r }, "missing rf_h"},
		{"fifteen rows", nil, func(r [][]string) [][]string { return r[:16] }, "missing bins 16"},
		{"wrong family", nil, func(r [][]string) [][]string { r[1][9] = "iec_60598_lumos_skyline"; return r }, "different evidence family"},
		{"unknown attestation", nil, func(r [][]string) [][]string { r[1][9] = "missing"; return r }, "no attestation with that id exists"},
		{"missing headline", map[string]string{}, nil, "hue bins require tm_30_rf on records"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := hueBinBundle(t, test.records, test.edit)
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					_, err := Convert(input, Options{})
					if err == nil {
						t.Fatal("invalid hue bins converted")
					}
					location := "row"
					if test.name == "fifteen rows" {
						location = "for"
					}
					for _, want := range []string{"tm_30_hue_bins", location, bundleRecordID(t, bundle), test.want} {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("error %q lacks %q", err, want)
						}
					}
				})
			}
		})
	}
}

func TestTM30HueBinsAnchorAcrossReaders(t *testing.T) {
	bundle := isolatedColourBundle(t, map[string]string{"tm_30_rf": "87", "tm_30_rf__value_type": "rated"})
	setColourAttestations(t, bundle)
	writeHueBinRows(t, bundle, nil)
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader+"/measured", func(t *testing.T) {
			_, err := Convert(input, Options{})
			if err == nil {
				t.Fatal("measured hue bins without LM-79 converted")
			}
			for _, want := range []string{"tm_30_hue_bins row 1", bundleRecordID(t, bundle), "column \"rf_h\"", "no photometric lm_79"} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q lacks %q", err, want)
				}
			}
		})
	}
	writeHueBinRows(t, bundle, func(rows [][]string) [][]string {
		for _, row := range rows[1:] {
			row[4], row[5] = "rated", "manufacturer_data_export"
		}
		return rows
	})
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader+"/rated", func(t *testing.T) {
			record := convertOneRecord(t, input, Options{}).Record
			for _, entry := range arrayAt(t, record, "colorimetry.tm_30.rf_h_per_bin") {
				value := entry.(map[string]any)["rf_h"].(map[string]any)
				if value["value_type"] != "rated" {
					t.Errorf("value_type = %v", value["value_type"])
				}
				prov := value["provenance"].(map[string]any)
				if prov["source"] != "manufacturer_data_export" {
					t.Errorf("source = %v", prov["source"])
				}
				if _, ok := prov["attestation_ref"]; ok {
					t.Errorf("rated hue bin has attestation_ref: %v", prov)
				}
			}
		})
	}
}

func TestTM30HueBinsUnsupportedCompanionRefused(t *testing.T) {
	bundle := hueBinBundle(t, nil, func(rows [][]string) [][]string {
		rows[0] = append(rows[0], "rf_h__bogus")
		for i := range rows[1:] {
			rows[i+1] = append(rows[i+1], "")
		}
		return rows
	})
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			_, err := Convert(input, Options{})
			want := "sheet \"tm_30_hue_bins\": companion column \"rf_h__bogus\" has unsupported suffix"
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("error = %v, want %q", err, want)
			}
		})
	}
}
