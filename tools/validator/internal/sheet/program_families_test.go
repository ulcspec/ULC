package sheet

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func taxonomyEnum(t *testing.T, definition string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(filepath.Dir(schemaDir(t)), "schema", "taxonomy.schema.json"))
	if err != nil {
		t.Fatalf("read taxonomy schema: %v", err)
	}
	var schema struct {
		Defs map[string]struct {
			Enum []string `json:"enum"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("parse taxonomy schema: %v", err)
	}
	values := schema.Defs[definition].Enum
	if len(values) == 0 {
		t.Fatalf("%s enum is empty", definition)
	}
	out := map[string]bool{}
	for _, value := range values {
		out[value] = true
	}
	return out
}

func TestProgramFamiliesAreExactAndExhaustive(t *testing.T) {
	wantFamilies := map[attestationFamily][]string{
		attestationFamilyPhotometric: {"lm_79", "lm_79_08", "lm_79_19", "lm_79_24"},
		attestationFamilyMaintenance: {"lm_80", "lm_80_08", "lm_80_15", "lm_80_20", "lm_80_21", "tm_21", "tm_21_11", "tm_21_21"},
		attestationFamilyFlicker:     {"ieee_1789_2015", "lm_90_20", "nema_77_2017"},
		attestationFamilyMelanopic:   {"rp_46", "rp_46_23"},
	}
	gotFamilies := map[attestationFamily][]string{}
	for token, family := range programFamilies {
		gotFamilies[family] = append(gotFamilies[family], token)
	}
	for family := range gotFamilies {
		sort.Strings(gotFamilies[family])
	}
	if !reflect.DeepEqual(gotFamilies, wantFamilies) {
		t.Errorf("program families = %#v, want %#v", gotFamilies, wantFamilies)
	}

	enum := taxonomyEnum(t, "AttestationProgram")
	covered := map[string]bool{}
	for token := range programFamilies {
		if !enum[token] {
			t.Errorf("anchoring token %q is not declared by AttestationProgram", token)
		}
		covered[token] = true
	}
	for token := range nonAnchoringPrograms {
		if !enum[token] {
			t.Errorf("residue token %q is not declared by AttestationProgram", token)
		}
		if covered[token] {
			t.Errorf("token %q is both anchoring and residue", token)
		}
		covered[token] = true
	}
	for token := range enum {
		if !covered[token] {
			t.Errorf("AttestationProgram token %q is not triaged", token)
		}
	}
	if len(covered) != len(enum) {
		t.Errorf("program partition covers %d tokens, enum declares %d", len(covered), len(enum))
	}
}

func TestFamilyAnchorsUseAuthoredFamiliesAndSortedIDs(t *testing.T) {
	attestations := []any{
		map[string]any{"program": "lm_79_24", "attestation_id": "photo-z"},
		map[string]any{"program": "lm_79", "attestation_id": "photo-a"},
		map[string]any{"program": "tm_21_21", "attestation_id": "maintenance"},
		map[string]any{"program": "ieee_1789_2015", "attestation_id": "flicker"},
		map[string]any{"program": "rp_46", "attestation_id": "melanopic"},
		map[string]any{"program": "cie_13", "attestation_id": "residue"},
	}
	anchors := familyAnchors(attestations)
	if got := anchors[attestationFamilyPhotometric]; got.count != 2 || !reflect.DeepEqual(got.ids, []string{"photo-a", "photo-z"}) {
		t.Errorf("photometric anchor = %#v, want sorted two-id ambiguity", got)
	}
	for family, want := range map[attestationFamily]string{
		attestationFamilyMaintenance: "maintenance",
		attestationFamilyFlicker:     "flicker",
		attestationFamilyMelanopic:   "melanopic",
	} {
		anchor := anchors[family]
		if anchor.count != 1 || !reflect.DeepEqual(anchor.ids, []string{want}) {
			t.Errorf("%s anchor = %#v, want one id %q", family, anchor, want)
		}
	}
	for _, anchor := range anchors {
		if reflect.DeepEqual(anchor.ids, []string{"residue"}) {
			t.Error("declared residue program became an anchor")
		}
	}
}

func TestResidueProgramCannotAnchorMeasuredValue(t *testing.T) {
	ctx := newProvenanceContext([]any{
		map[string]any{"program": "cie_13", "attestation_id": "not-a-flicker-anchor"},
	})
	_, err := resolveProvenanceForField("flicker_value", provenanceDefaults{
		valueType: "measured",
		source:    "test_report",
		method:    "extracted",
		family:    attestationFamilyFlicker,
	}, Row{}, ctx)
	if err == nil {
		t.Fatal("measured flicker value auto-linked through a residue program")
	}
	for _, want := range []string{"flicker_value", "flicker"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q", err, want)
		}
	}
}

func TestResolverSelectsOnlyTheDeclaredProgramFamily(t *testing.T) {
	ctx := newProvenanceContext([]any{
		map[string]any{"program": "lm_79_24", "attestation_id": "photometric"},
		map[string]any{"program": "lm_80_21", "attestation_id": "maintenance"},
		map[string]any{"program": "lm_90_20", "attestation_id": "flicker"},
		map[string]any{"program": "rp_46_23", "attestation_id": "melanopic"},
	})
	for family, want := range map[attestationFamily]string{
		attestationFamilyPhotometric: "photometric",
		attestationFamilyMaintenance: "maintenance",
		attestationFamilyFlicker:     "flicker",
		attestationFamilyMelanopic:   "melanopic",
	} {
		resolved, err := resolveProvenanceForField("measured_value", provenanceDefaults{
			valueType: "measured",
			source:    "test_report",
			method:    "extracted",
			family:    family,
		}, Row{}, ctx)
		if err != nil {
			t.Errorf("family %s: %v", family, err)
			continue
		}
		if got := resolved.provenance["attestation_ref"]; got != want {
			t.Errorf("family %s selected %v, want %q", family, got, want)
		}
	}
}

func TestConfirmationRequiredAttestationCannotAnchorMeasuredEvidence(t *testing.T) {
	attestations := []any{
		map[string]any{
			"program":        "lm_90_20",
			"attestation_id": "case-flicker",
			"verification":   map[string]any{"type": "requires_manufacturer_confirmation"},
		},
	}
	ctx := newProvenanceContext(attestations)
	_, err := resolveProvenanceForField("value", provenanceDefaults{
		valueType: "measured",
		source:    "test_report",
		method:    "extracted",
		family:    attestationFamilyFlicker,
	}, Row{}, ctx)
	if err == nil || !strings.Contains(err.Error(), "no flicker") {
		t.Fatalf("automatic case-by-case anchor error = %v", err)
	}

	_, err = resolveProvenanceForField("value", provenanceDefaults{
		valueType: "measured",
		source:    "test_report",
		method:    "extracted",
		family:    attestationFamilyFlicker,
	}, Row{"value__attestation_ref": "case-flicker"}, ctx)
	if err == nil || !strings.Contains(err.Error(), "requires manufacturer confirmation") {
		t.Fatalf("explicit case-by-case anchor error = %v", err)
	}
}

func TestExplicitReferencesMustNameOneAttestationInTheDeclaredFamily(t *testing.T) {
	ctx := newProvenanceContext([]any{
		map[string]any{"program": "lm_79_24", "attestation_id": "photometric"},
		map[string]any{"program": "lm_90_20", "attestation_id": "duplicate"},
		map[string]any{"program": "nema_77_2017", "attestation_id": "duplicate"},
	})
	tests := []struct {
		name string
		row  Row
		want string
	}{
		{name: "missing direct", row: Row{"value__attestation_ref": "missing"}, want: "no attestation"},
		{name: "wrong family direct", row: Row{"value__attestation_ref": "photometric"}, want: "different evidence family"},
		{name: "duplicate direct", row: Row{"value__attestation_ref": "duplicate"}, want: "declared by 2 attestations"},
		{name: "wrong family base", row: Row{"value__value_type": "rated", "value__prov_method": "scaled", "value__base_attestation_ref": "photometric"}, want: "different evidence family"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolveProvenanceForField("value", provenanceDefaults{
				valueType: "measured",
				source:    "test_report",
				method:    "extracted",
				family:    attestationFamilyFlicker,
			}, test.row, ctx)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestAutomaticReferencesRejectDuplicateIDsAcrossFamilies(t *testing.T) {
	ctx := newProvenanceContext([]any{
		map[string]any{"program": "lm_79_24", "attestation_id": "shared-id"},
		map[string]any{"program": "lm_90_20", "attestation_id": "shared-id"},
	})
	_, err := resolveProvenanceForField("value", provenanceDefaults{
		valueType: "measured",
		source:    "test_report",
		method:    "extracted",
		family:    attestationFamilyFlicker,
	}, Row{}, ctx)
	if err == nil || !strings.Contains(err.Error(), "declared by 2 attestations") {
		t.Fatalf("automatic duplicate-id error = %v", err)
	}
	if got := ctx.singleAnchorID(attestationFamilyPhotometric); got != "" {
		t.Fatalf("generated selection accepted duplicate id %q", got)
	}
}
