package sheet

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/completeness"
)

func spectrumBundle(t *testing.T, change func([][]string) [][]string) string {
	t.Helper()
	bundle := t.TempDir()
	copyBundle(t, filepath.Join("testdata", "bundle-b"), bundle)
	path := filepath.Join(bundle, "spectral_power_distribution.csv")
	rows := readCSVRows(t, path)
	writeCSVRows(t, path, change(rows))
	return bundle
}

func TestSpectrumAcceptsSamplesAndLinksProvenanceAcrossReaders(t *testing.T) {
	bundle := spectrumBundle(t, func(rows [][]string) [][]string { return rows })
	var first map[string]any
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			record := convertOne(t, input, PatternB, completeness.LevelStandard)
			spectrum, _ := getPath(record, "colorimetry.spectral_power_distribution")
			block := spectrum.(map[string]any)
			if asFloat(t, block["wavelength_start_nm"]) != 350 || asFloat(t, block["wavelength_step_nm"]) != 5 {
				t.Errorf("spectrum grid = %v to %v", block["wavelength_start_nm"], block["wavelength_step_nm"])
			}
			values := block["values"].([]any)
			if len(values) != 87 || asFloat(t, values[0]) != 0.01 || asFloat(t, values[86]) != 0.87 {
				t.Errorf("unexpected spectral values: %v", values)
			}
			if block["unit"] != "mW/nm" || block["source_kind"] != "laboratory_table" || block["measured_through_optics"] != true || block["value_type"] != "measured" {
				t.Errorf("unexpected spectrum metadata: %v", block)
			}
			prov := block["provenance"].(map[string]any)
			if prov["source"] != "test_report" || prov["method"] != "transcribed" || prov["attestation_ref"] != "lm79_lumos_skyline_sr_ho" {
				t.Errorf("unexpected spectrum provenance: %v", prov)
			}
			if first == nil {
				first = block
			} else if !reflect.DeepEqual(first, block) {
				t.Errorf("CSV and XLSX spectra differ: %v versus %v", first, block)
			}
		})
	}
}

func TestSpectrumDecimalGridEmitsRoundedStepAcrossReaders(t *testing.T) {
	bundle := spectrumBundle(t, func(rows [][]string) [][]string {
		rows = rows[:4]
		rows[1][1], rows[2][1], rows[3][1] = "380", "380.10000000000002", "380.2"
		return rows
	})
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			record := convertOne(t, input, PatternB, completeness.LevelStandard)
			step, _ := getPath(record, "colorimetry.spectral_power_distribution.wavelength_step_nm")
			if step != 0.1 {
				t.Errorf("step = %v, want 0.1", step)
			}
		})
	}
}

func TestSpectrumBlockMetadataAcrossReaders(t *testing.T) {
	tests := []struct {
		name       string
		edit       func([][]string) [][]string
		optics     any
		typeOf     string
		provSource string
	}{
		{"later row", func(r [][]string) [][]string {
			for _, col := range []int{3, 4, 5} {
				r[2][col], r[1][col] = r[1][col], ""
			}
			return r
		}, true, "measured", "test_report"},
		{"matching repeated cells", func(r [][]string) [][]string {
			for _, col := range []int{3, 4, 5} {
				r[2][col] = r[1][col]
			}
			return r
		}, true, "measured", "test_report"},
		{"rated package spectrum", func(r [][]string) [][]string {
			r[1][5], r[1][7], r[1][8] = "FALSE", "rated", "datasheet_pdf"
			return r
		}, false, "rated", "datasheet_pdf"},
		{"blank optics", func(r [][]string) [][]string { r[1][5] = ""; return r }, nil, "measured", "test_report"},
		{"late rated provenance", func(r [][]string) [][]string {
			r[2][7], r[2][8] = "rated", "datasheet_pdf"
			return r
		}, true, "rated", "datasheet_pdf"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := spectrumBundle(t, test.edit)
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					record := convertOne(t, input, PatternB, completeness.LevelStandard)
					value, _ := getPath(record, "colorimetry.spectral_power_distribution")
					block := value.(map[string]any)
					if block["unit"] != "mW/nm" || block["source_kind"] != "laboratory_table" || block["value_type"] != test.typeOf {
						t.Errorf("unexpected spectrum metadata: %v", block)
					}
					if source := block["provenance"].(map[string]any)["source"]; source != test.provSource {
						t.Errorf("provenance source = %v, want %s", source, test.provSource)
					}
					if optics, present := block["measured_through_optics"]; test.optics == nil {
						if present {
							t.Errorf("blank optics cell emitted measured_through_optics=%v", optics)
						}
					} else if !present || optics != test.optics {
						t.Errorf("measured_through_optics = %v, want %v", optics, test.optics)
					}
				})
			}
		})
	}
}

func TestSpectrumRefusalsAcrossReaders(t *testing.T) {
	tests := []struct {
		name string
		edit func([][]string) [][]string
		want string
	}{
		{"off grid", func(r [][]string) [][]string { r[3][1] = "361"; return r }, "off the uniform grid"},
		{"duplicate", func(r [][]string) [][]string { r[3][1] = "355"; return r }, "strictly ascending"},
		{"descending", func(r [][]string) [][]string { r[3][1] = "354"; return r }, "strictly ascending"},
		{"nonnumeric wavelength", func(r [][]string) [][]string { r[2][1] = "oops"; return r }, "wavelength_nm"},
		{"nonnumeric value", func(r [][]string) [][]string { r[2][2] = "oops"; return r }, "value"},
		{"single row", func(r [][]string) [][]string { return r[:2] }, "at least two"},
		{"blank unit", func(r [][]string) [][]string { r[1][3] = ""; return r }, "unit"},
		{"unknown unit", func(r [][]string) [][]string { r[1][3] = "lux"; return r }, "unit"},
		{"conflicting block unit", func(r [][]string) [][]string { r[2][3] = "W/nm"; return r }, "conflicting unit"},
		{"conflicting provenance", func(r [][]string) [][]string { r[1][8] = "test_report"; r[2][8] = "datasheet_pdf"; return r }, "conflicting value__prov_source"},
		{"wrong family attestation", func(r [][]string) [][]string { r[1][12] = "iec_60598_lumos_skyline"; return r }, "different evidence family"},
		{"measured package spectrum", func(r [][]string) [][]string { r[1][5] = "FALSE"; return r }, "measured_through_optics=false conflicts with effective value__value_type=measured; set value__value_type=rated"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := spectrumBundle(t, test.edit)
			for reader, input := range supplementaryInputs(t, bundle) {
				t.Run(reader, func(t *testing.T) {
					_, err := Convert(input, Options{})
					if err == nil {
						t.Fatal("invalid spectrum converted")
					}
					for _, want := range []string{"spectral_power_distribution", "row", "lumos-skyline-sr-ho-3000k", test.want} {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("error %q does not contain %q", err, want)
						}
					}
				})
			}
		})
	}
}
