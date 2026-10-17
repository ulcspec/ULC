package sheet

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/colortuning"
	"github.com/ulcspec/ULC/tools/validator/internal/findings"
	"github.com/ulcspec/ULC/tools/validator/internal/index"
	"github.com/ulcspec/ULC/tools/validator/internal/validate"
)

func writeColorRows(t *testing.T, bundle, sheet string, rows ...Row) {
	t.Helper()
	header := readCSVRows(t, filepath.Join(filepath.Dir(schemaDir(t)), "templates", "workbook", sheet+".csv"))[0]
	out := [][]string{header}
	id := bundleRecordID(t, bundle)
	for _, row := range rows {
		cells := make([]string, len(header))
		for i, h := range header {
			cells[i] = row[h]
			if h == "record_id" {
				cells[i] = id
			}
		}
		out = append(out, cells)
	}
	writeCSVRows(t, filepath.Join(bundle, sheet+".csv"), out)
}
func colorBundle(t *testing.T) string {
	t.Helper()
	bundle := colourBundle(t, map[string]string{"color_tunability": "tunable_white", "nominal_cct_at_test": "3000", "nominal_cct_k": "3000", "color_tuning_baseline_role": "setting"})
	writeColorRows(t, bundle, "color_states", Row{"state_id": "warm", "role": "range_low_end", "nominal_cct_k": "2700", "cri_ra": "90", "melanopic_der": "0.3", "lumens": "1200"}, Row{"state_id": "cool", "role": "range_high_end", "nominal_cct_k": "6500", "cri_ra": "90", "lumens": "1350"})
	return bundle
}
func editColorCell(t *testing.T, bundle, sheet string, row int, field, value string) {
	t.Helper()
	path := filepath.Join(bundle, sheet+".csv")
	rows := readCSVRows(t, path)
	for i, h := range rows[0] {
		if h == field {
			rows[row][i] = value
			writeCSVRows(t, path, rows)
			return
		}
	}
	t.Fatalf("missing header %s on %s", field, sheet)
}

func TestColorStatesConvertAndGroupSpectraAcrossReaders(t *testing.T) {
	bundle := colorBundle(t)
	setClaimsCell(t, bundle, "records", "color_tunability", "multichannel")
	writeColorRows(t, bundle, "emission_channels", Row{"channel_id": "red", "kind": "color", "peak_wavelength_nm": "630", "lumens": "100"}, Row{"channel_id": "white", "kind": "white", "nominal_cct_k": "3000"})
	for field, value := range map[string]string{"measured_cct_k": "2711", "duv": "0.001", "cri_r9": "70", "tm_30_rf": "90", "tm_30_rg": "99", "efficacy_lm_per_w": "80", "cri_ra__value_type": "nominal", "cri_ra__prov_source": "manufacturer_direct", "cri_ra__prov_method": "transcribed"} {
		editColorCell(t, bundle, "color_states", 1, field, value)
	}
	writeColorRows(t, bundle, "spectral_power_distribution",
		Row{"wavelength_nm": "380", "value": "1", "unit": "relative", "source_kind": "laboratory_table", "value__value_type": "rated", "value__prov_source": "datasheet_pdf"},
		Row{"state_id": "warm", "wavelength_nm": "380", "value": "2", "unit": "relative", "source_kind": "laboratory_table", "value__value_type": "rated", "value__prov_source": "datasheet_pdf"},
		Row{"channel_id": "red", "wavelength_nm": "380", "value": "3", "unit": "relative", "source_kind": "laboratory_table", "value__value_type": "rated", "value__prov_source": "datasheet_pdf"},
		Row{"wavelength_nm": "385", "value": "4"}, Row{"state_id": "warm", "wavelength_nm": "385", "value": "5"}, Row{"channel_id": "red", "wavelength_nm": "385", "value": "6"})
	var first map[string]any
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			r := convertOneRecord(t, input, Options{}).Record
			low, high, ok := colortuning.Ends(r)
			if !ok || low.CCT != 2700 || high.CCT != 6500 {
				t.Fatal("nominal CCT must win over measured CCT")
			}
			assertProvenanceDefaults(t, "state measured CCT", low.Colorimetry["measured_cct_k"], "K", "rated", "datasheet_pdf", "transcribed", "")
			assertProvenanceDefaults(t, "state DER", low.AlphaOpicMetrics["melanopic_der"], "ratio", "rated", "datasheet_pdf", "transcribed", "")
			cri := low.Colorimetry["cri_ra"].(map[string]any)
			if _, present := cri["unit"]; present {
				t.Fatal("dimensionless CRI must omit unit")
			}
			if cri["value_type"] != "nominal" || !reflect.DeepEqual(cri["provenance"], map[string]any{"source": "manufacturer_direct", "method": "transcribed"}) {
				t.Fatalf("CRI override = %v", cri)
			}
			baseline, _ := getPath(r, "colorimetry.spectral_power_distribution.values")
			if baseline.([]any)[0] != int64(1) {
				t.Fatal("baseline spectrum changed target")
			}
			if low.Colorimetry["spectral_power_distribution"].(map[string]any)["values"].([]any)[0] != int64(2) {
				t.Fatal("state spectrum changed target")
			}
			channels := arrayAt(t, r, "color_tuning.channels")
			red := channels[0].(map[string]any)
			if red["colorimetry"].(map[string]any)["spectral_power_distribution"].(map[string]any)["values"].([]any)[0] != int64(3) {
				t.Fatal("channel spectrum changed target")
			}
			assertProvenanceDefaults(t, "channel peak", red["peak_wavelength_nm"], "nm", "rated", "datasheet_pdf", "transcribed", "")
			r["index"] = index.Build(r)
			if first == nil {
				first = r
			} else if !reflect.DeepEqual(first, r) {
				t.Fatal("CSV and XLSX records differ")
			}
			v, err := validate.NewValidator(schemaDir(t))
			if err != nil {
				t.Fatal(err)
			}
			report := findings.NewReport()
			v.Validate(r, report)
			if report.HasErrors() {
				t.Fatalf("converted record fails schema: %+v", report.Findings)
			}
		})
	}
}

func TestColorStatesRefusalsC1ToC9AcrossReaders(t *testing.T) {
	tests := []struct {
		name, want string
		edit       func(string)
	}{
		{"C1 static white", "non-static", func(b string) { setClaimsCell(t, b, "records", "color_tunability", "static_white") }},
		{"C1 no capability", "non-static", func(b string) { setClaimsCell(t, b, "records", "color_tunability", "") }},
		{"C2 rows without role", "require color_tuning_baseline_role", func(b string) { setClaimsCell(t, b, "records", "color_tuning_baseline_role", "") }},
		{"C2 role without rows", "requires color_states", func(b string) { writeColorRows(t, b, "color_states") }},
		{"C3 blank state id", "nonempty lowercase slug", func(b string) { editColorCell(t, b, "color_states", 1, "state_id", "") }},
		{"C3 nonslug state id", "nonempty lowercase slug", func(b string) { editColorCell(t, b, "color_states", 1, "state_id", "Warm State") }},
		{"C3 duplicate state id", "repeated state_id", func(b string) { editColorCell(t, b, "color_states", 2, "state_id", "warm") }},
		{"C3 duplicate channel id", "repeated channel_id", func(b string) {
			writeColorRows(t, b, "emission_channels", Row{"channel_id": "red", "kind": "color"}, Row{"channel_id": "red", "kind": "color"})
		}},
		{"C4 state role", "outside ColorStateRole", func(b string) { editColorCell(t, b, "color_states", 1, "role", "unknown") }},
		{"C4 baseline role", "outside ColorStateRole", func(b string) { setClaimsCell(t, b, "records", "color_tuning_baseline_role", "unknown") }},
		{"C4 channel kind", "outside EmissionChannelKind", func(b string) { writeColorRows(t, b, "emission_channels", Row{"channel_id": "red", "kind": "unknown"}) }},
		{"C5 missing CCT", "state requires", func(b string) { editColorCell(t, b, "color_states", 1, "nominal_cct_k", "") }},
		{"C5 nonbin CCT", "not a NominalCCT bin", func(b string) { editColorCell(t, b, "color_states", 1, "nominal_cct_k", "1850") }},
		{"C6 dim state without percent", "requires dim_level_percent", func(b string) { editColorCell(t, b, "color_states", 1, "role", "dim_level") }},
		{"C6 setting with percent", "setting forbids", func(b string) { setClaimsCell(t, b, "records", "color_tuning_baseline_dim_level_percent", "50") }},
		{"C6 zero percent", "in (0, 100]", func(b string) { editColorCell(t, b, "color_states", 1, "dim_level_percent", "0") }},
		{"C6 high percent", "in (0, 100]", func(b string) { editColorCell(t, b, "color_states", 1, "dim_level_percent", "101") }},
		{"C7 both spectrum ids", "never both", func(b string) {
			writeColorRows(t, b, "spectral_power_distribution", Row{"state_id": "warm", "channel_id": "red"})
		}},
		{"C7 unknown state id", "unknown state_id", func(b string) { writeColorRows(t, b, "spectral_power_distribution", Row{"state_id": "missing"}) }},
		{"C7 unknown channel id", "unknown channel_id", func(b string) { writeColorRows(t, b, "spectral_power_distribution", Row{"channel_id": "missing"}) }},
		{"C8 repeated baseline end", "repeats the baseline role", func(b string) { setClaimsCell(t, b, "records", "color_tuning_baseline_role", "range_low_end") }},
		{"C9 lone end", "V2:", func(b string) { editColorCell(t, b, "color_states", 2, "role", "setting") }},
		{"C9 reversed range", "V3:", func(b string) { editColorCell(t, b, "color_states", 1, "nominal_cct_k", "6500") }},
		{"C9 out of range", "V4:", func(b string) { setClaimsCell(t, b, "records", "nominal_cct_at_test", "1800") }},
		{"C9 repeated identity", "V5:", func(b string) { editColorCell(t, b, "color_states", 1, "nominal_cct_k", "3000") }},
		{"C9 unnamed shared CCT", "V6:", func(b string) {
			setClaimsCell(t, b, "records", "color_tuning_baseline_label", "Tested")
			editColorCell(t, b, "color_states", 1, "nominal_cct_k", "3000")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := colorBundle(t)
			tt.edit(b)
			for reader, input := range supplementaryInputs(t, b) {
				t.Run(reader, func(t *testing.T) {
					_, err := Convert(input, Options{})
					if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "row ") || !strings.Contains(err.Error(), bundleRecordID(t, b)) {
						t.Fatalf("error = %v, want contextual %q refusal", err, tt.want)
					}
				})
			}
		})
	}
}

func TestColorStatesDimToWarmAndChannelsOnly(t *testing.T) {
	b := colorBundle(t)
	setClaimsCell(t, b, "records", "color_tunability", "dim_to_warm")
	setClaimsCell(t, b, "records", "color_tuning_baseline_role", "range_high_end")
	setClaimsCell(t, b, "records", "color_tuning_baseline_dim_level_percent", "100")
	writeColorRows(t, b, "color_states", Row{"state_id": "warm", "role": "range_low_end", "measured_cct_k": "1850", "dim_level_percent": "10"})
	for reader, input := range supplementaryInputs(t, b) {
		t.Run(reader, func(t *testing.T) {
			r := convertOneRecord(t, input, Options{}).Record
			low, high, ok := colortuning.Ends(r)
			if !ok || low.CCT != 1850 || high.CCT != 3000 || low.DimLevelPercent != 10 || !high.Baseline {
				t.Fatalf("dim-to-warm ends = %+v %+v %v", low, high, ok)
			}
		})
	}
	setClaimsCell(t, b, "records", "color_tunability", "rgbw")
	setClaimsCell(t, b, "records", "color_tuning_baseline_role", "setting")
	setClaimsCell(t, b, "records", "color_tuning_baseline_dim_level_percent", "")
	writeColorRows(t, b, "color_states")
	writeColorRows(t, b, "emission_channels", Row{"channel_id": "white", "kind": "white", "nominal_cct_k": "3000"})
	for reader, input := range supplementaryInputs(t, b) {
		t.Run(reader+" channels only", func(t *testing.T) {
			r := convertOneRecord(t, input, Options{}).Record
			if len(colortuning.States(r)) != 1 {
				t.Fatal("channels must not become luminaire states")
			}
		})
	}
}
