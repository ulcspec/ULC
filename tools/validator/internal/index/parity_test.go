package index

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// TestBuilderSchemaParity replaces the retired Python builder-parity-guard
// and catches the two classes of drift the Python guard caught:
//
//  1. Builder emits a key the schema does not declare (`unknown_key` errors
//     on downstream validators).
//  2. Schema lists a key as required but the builder does not produce it
//     under any input (records silently fail core-level validation).
//
// Coverage comes from a maximal synthetic fixture that triggers every emit
// path in Build(). Unlike TestBuilderMatchesStoredIndex — which only exercises
// the branches the four canonical reference records happen to hit — this
// fixture forces every branch, so a newly added emit site without matching
// schema support (or a newly required schema key without builder coverage)
// fails loudly.
func TestBuilderSchemaParity(t *testing.T) {
	schemaPath := filepath.Join(repoRoot(t), "schema", "ulc.schema.json")
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read %s: %v", schemaPath, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var schema map[string]any
	if err := dec.Decode(&schema); err != nil {
		t.Fatalf("parse schema: %v", err)
	}

	defs, _ := schema["$defs"].(map[string]any)
	indexDef, _ := defs["Index"].(map[string]any)
	propsMap, _ := indexDef["properties"].(map[string]any)
	requiredArr, _ := indexDef["required"].([]any)

	schemaProps := map[string]struct{}{}
	for k := range propsMap {
		schemaProps[k] = struct{}{}
	}
	schemaRequired := map[string]struct{}{}
	for _, k := range requiredArr {
		if s, ok := k.(string); ok {
			schemaRequired[s] = struct{}{}
		}
	}

	built := Build(maximalFixture())
	emittedKeys := map[string]struct{}{}
	for k := range built {
		emittedKeys[k] = struct{}{}
	}

	// Check 1: every emitted key is declared in schema Index.properties.
	unknown := difference(emittedKeys, schemaProps)
	if len(unknown) > 0 {
		t.Errorf("builder emits %d key(s) not declared in schema Index.properties: %v",
			len(unknown), unknown)
	}

	// Check 2: every schema-required key is produced under maximal input.
	missing := difference(schemaRequired, emittedKeys)
	if len(missing) > 0 {
		t.Errorf("schema requires %d key(s) the builder does not produce: %v",
			len(missing), missing)
	}
}

// repoRoot shares the resolution logic with builder_test.go but lives in its
// own function so the two tests can be run independently.
//
// Tests run from tools/validator/internal/index; four levels up is repo root.
func repoRoot(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return p
}

// maximalFixture is a synthetic record that triggers every emit path in
// Build(). Values are not realistic — the only goal is to walk every branch
// the builder consults. Keep in sync with builder.go's emission points; a
// change there that adds a new emit path needs a matching field here.
func maximalFixture() map[string]any {
	return map[string]any{
		"record_id":         "test-record",
		"conformance_level": "full",
		"product_family": map[string]any{
			"family_id":           "test-fam",
			"family_display_name": "Test Family",
			"catalog_line":        "TF",
			"catalog_model":       "TF-100",
			"primary_category":    "downlight",
			"secondary_function":  []any{"wall_wash"},
			"indoor_outdoor":      "indoor",
			"mounting_types":      []any{"recessed_ceiling"},
			"environment_rating":  "dry",
			"manufacturer": map[string]any{
				"slug":         "testmfr",
				"display_name": "Test Manufacturer",
			},
			"shared_mechanical": map[string]any{
				"ip_rating": "IP20",
				"ik_rating": "IK06",
			},
			"shared_attestations": []any{
				map[string]any{"program": "ul_listed"},
			},
		},
		"configuration": map[string]any{
			"photometric_scenario_id": "test-scenario",
			"scenario_label":          "Test Scenario",
			"catalog_number":          "TF-100-30-90",
			"tested_axes":             map[string]any{"color_tunability": "static_white"},
			"tested_conditions":       map[string]any{"nominal_cct_at_test": "3000"},
		},
		"electrical": map[string]any{
			"input_power_w":   map[string]any{"value": int64(17)},
			"driver_protocol": "0-10v",
		},
		"photometry": map[string]any{
			"total_luminous_flux_lm":      map[string]any{"value": int64(1301)},
			"luminaire_efficacy_lm_per_w": map[string]any{"value": int64(77)},
			"distribution_type":           "symmetric",
			"beam_family":                 "medium_flood",
			"ugr_4h_8h":                   map[string]any{"value": int64(19)},
		},
		"colorimetry": map[string]any{
			"cri_ra": map[string]any{"value": int64(92)},
		},
		"outdoor_classification": map[string]any{
			"outdoor_distribution_type": "type_iii",
			"bug_rating": map[string]any{
				"b": int64(1),
				"u": int64(0),
				"g": int64(1),
			},
		},
		"attestations": []any{
			map[string]any{"program": "lm_79_24"},
		},
		"sustainability_declaration": map[string]any{
			"declaration_type": "ilfi_declare",
		},
		"source_files": []any{
			map[string]any{
				"file_type": "datasheet_pdf",
				"reference": map[string]any{
					"filename": "x.pdf",
					"sha256":   "0000000000000000000000000000000000000000000000000000000000000000",
				},
			},
			map[string]any{
				"file_type": "ies_file",
				"reference": map[string]any{
					"filename": "x.ies",
					"sha256":   "1111111111111111111111111111111111111111111111111111111111111111",
				},
			},
		},
	}
}

func difference(a, b map[string]struct{}) []string {
	out := []string{}
	for k := range a {
		if _, ok := b[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
