package validate

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	embedded "github.com/ulcspec/ULC/schema"
	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// Pin the schema keywords that make the referenced baseline unambiguous.
func TestColorTuningSchemaShape(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(embedded.ULCSchemaJSON, &doc); err != nil {
		t.Fatal(err)
	}
	defs := doc["$defs"].(map[string]any)
	tuning := defs["ColorTuning"].(map[string]any)
	for name, want := range map[string][]any{
		"ColorTuning":     {"baseline_role"},
		"ColorState":      {"state_id", "role", "colorimetry"},
		"EmissionChannel": {"channel_id", "kind"},
	} {
		if got := defs[name].(map[string]any)["required"]; !reflect.DeepEqual(got, want) {
			t.Errorf("%s required = %v, want %v", name, got, want)
		}
	}
	props := tuning["properties"].(map[string]any)
	for _, key := range []string{"states", "channels"} {
		if props[key].(map[string]any)["minItems"] != float64(1) {
			t.Errorf("%s must require a nonempty array", key)
		}
	}
	limits := props["states"].(map[string]any)["allOf"].([]any)
	if len(limits) != 2 {
		t.Fatalf("end-role constraints = %d, want 2", len(limits))
	}
	for i, role := range []string{"range_low_end", "range_high_end"} {
		limit := limits[i].(map[string]any)
		if limit["minContains"] != float64(0) || limit["maxContains"] != float64(1) {
			t.Errorf("%s must allow zero or one listed end", role)
		}
		contains := limit["contains"].(map[string]any)
		if got := contains["properties"].(map[string]any)["role"].(map[string]any)["const"]; got != role {
			t.Errorf("contains role = %v, want %s", got, role)
		}
	}
	then := doc["then"].(map[string]any)["properties"].(map[string]any)
	if got, exists := then["color_tuning"]; !exists || got != false {
		t.Fatalf("static-white conditional must use false schema at /color_tuning, got %v", got)
	}
	color := defs["ColorState"].(map[string]any)["properties"].(map[string]any)["colorimetry"].(map[string]any)
	if len(color["allOf"].([]any)) != 2 {
		t.Fatal("state colorimetry must compose Colorimetry and the CCT requirement")
	}
}

func TestColorTuningSchema(t *testing.T) {
	root := repoRoot(t)
	v, err := NewValidator(filepath.Join(root, "schema"))
	if err != nil {
		t.Fatal(err)
	}
	state := func(role string) map[string]any {
		return map[string]any{"state_id": "published-state", "role": role, "colorimetry": map[string]any{"nominal_cct_k": "2700"}}
	}
	tests := []struct {
		name   string
		change func(map[string]any, map[string]any)
		valid  bool
	}{
		{"setting and one published end", func(_ map[string]any, _ map[string]any) {}, true},
		{"settings only needs no end", func(_ map[string]any, b map[string]any) { b["states"] = []any{state("setting")} }, true},
		{"channels only", func(_ map[string]any, b map[string]any) {
			delete(b, "states")
			b["channels"] = []any{map[string]any{"channel_id": "red", "kind": "color"}}
		}, true},
		{"static white refuses block", func(r map[string]any, _ map[string]any) {
			r["configuration"].(map[string]any)["tested_axes"].(map[string]any)["color_tunability"] = "static_white"
		}, false},
		{"absent capability refuses block", func(r map[string]any, _ map[string]any) {
			delete(r["configuration"].(map[string]any)["tested_axes"].(map[string]any), "color_tunability")
		}, false},
		{"absent axes refuses block", func(r map[string]any, _ map[string]any) { delete(r["configuration"].(map[string]any), "tested_axes") }, false},
		{"baseline role required", func(_ map[string]any, b map[string]any) { delete(b, "baseline_role") }, false},
		{"rows required", func(_ map[string]any, b map[string]any) { delete(b, "states") }, false},
		{"empty states refused", func(_ map[string]any, b map[string]any) { b["states"] = []any{} }, false},
		{"empty channels refused", func(_ map[string]any, b map[string]any) { b["channels"] = []any{} }, false},
		{"baseline end cannot repeat", func(_ map[string]any, b map[string]any) { b["baseline_role"] = "range_low_end" }, false},
		{"duplicate low end", func(_ map[string]any, b map[string]any) {
			b["states"] = []any{state("range_low_end"), state("range_low_end")}
		}, false},
		{"duplicate high end", func(_ map[string]any, b map[string]any) {
			b["states"] = []any{state("range_high_end"), state("range_high_end")}
		}, false},
		{"dim baseline needs percent", func(_ map[string]any, b map[string]any) { b["baseline_role"] = "dim_level" }, false},
		{"dim baseline with percent", func(_ map[string]any, b map[string]any) {
			b["baseline_role"] = "dim_level"
			b["baseline_dim_level_percent"] = 50
		}, true},
		{"setting baseline forbids percent", func(_ map[string]any, b map[string]any) { b["baseline_dim_level_percent"] = 100 }, false},
		{"end baseline allows percent", func(_ map[string]any, b map[string]any) {
			b["baseline_role"] = "range_high_end"
			b["baseline_dim_level_percent"] = 100
		}, true},
		{"dim state needs percent", func(_ map[string]any, b map[string]any) { b["states"] = []any{state("dim_level")} }, false},
		{"dim state with percent", func(_ map[string]any, b map[string]any) {
			s := state("dim_level")
			s["dim_level_percent"] = 10
			b["states"] = []any{s}
		}, true},
		{"setting state forbids percent", func(_ map[string]any, b map[string]any) {
			s := state("setting")
			s["dim_level_percent"] = 10
			b["states"] = []any{s}
		}, false},
		{"end state allows percent", func(_ map[string]any, b map[string]any) {
			s := state("range_low_end")
			s["dim_level_percent"] = 1
			b["states"] = []any{s}
		}, true},
		{"zero percent refused", func(_ map[string]any, b map[string]any) {
			s := state("dim_level")
			s["dim_level_percent"] = 0
			b["states"] = []any{s}
		}, false},
		{"percent over 100 refused", func(_ map[string]any, b map[string]any) {
			s := state("dim_level")
			s["dim_level_percent"] = 101
			b["states"] = []any{s}
		}, false},
		{"state id required", func(_ map[string]any, b map[string]any) {
			s := state("setting")
			delete(s, "state_id")
			b["states"] = []any{s}
		}, false},
		{"state id slug", func(_ map[string]any, b map[string]any) {
			s := state("setting")
			s["state_id"] = "Bad ID"
			b["states"] = []any{s}
		}, false},
		{"state role required", func(_ map[string]any, b map[string]any) {
			s := state("setting")
			delete(s, "role")
			b["states"] = []any{s}
		}, false},
		{"state colorimetry required", func(_ map[string]any, b map[string]any) {
			s := state("setting")
			delete(s, "colorimetry")
			b["states"] = []any{s}
		}, false},
		{"state CCT required", func(_ map[string]any, b map[string]any) {
			s := state("setting")
			s["colorimetry"] = map[string]any{}
			b["states"] = []any{s}
		}, false},
		{"rated off-bin CCT", func(_ map[string]any, b map[string]any) {
			s := state("setting")
			s["colorimetry"] = map[string]any{"measured_cct_k": map[string]any{"value": 1850, "value_type": "rated"}}
			b["states"] = []any{s}
		}, true},
		{"channel id required", func(_ map[string]any, b map[string]any) { b["channels"] = []any{map[string]any{"kind": "white"}} }, false},
		{"channel kind required", func(_ map[string]any, b map[string]any) { b["channels"] = []any{map[string]any{"channel_id": "white"}} }, false},
		{"channel kind enum", func(_ map[string]any, b map[string]any) {
			b["channels"] = []any{map[string]any{"channel_id": "white", "kind": "other"}}
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := loadOrFail(t, filepath.Join(root, "examples", "erco-quintessence-30416-023.ulc")).(map[string]any)
			r["configuration"].(map[string]any)["tested_axes"].(map[string]any)["color_tunability"] = "tunable_white"
			b := map[string]any{"baseline_role": "setting", "states": []any{state("range_low_end")}}
			r["color_tuning"] = b
			tt.change(r, b)
			report := findings.NewReport()
			v.Validate(r, report)
			if got := !report.HasErrors(); got != tt.valid {
				t.Fatalf("valid = %v, want %v: %+v", got, tt.valid, report.Findings)
			}
		})
	}
}
