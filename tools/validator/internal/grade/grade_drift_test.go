package grade

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// This file holds the two schema-drift guards: TestRubricPathsResolve (every
// shape-constructor rubric row resolves to a real schema field whose shape its
// closure accepts) and TestRubricExhaustiveness (every taxonomy enum referenced
// from ulc.schema.json outside /index is either mapped by the rubric or listed in
// descriptiveAllowlist). Both share a small JSON Schema $ref walker.

func loadJSONSchema(t *testing.T, name string) map[string]any {
	t.Helper()
	path := filepath.Join(repoRoot(t), "schema", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return m
}

func mapOf(v any) (map[string]any, bool) { m, ok := v.(map[string]any); return m, ok }

// refTarget splits a $ref into (defName, isTaxonomy, ok).
func refTarget(ref string) (string, bool, bool) {
	const taxPrefix = "taxonomy.schema.json#/$defs/"
	const ulcPrefix = "#/$defs/"
	if i := strings.Index(ref, taxPrefix); i >= 0 {
		return ref[i+len(taxPrefix):], true, true
	}
	if strings.HasPrefix(ref, ulcPrefix) {
		return ref[len(ulcPrefix):], false, true
	}
	return "", false, false
}

// derefObject follows $ref chains (ulc-local only) to the underlying object
// schema so its "properties" can be indexed. Taxonomy refs are leaves and are
// returned as-is.
func derefObject(node, ulcDefs map[string]any) map[string]any {
	seen := map[string]bool{}
	for {
		ref, ok := node["$ref"].(string)
		if !ok {
			return node
		}
		name, isTax, ok := refTarget(ref)
		if !ok || isTax || seen[name] {
			return node
		}
		seen[name] = true
		target, ok := mapOf(ulcDefs[name])
		if !ok {
			return node
		}
		node = target
	}
}

// childSchema returns the schema for property key under node, following $refs and
// allOf branches.
func childSchema(node map[string]any, key string, ulcDefs map[string]any) (map[string]any, bool) {
	node = derefObject(node, ulcDefs)
	if props, ok := mapOf(node["properties"]); ok {
		if child, ok := mapOf(props[key]); ok {
			return child, true
		}
	}
	if allOf, ok := node["allOf"].([]any); ok {
		for _, b := range allOf {
			if bm, ok := mapOf(b); ok {
				if child, ok := childSchema(bm, key, ulcDefs); ok {
					return child, true
				}
			}
		}
	}
	return nil, false
}

// resolveDataPath walks a data JSON Pointer (slash-separated components) from the
// root schema to the leaf property schema.
func resolveDataPath(root map[string]any, comps []string, ulcDefs map[string]any) (map[string]any, bool) {
	node := root
	for _, c := range comps {
		child, ok := childSchema(node, c, ulcDefs)
		if !ok {
			return nil, false
		}
		node = child
	}
	return node, true
}

// classifyLeaf maps a leaf property schema to one of provnumber/array/object/
// string/scalar so a representative value can be built.
func classifyLeaf(node, ulcDefs, taxDefs map[string]any) string {
	// Unwrap a single-branch allOf wrapper ({allOf:[{$ref}], description:...}).
	if allOf, ok := node["allOf"].([]any); ok && len(allOf) == 1 {
		if b, ok := mapOf(allOf[0]); ok {
			node = b
		}
	}
	if ref, ok := node["$ref"].(string); ok {
		name, isTax, _ := refTarget(ref)
		if !isTax && name == "ProvenancedNumber" {
			return "provnumber"
		}
		defs := ulcDefs
		if isTax {
			defs = taxDefs
		}
		if target, ok := mapOf(defs[name]); ok {
			node = target
		}
	}
	switch node["type"] {
	case "array":
		return "array"
	case "object":
		return "object"
	case "string":
		return "string"
	case "integer", "number":
		return "scalar"
	}
	if _, ok := node["properties"]; ok {
		return "object"
	}
	if _, ok := node["enum"]; ok {
		return "string"
	}
	return ""
}

func representativeValue(kind string) any {
	switch kind {
	case "provnumber":
		return map[string]any{"value": float64(1)}
	case "array":
		return []any{float64(1)}
	case "object":
		return map[string]any{"placeholder": float64(1)}
	case "string":
		return "x"
	case "scalar":
		return float64(1)
	}
	return nil
}

func setDataPath(rec map[string]any, comps []string, val any) {
	node := rec
	for _, c := range comps[:len(comps)-1] {
		next, ok := node[c].(map[string]any)
		if !ok {
			next = map[string]any{}
			node[c] = next
		}
		node = next
	}
	node[comps[len(comps)-1]] = val
}

// predicateBackedPaths are rows whose path is a clean JSON Pointer but whose
// present-closure is a structural predicate (not a num/str/obj/arr/scalarNum shape
// constructor), so a generic schema-correct value would not satisfy it. They are
// covered by the behavioral tests instead.
var predicateBackedPaths = map[string]bool{
	"/operating_point": true, // hasOperatingPoint: needs a recognized qualifier
	"/uncertainty":     true, // hasUncertainty: needs coverage_factor_k + an expanded_*
}

// shapeTestable reports whether a rubric path is a single resolvable JSON Pointer
// backed by a shape constructor (not a prose label like "safety listing ..." or
// the "input_voltage_v (or ...)" either/or row, and not a structural-predicate row).
func shapeTestable(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.ContainsAny(path, " (") && !predicateBackedPaths[path]
}

// TestRubricPathsResolve is the shape guard: for every rubric and observation row
// whose path is a single JSON Pointer, the path resolves to a real field in
// ulc.schema.json and the row's present-closure accepts a schema-correct value for
// that field's shape. This catches a closure/shape mismatch (num applied to an
// object, str to an array), the class of bug that silently makes a gate never match.
func TestRubricPathsResolve(t *testing.T) {
	ulc := loadJSONSchema(t, "ulc.schema.json")
	tax := loadJSONSchema(t, "taxonomy.schema.json")
	ulcDefs, _ := mapOf(ulc["$defs"])
	taxDefs, _ := mapOf(tax["$defs"])

	for _, ru := range rubric {
		if !shapeTestable(ru.path) {
			continue
		}
		comps := strings.Split(strings.TrimPrefix(ru.path, "/"), "/")
		leaf, ok := resolveDataPath(ulc, comps, ulcDefs)
		if !ok {
			t.Errorf("rubric path %q (level %s) does not resolve in ulc.schema.json", ru.path, ru.level)
			continue
		}
		kind := classifyLeaf(leaf, ulcDefs, taxDefs)
		if kind == "" {
			t.Errorf("rubric path %q: could not classify the schema shape of the leaf", ru.path)
			continue
		}
		rec := map[string]any{}
		setDataPath(rec, comps, representativeValue(kind))
		if !ru.present(rec) {
			t.Errorf("rubric path %q: schema shape is %q, but the row's present-closure rejects a correct %s value (closure/shape mismatch)", ru.path, kind, kind)
		}
	}
}

// rubricTaxonomies is the set of taxonomy enum $defs the rubric maps (via each
// row's taxonomy field).
func rubricTaxonomies() map[string]bool {
	out := map[string]bool{}
	for _, ru := range rubric {
		if ru.taxonomy != "" {
			out[ru.taxonomy] = true
		}
	}
	return out
}

// descriptiveAllowlist is the set of taxonomy enums referenced from ulc.schema.json
// (outside /index) that are intentionally NOT graded: provenance, status, value-type,
// file-format, and domain sub-structure enums. Enumerated explicitly so the
// exhaustiveness guard is green on day one and any newly-referenced enum forces a
// conscious decision (grade it, or allowlist it as descriptive).
var descriptiveAllowlist = map[string]bool{
	// Record + provenance + file plumbing.
	"RecordStatus":        true,
	"ProvenanceSource":    true,
	"ProvenanceMethod":    true,
	"RegulatoryValueType": true,
	"ComparisonOperator":  true,
	"SourceFileType":      true,
	"FileGenerationType":  true,
	"PhotometryFormat":    true,
	// Attestation sub-structure (the AttestationProgram itself is graded; these qualify it).
	"AttestationStatus":           true,
	"AttestationVerificationType": true,
	// Lab + test-condition depth (observations, not gated on the specific enum).
	"LaboratoryCertification":       true,
	"LaboratoryAccreditationScheme": true,
	"NonstandardConditionFlag":      true,
	"TemperatureAxis":               true,
	"AmbientCleanliness":            true,
	// Descriptive labels and projections.
	"DimmingCurve":               true,
	"BeamFamily":                 true,
	"PhotometryMethod":           true,
	"NegativeIntensityHandling":  true,
	"GoniometerType":             true,
	"StabilizationMethod":        true,
	"TemperatureMonitoringPoint": true,
	"DutOperatingMode":           true,
	// Flicker block (observation) sub-structure.
	"FlickerMetric":                          true,
	"FlickerDimmingType":                     true,
	"FlickerRiskLevel":                       true,
	"FlickerSamplingClass":                   true,
	"FlickerTestChamberType":                 true,
	"FlickerWaveformFileFormat":              true,
	"FlickerPhotodetectorSpectralCorrection": true,
	// Alpha-opic / circadian block (observation).
	"AlphaOpicChannel": true,
	// Chromaticity-shift projection block (observation).
	"ChromaticityShiftMetric":    true,
	"ChromaticityShiftMode":      true,
	"ChromaticityShiftThreshold": true,
	"TM35Edition":                true,
	// Lumen-maintenance block sub-structure (the block presence is what is gated).
	"LumenMaintenanceDeclarationFramework": true,
	"LumenMaintenanceProjectionMethod":     true,
	"FluxMaintenanceQuantity":              true,
	"FluxMaintenanceThreshold":             true,
	"ProjectionBasis":                      true,
	"ProjectionReliability":                true,
	"TM21InterpolationType":                true,
	"TestedProductType":                    true,
	"ThermalControlMethod":                 true,
	// Sustainability block (observation).
	"SustainabilityDeclarationType": true,
	"IngredientRedListStatus":       true,
}

// TestRubricExhaustiveness is the drift guard: every taxonomy enum referenced from
// ulc.schema.json (outside the generated /index subtree) is either mapped by the
// rubric (some row's taxonomy field) or listed in descriptiveAllowlist. A new
// taxonomy-typed field added to the schema without a rubric or allowlist decision
// fails this test.
func TestRubricExhaustiveness(t *testing.T) {
	ulc := loadJSONSchema(t, "ulc.schema.json")
	tax := loadJSONSchema(t, "taxonomy.schema.json")
	ulcDefs, _ := mapOf(ulc["$defs"])
	taxDefs, _ := mapOf(tax["$defs"])

	referenced := map[string]bool{}
	visited := map[string]bool{}
	var walk func(node any)
	walk = func(node any) {
		switch n := node.(type) {
		case map[string]any:
			if ref, ok := n["$ref"].(string); ok {
				name, isTax, ok := refTarget(ref)
				if ok && isTax {
					referenced[name] = true
					return
				}
				if ok && !isTax {
					if !visited[name] {
						visited[name] = true
						walk(ulcDefs[name])
					}
					return
				}
			}
			for k, v := range n {
				walk(v)
				_ = k
			}
		case []any:
			for _, item := range n {
				walk(item)
			}
		}
	}
	// Walk from the root properties, skipping the generated /index subtree.
	rootProps, _ := mapOf(ulc["properties"])
	for name, sub := range rootProps {
		if name == "index" {
			continue
		}
		walk(sub)
	}

	covered := rubricTaxonomies()
	uncovered := []string{}
	for name := range referenced {
		// Only enums matter; non-enum taxonomy defs (if any) are not grading targets.
		def, ok := mapOf(taxDefs[name])
		if !ok {
			continue
		}
		if _, isEnum := def["enum"]; !isEnum {
			continue
		}
		if covered[name] || descriptiveAllowlist[name] {
			continue
		}
		uncovered = append(uncovered, name)
	}
	if len(uncovered) > 0 {
		sort.Strings(uncovered)
		t.Errorf("taxonomy enums referenced from ulc.schema.json but neither graded by the rubric nor in descriptiveAllowlist: %v\n"+
			"Decide for each: add a rubric row (with its taxonomy field) or add it to descriptiveAllowlist.", uncovered)
	}
}

// loadEnumMembers loads a taxonomy enum's members as a set, failing if the named
// $def is missing or is not an enum.
func loadEnumMembers(t *testing.T, taxDefs map[string]any, name string) map[string]bool {
	t.Helper()
	def, ok := mapOf(taxDefs[name])
	if !ok {
		t.Fatalf("taxonomy.schema.json has no $def %q", name)
	}
	raw, ok := def["enum"].([]any)
	if !ok {
		t.Fatalf("taxonomy $def %q is not an enum", name)
	}
	set := map[string]bool{}
	for _, v := range raw {
		if s, ok := v.(string); ok {
			set[s] = true
		}
	}
	return set
}

// TestPredicateSetsAreRealEnumMembers guards the hand-maintained accept-sets inside
// the applicability predicates (which categories are outdoor-site or directional,
// which protocols require analog dimming detail, which attestations satisfy the
// region-conditional safety gate): every key must be a real member of its backing
// taxonomy enum. TestRubricExhaustiveness checks a rubric row's taxonomy field against
// the schema, but these internal subsets are invisible to it, so a typo or a renamed
// token would otherwise pass the suite and silently waive or misapply a gate.
func TestPredicateSetsAreRealEnumMembers(t *testing.T) {
	tax := loadJSONSchema(t, "taxonomy.schema.json")
	taxDefs, _ := mapOf(tax["$defs"])

	cases := []struct {
		name string
		set  map[string]bool
		enum string
	}{
		{"directionalCategories", directionalCategories, "PrimaryCategory"},
		{"outdoorSiteCategories", outdoorSiteCategories, "PrimaryCategory"},
		{"linearCategories", linearCategories, "PrimaryCategory"},
		{"analogPhaseDimming", analogPhaseDimming, "DimmingProtocol"},
		{"naSafetyListings", naSafetyListings, "AttestationProgram"},
		{"anySafetyListings", anySafetyListings, "AttestationProgram"},
	}
	for _, c := range cases {
		members := loadEnumMembers(t, taxDefs, c.enum)
		for token := range c.set {
			if !members[token] {
				t.Errorf("predicate set %s contains %q, not a member of the %s enum in taxonomy.schema.json", c.name, token, c.enum)
			}
		}
	}
}
