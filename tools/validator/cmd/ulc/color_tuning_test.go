package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
	"github.com/ulcspec/ULC/tools/validator/internal/index"
)

func TestValidateColorTuningAfterNumberNormalization(t *testing.T) {
	raw, err := os.ReadFile(exampleRecord(t, "erco-quintessence-30416-023.ulc"))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeStrict(raw)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := normalizeNumbers(decoded)
	if err != nil {
		t.Fatal(err)
	}
	r := normalized.(map[string]any)
	r["configuration"].(map[string]any)["tested_axes"].(map[string]any)["color_tunability"] = "multichannel"
	r["configuration"].(map[string]any)["tested_conditions"].(map[string]any)["nominal_cct_at_test"] = "3000"
	r["color_tuning"] = map[string]any{"baseline_role": "setting", "states": []any{map[string]any{"state_id": "baseline-copy", "role": "setting", "colorimetry": map[string]any{"measured_cct_k": map[string]any{"value": 3000, "value_type": "rated"}}}}}
	r["index"] = index.Build(r)
	body, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "repeated.ulc")
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	out, code := captureStdout(t, func() int { return runValidate([]string{path, "--json"}) })
	if code != 1 || !strings.Contains(out, string(findings.CodeColorTuningInconsistent)) || !strings.Contains(out, "V5:") {
		t.Fatalf("exit = %d, output = %s", code, out)
	}
}
