// Package colortuning resolves published luminaire states without duplicating
// the tested baseline. It has no dependency on other internal packages.
package colortuning

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// State is a resolved luminaire state. Maps reference the authoritative blocks
// in the record, including the spectrum within Colorimetry; no values are merged.
type State struct {
	ID               string
	Role             string
	Label            string
	DimLevelPercent  float64
	CCT              float64
	HasCCT           bool
	Baseline         bool
	Path             string
	Colorimetry      map[string]any
	AlphaOpicMetrics map[string]any
	Lumens           map[string]any
	Efficacy         map[string]any
}

// States returns the tested baseline first, followed by published states in
// record order. An absent dim level means full output (100 percent).
func States(record map[string]any) []State {
	tuning := object(record, "color_tuning")
	color := object(record, "colorimetry")
	cct, hasCCT := stateCCT(color)
	cfg := object(object(record, "configuration"), "tested_conditions")
	if nominal, ok := numeric(cfg["nominal_cct_at_test"]); ok {
		cct, hasCCT = nominal, true
	}
	baseline := State{
		Role: text(tuning, "baseline_role"), Label: text(tuning, "baseline_label"),
		DimLevelPercent: dim(tuning, "baseline_dim_level_percent"), CCT: cct, HasCCT: hasCCT,
		Baseline: true, Colorimetry: color, AlphaOpicMetrics: object(record, "alpha_opic_metrics"),
		Lumens:   object(object(record, "photometry"), "total_luminous_flux_lm"),
		Efficacy: object(object(record, "photometry"), "luminaire_efficacy_lm_per_w"),
	}
	out := []State{baseline}
	rows, _ := tuning["states"].([]any)
	for i, raw := range rows {
		row, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		color := object(row, "colorimetry")
		cct, hasCCT := stateCCT(color)
		out = append(out, State{ID: text(row, "state_id"), Role: text(row, "role"), Label: text(row, "label"),
			DimLevelPercent: dim(row, "dim_level_percent"), CCT: cct, HasCCT: hasCCT,
			Path: fmt.Sprintf("/color_tuning/states/%d", i), Colorimetry: color,
			AlphaOpicMetrics: object(row, "alpha_opic_metrics"), Lumens: object(row, "lumens"), Efficacy: object(row, "efficacy_lm_per_w")})
	}
	return out
}

// Ends returns the low and high end, including a referenced baseline end.
// Resolved is true only when both ends carry a CCT under the single CCT rule.
func Ends(record map[string]any) (low, high State, resolved bool) {
	for _, s := range States(record) {
		switch s.Role {
		case "range_low_end":
			low = s
		case "range_high_end":
			high = s
		}
	}
	return low, high, low.HasCCT && high.HasCCT
}

// Problem is one cross-field inconsistency. Schema validation owns field shapes.
type Problem struct {
	Path    string
	Message string
}

// Problems applies V1 to V6 to the same resolved states used by every reader.
func Problems(record map[string]any) []Problem {
	tuning := object(record, "color_tuning")
	if tuning == nil {
		return nil
	}
	var out []Problem
	add := func(path, msg string) { out = append(out, Problem{path, msg}) }
	for _, pair := range []struct{ key, id string }{{"states", "state_id"}, {"channels", "channel_id"}} {
		seen := map[string]bool{}
		rows, _ := tuning[pair.key].([]any)
		for i, raw := range rows {
			row, _ := raw.(map[string]any)
			id := text(row, pair.id)
			if id == "" {
				continue
			}
			if seen[id] {
				add(fmt.Sprintf("/color_tuning/%s/%d/%s", pair.key, i, pair.id), "V1: repeated "+pair.id)
			}
			seen[id] = true
		}
	}
	states := States(record)
	hasLow, hasHigh := false, false
	for _, s := range states {
		hasLow = hasLow || s.Role == "range_low_end"
		hasHigh = hasHigh || s.Role == "range_high_end"
	}
	if hasLow != hasHigh {
		add("/color_tuning", "V2: a published range must have both ends, counting the baseline")
	}
	low, high, resolved := Ends(record)
	if resolved && low.CCT >= high.CCT {
		add("/color_tuning", "V3: the low-end CCT must be below the high-end CCT")
	}
	capability := text(object(object(record, "configuration"), "tested_axes"), "color_tunability")
	if resolved && (capability == "tunable_white" || capability == "dim_to_warm") {
		for _, s := range states {
			if s.HasCCT && (s.CCT < low.CCT || s.CCT > high.CCT) {
				path := s.Path
				if s.Baseline {
					path = "/color_tuning/baseline_role"
				}
				add(path, "V4: a luminaire state is outside the published white range")
			}
		}
	}
	for i, a := range states {
		for _, b := range states[i+1:] {
			if !a.HasCCT || !b.HasCCT || a.CCT != b.CCT || a.DimLevelPercent != b.DimLevelPercent {
				continue
			}
			if strings.EqualFold(a.Label, b.Label) {
				add(b.Path, "V5: two states share CCT, dim level, and case-folded label")
			}
			if a.Label == "" || b.Label == "" {
				add(b.Path, "V6: states sharing CCT and dim level must each have a label")
			}
		}
	}
	return out
}

func object(m map[string]any, key string) map[string]any { v, _ := m[key].(map[string]any); return v }
func text(m map[string]any, key string) string           { v, _ := m[key].(string); return v }
func dim(m map[string]any, key string) float64 {
	if v, ok := numeric(m[key]); ok {
		return v
	}
	return 100
}
func stateCCT(color map[string]any) (float64, bool) {
	if v, ok := numeric(color["nominal_cct_k"]); ok {
		return v, true
	}
	return numeric(object(color, "measured_cct_k")["value"])
}
func numeric(raw any) (float64, bool) {
	var v float64
	switch n := raw.(type) {
	case string:
		parsed, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0, false
		}
		v = parsed
	case json.Number:
		parsed, err := n.Float64()
		if err != nil {
			return 0, false
		}
		v = parsed
	case float64:
		v = n
	case int64:
		v = float64(n)
	case int:
		v = float64(n)
	default:
		return 0, false
	}
	return v, !math.IsNaN(v) && !math.IsInf(v, 0)
}
