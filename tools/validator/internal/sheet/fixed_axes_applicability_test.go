package sheet

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/completeness"
)

func fixedOnlyBundle(t *testing.T, src string, cells map[string]string, excluded [][]string) string {
	t.Helper()
	bundle := t.TempDir()
	copyBundle(t, filepath.Join("testdata", src), bundle)
	for header, value := range cells {
		setClaimsCell(t, bundle, "records", header, value)
	}
	if excluded != nil {
		rows := [][]string{{"record_id", "axes", "reason"}}
		rows = append(rows, excluded...)
		writeCSVRows(t, filepath.Join(bundle, "excluded_combinations.csv"), rows)
	}
	return bundle
}

func applicabilityKeys(app map[string]any) []string {
	keys := make([]string, 0, len(app))
	for key := range app {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestFixedOnlyApplicabilityAcrossReaders(t *testing.T) {
	bundle := fixedOnlyBundle(t, "bundle", map[string]string{
		"applicable_catalog_pattern":    "ORB-1200-40-{FINISH}",
		"fixed_axes":                    `{"series":"ORB","size":"1200","cct":"40"}`,
		"applicable_sku_count_estimate": "1",
	}, [][]string{
		{"acme-orbit-1200-4000k", `{"finish":"black"}`, "Separate configuration"},
		{"acme-orbit-1200-4000k", `{"finish":["bronze","silver"]}`, "Separate finishes"},
	})
	plain, err := Convert(filepath.Join("testdata", "bundle"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	var first map[string]any
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			results, err := Convert(input, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if results[0].Pattern != PatternA {
				t.Fatalf("pattern = %s, want A", results[0].Pattern)
			}
			rec := results[0].Record
			app := rec["applicability"].(map[string]any)
			wantKeys := []string{"applicable_catalog_pattern", "applicable_sku_count_estimate", "excluded_combinations", "fixed_axes"}
			if !reflect.DeepEqual(applicabilityKeys(app), wantKeys) {
				t.Fatalf("applicability keys = %v", applicabilityKeys(app))
			}
			if app["applicable_catalog_pattern"] != "ORB-1200-40-{FINISH}" || app["applicable_sku_count_estimate"] != int64(1) {
				t.Fatalf("applicability header = %v", app)
			}
			if !reflect.DeepEqual(app["fixed_axes"], map[string]any{"series": "ORB", "size": "1200", "cct": "40"}) {
				t.Fatalf("fixed axes = %v", app["fixed_axes"])
			}
			wantExcluded := []any{
				map[string]any{"axes": map[string]any{"finish": "black"}, "reason": "Separate configuration"},
				map[string]any{"axes": map[string]any{"finish": []any{"bronze", "silver"}}, "reason": "Separate finishes"},
			}
			if !reflect.DeepEqual(app["excluded_combinations"], wantExcluded) {
				t.Fatalf("excluded rows = %v", app["excluded_combinations"])
			}
			if completeness.AchievedLevel(rec) != completeness.AchievedLevel(plain[0].Record) {
				t.Fatal("applicability changed grade")
			}
			wantSchemaValid(t, rec)
			if first == nil {
				first = rec
			} else if !reflect.DeepEqual(first, rec) {
				t.Fatal("CSV and XLSX records differ")
			}
		})
	}
}

func TestFixedOnlyApplicabilityPatternC(t *testing.T) {
	bundle := fixedOnlyBundle(t, "bundle-c", map[string]string{"fixed_axes": `{"color":"RGB"}`}, nil)
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			results, err := Convert(input, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if results[0].Pattern != PatternC {
				t.Fatalf("pattern = %s, want C", results[0].Pattern)
			}
			app := results[0].Record["applicability"].(map[string]any)
			if !reflect.DeepEqual(applicabilityKeys(app), []string{"fixed_axes"}) || !reflect.DeepEqual(app["fixed_axes"], map[string]any{"color": "RGB"}) {
				t.Fatalf("applicability = %v", app)
			}
			wantSchemaValid(t, results[0].Record)
		})
	}
}

func TestFixedOnlyExcludedRowsAloneWriteTheBlock(t *testing.T) {
	bundle := fixedOnlyBundle(t, "bundle", nil, [][]string{{"acme-orbit-1200-4000k", `{"finish":"black"}`, "Separate configuration"}})
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			results, err := Convert(input, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if results[0].Pattern != PatternA {
				t.Fatalf("pattern = %s, want A", results[0].Pattern)
			}
			app := results[0].Record["applicability"].(map[string]any)
			if !reflect.DeepEqual(applicabilityKeys(app), []string{"excluded_combinations"}) {
				t.Fatalf("applicability = %v", app)
			}
			wantSchemaValid(t, results[0].Record)
		})
	}
}

func TestFixedOnlyApplicabilityRefusals(t *testing.T) {
	for _, src := range []string{"bundle", "bundle-c"} {
		for _, tc := range []struct {
			name, column, value, want string
			excluded                  [][]string
		}{
			{name: "malformed fixed axes", column: "fixed_axes", value: `{"series":`, want: `column "fixed_axes": invalid JSON object cell`},
			{name: "invalid estimate", column: "applicable_sku_count_estimate", value: "x", want: `column "applicable_sku_count_estimate": invalid integer "x"`},
			{name: "missing reason", excluded: [][]string{{"acme-orbit-1200-4000k", `{"finish":"black"}`, ""}}, want: `excluded_combinations row 1 for "acme-orbit-1200-4000k": missing reason`},
		} {
			t.Run(src+"/"+tc.name, func(t *testing.T) {
				cells := map[string]string{}
				if tc.column != "" {
					cells[tc.column] = tc.value
				}
				bundle := fixedOnlyBundle(t, src, cells, tc.excluded)
				if tc.excluded != nil {
					rows := readCSVRows(t, filepath.Join(bundle, "excluded_combinations.csv"))
					id := bundleRecordID(t, bundle)
					rows[1][0] = id
					writeCSVRows(t, filepath.Join(bundle, "excluded_combinations.csv"), rows)
				}
				want := strings.ReplaceAll(tc.want, "acme-orbit-1200-4000k", bundleRecordID(t, bundle))
				for reader, input := range supplementaryInputs(t, bundle) {
					t.Run(reader, func(t *testing.T) {
						_, err := Convert(input, Options{})
						if err == nil || !strings.Contains(err.Error(), want) {
							t.Fatalf("refusal = %v, want %s", err, want)
						}
					})
				}
			})
		}
	}
}
