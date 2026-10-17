package completeness

import "testing"

func colorState(role, cct string) map[string]any {
	return map[string]any{"role": role, "colorimetry": map[string]any{"nominal_cct_k": cct, "cri_ra": map[string]any{"value": 90.0}}, "alpha_opic_metrics": map[string]any{"melanopic_der": map[string]any{"value": 0.5}}}
}
func addColorRange(r map[string]any) {
	r["color_tuning"] = map[string]any{"baseline_role": "setting", "states": []any{colorState("range_low_end", "2700"), colorState("range_high_end", "6500")}}
}

func TestColorStatesGradeEndpointsAndKeepBaselineRules(t *testing.T) {
	for _, capability := range []string{"tunable_white", "dim_to_warm"} {
		t.Run(capability, func(t *testing.T) {
			r := fullBase()
			r["configuration"].(map[string]any)["tested_axes"].(map[string]any)["color_tunability"] = capability
			if got := AchievedLevel(r); got != LevelCore {
				t.Fatalf("missing ends grade = %v, want core", got)
			}
			addColorRange(r)
			if got := AchievedLevel(r); got != LevelFull {
				t.Fatalf("complete endpoints grade = %v, want full", got)
			}
			low := r["color_tuning"].(map[string]any)["states"].([]any)[0].(map[string]any)
			delete(low, "alpha_opic_metrics")
			if got := AchievedLevel(r); got != LevelStandard {
				t.Fatalf("missing endpoint DER grade = %v, want standard", got)
			}
			low["alpha_opic_metrics"] = map[string]any{"melanopic_der": map[string]any{"value": 0.3}}
			delete(r["colorimetry"].(map[string]any), "cri_ra")
			if got := AchievedLevel(r); got != LevelIncomplete {
				t.Fatalf("listed-end CRI must not replace baseline CRI: %v", got)
			}
		})
	}
}

func TestColorStatesGradeReferencedBaselineEnd(t *testing.T) {
	r := fullBase()
	r["configuration"].(map[string]any)["tested_axes"].(map[string]any)["color_tunability"] = "tunable_white"
	r["color_tuning"] = map[string]any{"baseline_role": "range_high_end", "states": []any{colorState("range_low_end", "2700")}}
	if !hasColorRange(r) || hasColorEndMetrics(r) {
		t.Fatal("baseline endpoint must require its own DER")
	}
	r["alpha_opic_metrics"] = map[string]any{"melanopic_der": map[string]any{"value": 0.6}}
	if !hasColorEndMetrics(r) || AchievedLevel(r) != LevelFull {
		t.Fatal("baseline endpoint metrics were not counted")
	}
	delete(r["colorimetry"].(map[string]any), "cri_ra")
	if hasColorEndMetrics(r) {
		t.Fatal("baseline CRI must be required at the endpoint")
	}
}

func TestColorStatesEnrichmentNeverGates(t *testing.T) {
	for _, capability := range []string{"rgb", "rgbw", "rgba", "rgbww", "multichannel"} {
		r := standardBase()
		r["configuration"].(map[string]any)["tested_axes"].(map[string]any)["color_tunability"] = capability
		before := AchievedLevel(r)
		addColorRange(r)
		r["color_tuning"].(map[string]any)["channels"] = []any{map[string]any{"channel_id": "white", "kind": "white"}}
		if got := AchievedLevel(r); got != before {
			t.Fatalf("%s channels or states moved grade %v to %v", capability, before, got)
		}
	}
	r := fullBase()
	r["configuration"].(map[string]any)["tested_axes"].(map[string]any)["color_tunability"] = "tunable_white"
	addColorRange(r)
	before := AchievedLevel(r)
	for _, raw := range r["color_tuning"].(map[string]any)["states"].([]any) {
		raw.(map[string]any)["colorimetry"].(map[string]any)["spectral_power_distribution"] = map[string]any{"values": []any{0.1, 0.2}}
	}
	if !hasColorEndSpectra(r) || AchievedLevel(r) != before {
		t.Fatal("end spectra should satisfy enrichment without moving the grade")
	}
	for _, raw := range []any{nil, true, []any{}, map[string]any{"states": []any{nil, 42}}} {
		r := map[string]any{"color_tuning": raw}
		hasColorRange(r)
		hasColorEndMetrics(r)
		hasColorEndSpectra(r)
		AchievedLevel(r)
	}
}
