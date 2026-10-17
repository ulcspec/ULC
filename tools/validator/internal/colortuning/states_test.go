package colortuning

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func fixture() map[string]any {
	return map[string]any{
		"configuration": map[string]any{"tested_axes": map[string]any{"color_tunability": "tunable_white"}, "tested_conditions": map[string]any{"nominal_cct_at_test": "3000"}},
		"colorimetry":   map[string]any{"nominal_cct_k": "3500", "measured_cct_k": map[string]any{"value": 2985.0}},
		"color_tuning":  map[string]any{"baseline_role": "setting", "states": []any{testState("warm", "range_low_end", "2700"), testState("cool", "range_high_end", "6500")}},
	}
}
func testState(id, role, cct string) map[string]any {
	return map[string]any{"state_id": id, "role": role, "colorimetry": map[string]any{"nominal_cct_k": cct}}
}
func testRows(r map[string]any) []any { return object(r, "color_tuning")["states"].([]any) }

func TestStatesResolveBaselineFirstAndKeepAuthoritativeBlocks(t *testing.T) {
	r := fixture()
	spectrum := map[string]any{"values": []any{0.2, 0.4}}
	object(r, "colorimetry")["spectral_power_distribution"] = spectrum
	r["alpha_opic_metrics"] = map[string]any{"melanopic_der": map[string]any{"value": 0.5}}
	r["photometry"] = map[string]any{"total_luminous_flux_lm": map[string]any{"value": 1200}, "luminaire_efficacy_lm_per_w": map[string]any{"value": 90}}
	warm := testRows(r)[0].(map[string]any)
	warm["label"] = "Warm"
	warm["dim_level_percent"] = 10.0
	warm["alpha_opic_metrics"] = map[string]any{"melanopic_der": map[string]any{"value": 0.3}}
	warm["lumens"] = map[string]any{"value": 100}
	warm["efficacy_lm_per_w"] = map[string]any{"value": 80}
	states := States(r)
	if len(states) != 3 {
		t.Fatalf("states = %v", states)
	}
	if !states[0].Baseline || states[1].Baseline || states[0].CCT != 3000 || states[0].DimLevelPercent != 100 {
		t.Fatalf("baseline = %+v", states[0])
	}
	if states[1].ID != "warm" || states[1].Role != "range_low_end" || states[1].Label != "Warm" || states[1].DimLevelPercent != 10 || states[2].ID != "cool" {
		t.Fatalf("published order and details = %+v", states)
	}
	if !reflect.DeepEqual(states[1].AlphaOpicMetrics, warm["alpha_opic_metrics"]) || !reflect.DeepEqual(states[1].Lumens, warm["lumens"]) || !reflect.DeepEqual(states[1].Efficacy, warm["efficacy_lm_per_w"]) {
		t.Fatal("state values are not its own blocks")
	}
	states[0].Colorimetry["pointer_check"] = true
	if object(r, "colorimetry")["pointer_check"] != true {
		t.Fatal("resolver copied baseline colorimetry")
	}
	states[1].Colorimetry["pointer_check"] = true
	if object(warm, "colorimetry")["pointer_check"] != true {
		t.Fatal("resolver copied listed colorimetry")
	}
	if !reflect.DeepEqual(states[0].Colorimetry["spectral_power_distribution"], spectrum) {
		t.Fatal("baseline spectrum lost")
	}
	if states[1].Colorimetry["spectral_power_distribution"] != nil {
		t.Fatal("baseline spectrum leaked into listed state")
	}
	if states[0].Lumens["value"] != 1200 || states[0].Efficacy["value"] != 90 {
		t.Fatal("baseline photometry lost")
	}
}

func TestStatesUseOneCCTRule(t *testing.T) {
	r := fixture()
	for _, tt := range []struct {
		name   string
		change func()
		want   float64
	}{
		{"tested bin wins", func() {}, 3000},
		{"colorimetry bin fallback", func() { delete(object(object(r, "configuration"), "tested_conditions"), "nominal_cct_at_test") }, 3500},
		{"measured fallback", func() { delete(object(r, "colorimetry"), "nominal_cct_k") }, 2985},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tt.change()
			if got := States(r)[0].CCT; got != tt.want {
				t.Fatalf("CCT = %v, want %v", got, tt.want)
			}
		})
	}
	low := testRows(r)[0].(map[string]any)
	low["colorimetry"] = map[string]any{"measured_cct_k": map[string]any{"value": json.Number("1850"), "value_type": "rated"}}
	a, b, ok := Ends(r)
	if !ok || a.CCT != 1850 || b.CCT != 6500 {
		t.Fatalf("off-bin ends = %+v %+v %v", a, b, ok)
	}
	tuning := object(r, "color_tuning")
	tuning["baseline_role"] = "range_high_end"
	tuning["states"] = []any{low}
	a, b, ok = Ends(r)
	if !ok || !b.Baseline || a.CCT != 1850 || b.CCT != 2985 {
		t.Fatalf("baseline high end = %+v %+v %v", a, b, ok)
	}
}

func TestProblemsV1ToV6AndAcceptedCases(t *testing.T) {
	tests := []struct {
		name, code string
		change     func(map[string]any)
	}{
		{"valid white range", "", func(map[string]any) {}},
		{"duplicate state id", "V1", func(r map[string]any) { testRows(r)[1].(map[string]any)["state_id"] = "warm" }},
		{"duplicate channel id", "V1", func(r map[string]any) {
			object(r, "color_tuning")["channels"] = []any{map[string]any{"channel_id": "red"}, map[string]any{"channel_id": "red"}}
		}},
		{"lone end", "V2", func(r map[string]any) { object(r, "color_tuning")["states"] = testRows(r)[:1] }},
		{"reversed range", "V3", func(r map[string]any) {
			object(testRows(r)[0].(map[string]any), "colorimetry")["nominal_cct_k"] = "6500"
		}},
		{"setting outside range", "V4", func(r map[string]any) {
			object(r, "color_tuning")["states"] = append(testRows(r), testState("outside", "setting", "7000"))
		}},
		{"baseline outside range", "V4", func(r map[string]any) {
			object(object(r, "configuration"), "tested_conditions")["nominal_cct_at_test"] = "7000"
		}},
		{"repeated baseline", "V5", func(r map[string]any) {
			object(r, "color_tuning")["states"] = append(testRows(r), testState("copy", "setting", "3000"))
		}},
		{"case folded duplicate label", "V5", func(r map[string]any) {
			object(r, "color_tuning")["baseline_label"] = "TESTED"
			s := testState("copy", "setting", "3000")
			s["label"] = "tested"
			object(r, "color_tuning")["states"] = append(testRows(r), s)
		}},
		{"unicode case fold", "V5", func(r map[string]any) {
			object(r, "color_tuning")["baseline_label"] = "Σ"
			s := testState("copy", "setting", "3000")
			s["label"] = "ς"
			object(r, "color_tuning")["states"] = append(testRows(r), s)
		}},
		{"one unnamed same CCT and dim", "V6", func(r map[string]any) {
			object(r, "color_tuning")["baseline_label"] = "Tested"
			object(r, "color_tuning")["states"] = append(testRows(r), testState("preset", "setting", "3000"))
		}},
		{"different labeled same CCT settings", "", func(r map[string]any) {
			object(r, "color_tuning")["baseline_label"] = "Tested"
			object(object(r, "configuration"), "tested_axes")["color_tunability"] = "multichannel"
			for _, label := range []string{"Circadian", "Standard"} {
				s := testState(strings.ToLower(label), "setting", "3000")
				s["label"] = label
				object(r, "color_tuning")["states"] = append(testRows(r), s)
			}
		}},
		{"multichannel preset outside white range", "", func(r map[string]any) {
			object(object(r, "configuration"), "tested_axes")["color_tunability"] = "multichannel"
			object(r, "color_tuning")["states"] = append(testRows(r), testState("preset", "setting", "7000"))
		}},
		{"different dim levels at same CCT", "", func(r map[string]any) {
			for i, pct := range []float64{10, 50} {
				s := testState([]string{"dim-ten", "dim-fifty"}[i], "dim_level", "3000")
				s["dim_level_percent"] = pct
				object(r, "color_tuning")["states"] = append(testRows(r), s)
			}
		}},
		{"end repeated as dim level", "V5", func(r map[string]any) {
			s := testState("warm-copy", "dim_level", "2700")
			s["dim_level_percent"] = 100
			object(r, "color_tuning")["states"] = append(testRows(r), s)
		}},
		{"dim to warm baseline end", "", func(r map[string]any) {
			object(object(r, "configuration"), "tested_axes")["color_tunability"] = "dim_to_warm"
			b := object(r, "color_tuning")
			b["baseline_role"] = "range_high_end"
			b["baseline_dim_level_percent"] = 100
			b["states"] = testRows(r)[:1]
			testRows(r)[0].(map[string]any)["dim_level_percent"] = 10
		}},
		{"settings only", "", func(r map[string]any) {
			object(r, "color_tuning")["states"] = []any{testState("preset", "setting", "4000")}
		}},
		{"RGBW channels only unnamed baseline", "", func(r map[string]any) {
			object(object(r, "configuration"), "tested_axes")["color_tunability"] = "rgbw"
			b := object(r, "color_tuning")
			delete(b, "states")
			b["channels"] = []any{map[string]any{"channel_id": "white", "kind": "white"}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := fixture()
			tt.change(r)
			problems := Problems(r)
			if tt.code == "" {
				if len(problems) > 0 {
					t.Fatalf("unexpected problems: %+v", problems)
				}
				return
			}
			for _, p := range problems {
				if strings.HasPrefix(p.Message, tt.code+":") {
					if p.Path == "" {
						t.Fatal("problem has no pointer")
					}
					return
				}
			}
			t.Fatalf("missing %s: %+v", tt.code, problems)
		})
	}
}

func TestMalformedRecordsAreTotal(t *testing.T) {
	for _, r := range []map[string]any{nil, {}, {"color_tuning": map[string]any{"states": []any{nil, 42, map[string]any{}}, "channels": []any{nil, map[string]any{}}}}, {"configuration": false, "colorimetry": []any{}, "color_tuning": true}} {
		States(r)
		Ends(r)
		Problems(r)
	}
}
