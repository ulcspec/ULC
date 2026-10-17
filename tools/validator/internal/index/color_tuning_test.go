package index

import "testing"

func TestColorRangeProjection(t *testing.T) {
	state := func(role, cct string) map[string]any {
		return map[string]any{"role": role, "colorimetry": map[string]any{"nominal_cct_k": cct}}
	}
	tests := []struct {
		name           string
		role, baseline string
		states         []any
		low, high      float64
		present        bool
	}{
		{"listed ends", "setting", "3000", []any{state("range_low_end", "2700"), state("range_high_end", "6500")}, 2700, 6500, true},
		{"baseline low end", "range_low_end", "2700", []any{state("range_high_end", "6500")}, 2700, 6500, true},
		{"baseline high end", "range_high_end", "3000", []any{state("range_low_end", "1800")}, 1800, 3000, true},
		{"lone end", "setting", "3000", []any{state("range_low_end", "2700")}, 0, 0, false},
		{"settings only", "setting", "3000", []any{state("setting", "4000")}, 0, 0, false},
		{"off-bin rated end", "range_high_end", "3000", []any{map[string]any{"role": "range_low_end", "colorimetry": map[string]any{"measured_cct_k": map[string]any{"value": 1850, "value_type": "rated"}}}}, 1850, 3000, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := map[string]any{"configuration": map[string]any{"tested_conditions": map[string]any{"nominal_cct_at_test": tt.baseline}}, "colorimetry": map[string]any{"nominal_cct_k": "4000"}, "color_tuning": map[string]any{"baseline_role": tt.role, "states": tt.states}}
			idx := Build(r)
			lo, loOK := idx["cct_range_low_k"]
			hi, hiOK := idx["cct_range_high_k"]
			if loOK != tt.present || hiOK != tt.present {
				t.Fatalf("range presence = %v/%v, want %v", loOK, hiOK, tt.present)
			}
			if tt.present {
				a, okA := getNumber(map[string]any{"v": lo}, "v")
				b, okB := getNumber(map[string]any{"v": hi}, "v")
				if !okA || !okB || a != tt.low || b != tt.high {
					t.Fatalf("range = %v/%v, want %v/%v", lo, hi, tt.low, tt.high)
				}
			}
			if idx["nominal_cct_k"] != tt.baseline {
				t.Fatal("tested baseline projection moved")
			}
		})
	}
	if idx := Build(nil); idx["cct_range_low_k"] != nil || idx["cct_range_high_k"] != nil {
		t.Fatal("absent tuning must omit range")
	}
}
