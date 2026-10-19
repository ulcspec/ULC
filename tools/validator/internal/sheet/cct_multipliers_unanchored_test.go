package sheet

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func unanchoredMultiplierBundle(t *testing.T, cells map[string]string, attestations ...map[string]string) string {
	t.Helper()
	bundle := isolatedColourBundle(t, nil)
	for header, value := range cells {
		setClaimsCell(t, bundle, "records", header, value)
	}
	setColourAttestations(t, bundle, attestations...)
	id := bundleRecordID(t, bundle)
	writeCSVRows(t, filepath.Join(bundle, "covered_axes.csv"), [][]string{
		{"record_id", "axis_key", "value", "baseline_axis_value", "rationale", "derivation_method"},
		{id, "cct", "3000", "4000", "Synthetic CCT scaling", "cct_multiplier"},
		{id, "cct", "4000", "4000", "Synthetic CCT scaling", "cct_multiplier"},
	})
	writeCSVRows(t, filepath.Join(bundle, "cct_multipliers.csv"), [][]string{
		{"record_id", "cct", "multiplier"}, {id, "3000", "0.95"}, {id, "4000", "1"},
	})
	return bundle
}

func TestCCTMultipliersWithoutLM79AcrossReaders(t *testing.T) {
	bundle := unanchoredMultiplierBundle(t, nil)
	var first map[string]any
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			results, err := Convert(input, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if results[0].Pattern != PatternB {
				t.Fatalf("pattern = %s, want B", results[0].Pattern)
			}
			rec := results[0].Record
			headline := provenancedNumberAt(t, rec, "photometry.total_luminous_flux_lm")
			declared, _ := getPath(rec, "photometry.declared_by_cct")
			rows := declared.([]any)
			if len(rows) != 2 {
				t.Fatalf("declared rows = %d", len(rows))
			}
			scaled := rows[0].(map[string]any)
			baseline := rows[1].(map[string]any)
			if scaled["cct"] != "3000" || baseline["cct"] != "4000" {
				t.Fatalf("row order = %v", rows)
			}
			if !reflect.DeepEqual(baseline["lumens"], headline) || fmt.Sprint(headline["value"]) != "1240" {
				t.Fatalf("baseline = %v, headline = %v", baseline, headline)
			}
			want := map[string]any{"value": int64(1178), "unit": "lm", "value_type": "rated", "provenance": map[string]any{"source": "datasheet_pdf", "method": "scaled", "extension_method": "cct_multiplier"}}
			if !reflect.DeepEqual(scaled["lumens"], want) {
				t.Fatalf("scaled row = %v, want %v", scaled["lumens"], want)
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

func TestCCTMultipliersAmbiguousAnchorTakesUnanchoredShape(t *testing.T) {
	bundle := unanchoredMultiplierBundle(t, nil, colourAttestation("lm79_a", "lm_79_08"), colourAttestation("lm79_b", "lm_79_19"))
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			results, err := Convert(input, Options{})
			if err != nil {
				t.Fatal(err)
			}
			rec := results[0].Record
			headline := provenancedNumberAt(t, rec, "photometry.total_luminous_flux_lm")
			declared, _ := getPath(rec, "photometry.declared_by_cct")
			rows := declared.([]any)
			if !reflect.DeepEqual(rows[1].(map[string]any)["lumens"], headline) {
				t.Fatal("ambiguous anchor baseline differs from headline")
			}
			want := map[string]any{"value": int64(1178), "unit": "lm", "value_type": "rated", "provenance": map[string]any{"source": "datasheet_pdf", "method": "scaled", "extension_method": "cct_multiplier"}}
			if !reflect.DeepEqual(rows[0].(map[string]any)["lumens"], want) {
				t.Fatalf("ambiguous scaled row = %v", rows[0])
			}
			wantSchemaValid(t, rec)
		})
	}
}

func TestCCTMultipliersAnchoredShapeUnchanged(t *testing.T) {
	bundle := t.TempDir()
	copyBundle(t, filepath.Join("testdata", "bundle-b"), bundle)
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			results, err := Convert(input, Options{})
			if err != nil {
				t.Fatal(err)
			}
			declared, _ := getPath(results[0].Record, "photometry.declared_by_cct")
			found := map[string]bool{}
			for _, item := range declared.([]any) {
				row := item.(map[string]any)
				cct := row["cct"].(string)
				var want map[string]any
				switch cct {
				case "3000":
					want = map[string]any{"value": int64(4200), "unit": "lm", "value_type": "measured", "provenance": map[string]any{"source": "ies", "method": "extracted", "attestation_ref": "lm79_lumos_skyline_sr_ho"}}
				case "4000":
					want = map[string]any{"value": int64(4494), "unit": "lm", "value_type": "rated", "provenance": map[string]any{"source": "datasheet_pdf", "method": "scaled", "base_attestation_ref": "lm79_lumos_skyline_sr_ho", "extension_method": "cct_multiplier"}}
				default:
					continue
				}
				found[cct] = true
				if !reflect.DeepEqual(row["lumens"], want) {
					t.Fatalf("anchored %s = %v, want %v", cct, row["lumens"], want)
				}
			}
			if len(found) != 2 {
				t.Fatalf("anchored rows missing: %v", found)
			}
			wantSchemaValid(t, results[0].Record)
		})
	}
}

func TestCCTMultipliersStillNeedBaselineFlux(t *testing.T) {
	bundle := unanchoredMultiplierBundle(t, map[string]string{"total_luminous_flux_lm": "", "luminaire_efficacy_lm_per_w": ""})
	for reader, input := range supplementaryInputs(t, bundle) {
		t.Run(reader, func(t *testing.T) {
			_, err := Convert(input, Options{})
			want := `a cct_multipliers table is present but photometry.total_luminous_flux_lm.value (the multiplier baseline) is missing`
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("refusal = %v, want %s", err, want)
			}
		})
	}
}

func TestGenerateDeclaredByCCTUnanchoredCopiesHeadline(t *testing.T) {
	provenance := map[string]any{"source": "datasheet_pdf", "method": "transcribed"}
	headline := map[string]any{"value": int64(1240), "unit": "lm", "value_type": "rated", "provenance": provenance}
	rows := generateDeclaredByCCT(map[string]float64{"3000": 0.95, "4000": 1.1}, []string{"3000", "4000"}, 1240, "4000", "", headline)
	baseline := rows[1].(map[string]any)["lumens"].(map[string]any)
	if !reflect.DeepEqual(baseline, headline) {
		t.Fatalf("baseline = %v, want headline %v", baseline, headline)
	}
	for _, item := range rows {
		lum := item.(map[string]any)["lumens"].(map[string]any)
		if _, ok := lum["provenance"].(map[string]any)["base_attestation_ref"]; ok {
			t.Fatal("unanchored row has base key")
		}
	}
	baseline["provenance"].(map[string]any)["source"] = "ies"
	if provenance["source"] != "datasheet_pdf" {
		t.Fatal("output mutation changed headline provenance")
	}
}
