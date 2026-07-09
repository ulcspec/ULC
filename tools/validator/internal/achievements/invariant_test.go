package achievements

import (
	"bytes"
	"encoding/json"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ulcspec/ULC/tools/validator/internal/completeness"
)

// projectInputs strips a record to the three inputs Compute is allowed to read: the
// merged ledger (top-level attestations[] and product_family.shared_attestations[]),
// sustainability_declaration, and record_status_as_of.
func projectInputs(full map[string]any) map[string]any {
	proj := map[string]any{}
	if v, ok := full["attestations"]; ok {
		proj["attestations"] = v
	}
	if v, ok := full["sustainability_declaration"]; ok {
		proj["sustainability_declaration"] = v
	}
	if v, ok := full["record_status_as_of"]; ok {
		proj["record_status_as_of"] = v
	}
	if pf, ok := full["product_family"].(map[string]any); ok {
		if sa, ok := pf["shared_attestations"]; ok {
			proj["product_family"] = map[string]any{"shared_attestations": sa}
		}
	}
	return proj
}

// 5.8(a) REPRODUCIBLE-INPUTS-ONLY over all examples: Compute(full) == Compute(projection).
// Any field Compute wrongly reads that a real record carries would diverge here.
func TestReproducibleInputsOverExamples(t *testing.T) {
	for _, path := range exampleFiles(t) {
		full := loadRecord(t, path)
		if !reflect.DeepEqual(Compute(full), Compute(projectInputs(full))) {
			t.Errorf("%s: Compute(full) != Compute(three-input projection)", filepath.Base(path))
		}
	}
}

// 5.8(b) Kitchen-sink negative fixture: every non-input top-level key populated with
// sentinel values, asserted equal to the same record stripped to the three inputs. This
// is the teeth the monolithic Compute would otherwise lack.
func TestReproducibleInputsKitchenSink(t *testing.T) {
	sharedAtts := []any{att("darksky_approved", nil)}
	stripped := map[string]any{
		"attestations":               []any{att("ul_924", map[string]any{"source_document_ref": evidence()})},
		"sustainability_declaration": map[string]any{"declaration_type": "red_list_approved"},
		"record_status_as_of":        "2026-07-01",
		"product_family":             map[string]any{"shared_attestations": sharedAtts},
	}
	full := map[string]any{
		"attestations":               []any{att("ul_924", map[string]any{"source_document_ref": evidence()})},
		"sustainability_declaration": map[string]any{"declaration_type": "red_list_approved"},
		"record_status_as_of":        "2026-07-01",
		"product_family": map[string]any{
			"shared_attestations": sharedAtts,
			"primary_category":    "panel_troffer",
		},
		// Every non-input top-level key, with sentinel content Compute must ignore.
		"emergency":          map[string]any{"emergency_role": "emergency_power_option", "power_source": "battery_integral"},
		"secondary_function": []any{"wall_wash"},
		"exit_sign":          map[string]any{"illumination_mode": "internally_illuminated"},
		"photometry":         map[string]any{"total_luminous_flux_lm": map[string]any{"value": 1000.0}},
		"electrical":         map[string]any{"input_power_w": map[string]any{"value": 10.0}},
		"colorimetry":        map[string]any{"cri_ra": map[string]any{"value": 90.0}},
	}
	if !reflect.DeepEqual(Compute(full), Compute(stripped)) {
		t.Errorf("Compute read a non-input field: kitchen-sink != stripped")
	}
}

// 5.9 Compute does not mutate its input record.
func TestComputeDoesNotMutate(t *testing.T) {
	for _, path := range exampleFiles(t) {
		record := loadRecord(t, path)
		before, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal before: %v", err)
		}
		_ = Compute(record)
		after, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal after: %v", err)
		}
		if !bytes.Equal(before, after) {
			t.Errorf("%s: Compute mutated the input record", filepath.Base(path))
		}
	}
}

// 5.9 Completeness isolation: AchievedLevel is byte-identical with and without
// achievements-only inputs (a sustainability_metric and an issuing_authority added to an
// existing attestation). The safety-listing and lm_79 gates are completeness's own reads;
// the new sustainability fields are not read by the grader.
func TestCompletenessIgnoresAchievementInputs(t *testing.T) {
	base := loadRecord(t, exampleByName(t, "erco-quintessence-30416-023.ulc"))
	withExtra := deepCopyRecord(t, base)
	atts, ok := withExtra["attestations"].([]any)
	if !ok || len(atts) == 0 {
		t.Fatalf("erco record has no attestations to augment")
	}
	first, ok := atts[0].(map[string]any)
	if !ok {
		t.Fatalf("first attestation is not an object")
	}
	first["sustainability_metric"] = map[string]any{
		"embodied_carbon_kgco2e":          10.0,
		"embodied_carbon_scope":           "a1_a3",
		"embodied_carbon_functional_unit": "one luminaire",
	}
	first["issuing_authority"] = "EPD International"

	if completeness.AchievedLevel(base) != completeness.AchievedLevel(withExtra) {
		t.Errorf("AchievedLevel moved when achievements-only inputs were added: base=%s withExtra=%s",
			completeness.AchievedLevel(base), completeness.AchievedLevel(withExtra))
	}
}

// 5.9 Copy-not-import boundary: the achievements and completeness packages must never
// import each other (the Result-shape contract is copy, not import). Enforced by parsing
// each package's non-test import specs, so it holds independent of any behavioral test.
func TestCopyNotImportBoundary(t *testing.T) {
	const achPath = "github.com/ulcspec/ULC/tools/validator/internal/achievements"
	const compPath = "github.com/ulcspec/ULC/tools/validator/internal/completeness"

	achImports := packageImports(t, ".")
	compImports := packageImports(t, filepath.Join("..", "completeness"))

	if achImports[compPath] {
		t.Error("achievements imports completeness; the contract is copy, not import")
	}
	if compImports[achPath] {
		t.Error("completeness imports achievements; the contract is copy, not import")
	}
}

// packageImports returns the set of import paths in a package's non-test .go files.
func packageImports(t *testing.T, dir string) map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", dir, err)
	}
	out := map[string]bool{}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			for _, imp := range f.Imports {
				out[strings.Trim(imp.Path.Value, `"`)] = true
			}
		}
	}
	return out
}
