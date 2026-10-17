package sheet

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	embedded "github.com/ulcspec/ULC/schema"
	"github.com/ulcspec/ULC/tools/validator/internal/colortuning"
)

var colorIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var colorEnumsOnce sync.Once
var colorEnums map[string]map[string]bool
var colorEnumsErr error

// Read converter refusal domains from the canonical taxonomy, without a second
// hand-maintained copy of its roles, channel kinds, or nominal CCT bins.
func colorEnum(name, token string) bool {
	colorEnumsOnce.Do(func() {
		var doc struct {
			Defs map[string]struct {
				Enum []string `json:"enum"`
			} `json:"$defs"`
		}
		colorEnumsErr = json.Unmarshal(embedded.TaxonomySchemaJSON, &doc)
		colorEnums = map[string]map[string]bool{}
		for name, def := range doc.Defs {
			members := map[string]bool{}
			for _, v := range def.Enum {
				members[v] = true
			}
			colorEnums[name] = members
		}
	})
	return colorEnumsErr == nil && colorEnums[name][token]
}

func checkColorDim(role, raw string) error {
	if role == "dim_level" && raw == "" {
		return fmt.Errorf("dim_level requires dim_level_percent")
	}
	if role == "setting" && raw != "" {
		return fmt.Errorf("setting forbids dim_level_percent")
	}
	if raw != "" {
		v, err := parseFloat(raw)
		if err != nil || v <= 0 || v > 100 {
			return fmt.Errorf("dim_level_percent must be numeric and in (0, 100]")
		}
	}
	return nil
}

func checkColorTuningRows(wb Workbook, id string, master Row, recordRow int) error {
	stateRows, channelRows := wb.RowsFor("color_states", id), wb.RowsFor("emission_channels", id)
	role := master["color_tuning_baseline_role"]
	if len(stateRows)+len(channelRows) == 0 && role == "" && master["color_tuning_baseline_label"] == "" && master["color_tuning_baseline_dim_level_percent"] == "" {
		return nil
	}
	recordsError := func(msg string) error { return fmt.Errorf("records row %d for %q: %s", recordRow, id, msg) }
	capability := master["color_tunability"]
	if capability == "" || capability == "static_white" {
		for _, sheet := range []string{"color_states", "emission_channels"} {
			if len(wb.RowsFor(sheet, id)) > 0 {
				return fmt.Errorf("%s row 1 for %q: rows require a non-static color_tunability", sheet, id)
			}
		}
		return recordsError("color_tuning requires a non-static color_tunability")
	}
	if role == "" {
		return recordsError("color states or channels require color_tuning_baseline_role")
	}
	if len(stateRows)+len(channelRows) == 0 {
		return recordsError("color_tuning_baseline_role requires color_states or emission_channels rows")
	}
	if !colorEnum("ColorStateRole", role) {
		return recordsError("color_tuning_baseline_role is outside ColorStateRole")
	}
	if err := checkColorDim(role, master["color_tuning_baseline_dim_level_percent"]); err != nil {
		return recordsError(err.Error())
	}
	for _, spec := range []struct{ sheet, id, enum, field string }{{"color_states", "state_id", "ColorStateRole", "role"}, {"emission_channels", "channel_id", "EmissionChannelKind", "kind"}} {
		seen := map[string]bool{}
		for i, row := range wb.RowsFor(spec.sheet, id) {
			bad := func(msg string) error { return fmt.Errorf("%s row %d for %q: %s", spec.sheet, i+1, id, msg) }
			key := row[spec.id]
			if !colorIDPattern.MatchString(key) {
				return bad(spec.id + " must be a nonempty lowercase slug")
			}
			if seen[key] {
				return bad("repeated " + spec.id)
			}
			seen[key] = true
			if !colorEnum(spec.enum, row[spec.field]) {
				return bad(spec.field + " is outside " + spec.enum)
			}
			if bin := row["nominal_cct_k"]; bin != "" && !colorEnum("NominalCCT", bin) {
				return bad("nominal_cct_k is not a NominalCCT bin")
			}
			if spec.sheet == "color_states" {
				if row["nominal_cct_k"] == "" && row["measured_cct_k"] == "" {
					return bad("state requires a nominal_cct_k or measured_cct_k")
				}
				if err := checkColorDim(row["role"], row["dim_level_percent"]); err != nil {
					return bad(err.Error())
				}
				if (role == "range_low_end" || role == "range_high_end") && row["role"] == role {
					return bad("listed end repeats the baseline role")
				}
			}
		}
	}
	return nil
}

var colorStateValuePaths = []struct{ field, path string }{
	{"measured_cct_k", "colorimetry.measured_cct_k"}, {"duv", "colorimetry.duv"},
	{"cri_ra", "colorimetry.cri_ra"}, {"cri_r9", "colorimetry.cri_r9"},
	{"tm_30_rf", "colorimetry.tm_30.rf"}, {"tm_30_rg", "colorimetry.tm_30.rg"},
	{"melanopic_der", "alpha_opic_metrics.melanopic_der"}, {"lumens", "lumens"}, {"efficacy_lm_per_w", "efficacy_lm_per_w"},
}

func assembleColorTuning(wb Workbook, id string, rec map[string]any, ctx provenanceContext) error {
	for _, sheet := range []string{"color_states", "emission_channels"} {
		rows := wb.RowsFor(sheet, id)
		if len(rows) == 0 {
			continue
		}
		out := make([]any, 0, len(rows))
		for i, row := range rows {
			entry := map[string]any{}
			fields := []string{"state_id", "role", "label"}
			paths := colorStateValuePaths
			target := "states"
			if sheet == "emission_channels" {
				fields = []string{"channel_id", "kind", "label"}
				paths = []struct{ field, path string }{{"peak_wavelength_nm", "peak_wavelength_nm"}, {"lumens", "lumens"}}
				target = "channels"
			}
			for _, field := range fields {
				copyIf(entry, row, field, field)
			}
			if bin := row["nominal_cct_k"]; bin != "" {
				if err := setPath(entry, "colorimetry.nominal_cct_k", bin); err != nil {
					return err
				}
			}
			if raw := row["dim_level_percent"]; raw != "" && sheet == "color_states" {
				v, err := parseNumber(raw)
				if err != nil {
					return fmt.Errorf("%s row %d for %q: %w", sheet, i+1, id, err)
				}
				entry["dim_level_percent"] = v
			}
			for _, spec := range paths {
				if row[spec.field] == "" {
					continue
				}
				v, err := supplementaryProvenancedNumber(sheet, spec.field, row, ctx)
				if err != nil {
					return fmt.Errorf("%s row %d for %q: %w", sheet, i+1, id, err)
				}
				if err := setPath(entry, spec.path, v); err != nil {
					return err
				}
			}
			out = append(out, entry)
			if err := setPath(rec, "color_tuning."+target, out); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkColorTuningProblems(id string, rec map[string]any, recordRow int) error {
	problems := colortuning.Problems(rec)
	if len(problems) == 0 {
		return nil
	}
	p := problems[0]
	sheet, row := "records", recordRow
	parts := strings.Split(p.Path, "/")
	if len(parts) > 3 {
		if n, err := strconv.Atoi(parts[3]); err == nil {
			row = n + 1
			switch parts[2] {
			case "states":
				sheet = "color_states"
			case "channels":
				sheet = "emission_channels"
			}
		}
	}
	return fmt.Errorf("%s row %d for %q: %s at %s", sheet, row, id, p.Message, p.Path)
}

// Group spectra by state or channel while preserving original row order and row
// numbers. Blank identifiers still target the baseline exactly as before.
func assembleSpectralPowerDistribution(wb Workbook, id string, rec map[string]any, ctx provenanceContext) error {
	rows := wb.RowsFor("spectral_power_distribution", id)
	type key struct{ state, channel string }
	type group struct {
		rows    []Row
		numbers []int
	}
	groups := map[key]*group{}
	var order []key
	for i, row := range rows {
		k := key{row["state_id"], row["channel_id"]}
		if k.state != "" && k.channel != "" {
			return fmt.Errorf("spectral_power_distribution row %d for %q: use state_id or channel_id, never both", i+1, id)
		}
		if groups[k] == nil {
			groups[k] = &group{}
			order = append(order, k)
		}
		groups[k].rows = append(groups[k].rows, row)
		groups[k].numbers = append(groups[k].numbers, i+1)
	}
	for _, k := range order {
		g := groups[k]
		target := rec
		if k.state != "" || k.channel != "" {
			array, idKey, want := "states", "state_id", k.state
			if k.channel != "" {
				array, idKey, want = "channels", "channel_id", k.channel
			}
			block, _ := rec["color_tuning"].(map[string]any)
			entries, _ := block[array].([]any)
			target = nil
			for _, raw := range entries {
				entry, _ := raw.(map[string]any)
				if entry[idKey] == want {
					target = entry
					break
				}
			}
			if target == nil {
				return fmt.Errorf("spectral_power_distribution row %d for %q: unknown %s %q", g.numbers[0], id, idKey, want)
			}
		}
		if err := assembleSpectrumRows(g.rows, g.numbers, id, target, ctx); err != nil {
			return err
		}
	}
	return nil
}
