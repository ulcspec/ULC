package completeness

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
// root schema to the leaf property schema. A component ending in "[]" is an array
// hop: resolve the array property, then descend into its items schema (a single hop;
// every array path in the rubric is one level deep).
func resolveDataPath(root map[string]any, comps []string, ulcDefs map[string]any) (map[string]any, bool) {
	node := root
	for _, c := range comps {
		key := strings.TrimSuffix(c, "[]")
		child, ok := childSchema(node, key, ulcDefs)
		if !ok {
			return nil, false
		}
		if strings.HasSuffix(c, "[]") {
			child = derefObject(child, ulcDefs)
			items, ok := mapOf(child["items"])
			if !ok {
				return nil, false
			}
			child = items
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
		// Match DualUnitLength specifically (not a DualUnit* prefix) so a future shapeTestable
		// DualUnitArea / DualUnitTemperature path is never mis-shaped as a DualUnitLength.
		if !isTax && name == "DualUnitLength" {
			return "dualunit"
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
	case "dualunit":
		// DualUnitLength requires mm + in; representativeValue takes only the kind string
		// and cannot read the def, so it hardcodes the two required unit members. The mm
		// leaf satisfies the scalarNum(..., "mm") present-closures on the DualUnitLength rows.
		return map[string]any{"mm": float64(1), "in": float64(1)}
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

// setDataPath builds the nested fixture down to the leaf. A component ending in "[]"
// becomes a one-element array whose single entry is the descending map (a single
// array hop, mirroring resolveDataPath). The leaf itself is never an array hop.
func setDataPath(rec map[string]any, comps []string, val any) {
	node := rec
	for _, c := range comps[:len(comps)-1] {
		if strings.HasSuffix(c, "[]") {
			key := strings.TrimSuffix(c, "[]")
			var elem map[string]any
			if arr, ok := node[key].([]any); ok && len(arr) > 0 {
				elem, _ = arr[0].(map[string]any)
			}
			if elem == nil {
				elem = map[string]any{}
				node[key] = []any{elem}
			}
			node = elem
			continue
		}
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
	"/product_family/cutsheet":           true, // hasCutsheet: needs an attached cutsheet with a content hash
	"/emergency/photometry_reference":    true, // hasEmergencyPhotometryReference: FileReference needing a content hash (like cutsheet)
	"/operating_point":                   true, // hasOperatingPoint: needs a recognized qualifier
	"/uncertainty":                       true, // hasUncertainty: needs coverage_factor_k + an expanded_*
	"/corrections_applied":               true, // hasCorrectionsApplied: needs a recognized correction leaf
	"/photometry/per_length_normalized":  true, // hasPerLengthNormalized: needs a per-length rate, not just reference_length
	"/outdoor_classification/bug_rating": true, // hasBugRating: needs b + u + g
	"/electrical/dimming_range_percent":  true, // hasDimmingRange: needs min + max
	// Enrichment/observation rows whose closures require recognized real content (a
	// placeholder object reads as absent), so the "... not disclosed" nudge fires on
	// hollow blocks. Behavioral coverage: TestTightenedObservationGatesRejectHollow.
	"/product_family/shared_mechanical/ambient_operating_range": true, // hasAmbientOperatingRange: needs a numeric c/f bound
	"/thermal_derating":                       true, // hasThermalDerating: needs thermal_control_method or curves
	"/flicker_measurements":                   true, // hasFlickerMeasurements: needs metrics or a qualifier enum
	"/alpha_opic_metrics":                     true, // hasAlphaOpicMetrics: needs melanopic_der / per_channel / observer
	"/chromaticity_shift_projection":          true, // hasChromaticityShiftProjection: needs projected_hours or an enum
	"/sustainability_declaration":             true, // hasSustainabilityDeclaration: needs a recognized declaration field
	"/product_family/physical_dimensions/epa": true, // hasEPA: needs a numeric m2/ft2 area
	// Block-level enrichment row whose closure requires a populated grid (a placeholder
	// object reads as absent), so the nudge fires on a hollow table.
	"/lumen_maintenance_luminaire/cie_97_lmf_table": true, // hasCie97LmfTable: needs a non-empty LMF/LLMF grid
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

// TestReferenceIlluminantTypePathResolves guards the wired-not-nudged field: because
// reference_illuminant_type carries no rubric row and is allowlisted, exhaustiveness
// passes whether or not the schema field exists, so a forgotten or misspelled wire
// would be undetectable. This asserts the path resolves to a real schema field.
func TestReferenceIlluminantTypePathResolves(t *testing.T) {
	ulc := loadJSONSchema(t, "ulc.schema.json")
	ulcDefs, _ := mapOf(ulc["$defs"])
	comps := []string{"colorimetry", "tm_30", "reference_illuminant_type"}
	if _, ok := resolveDataPath(ulc, comps, ulcDefs); !ok {
		t.Error("colorimetry.tm_30.reference_illuminant_type does not resolve in ulc.schema.json (a forgotten wire)")
	}
}

// TestEmergencyPhotometryReferencePathResolves guards a predicate-backed field: because
// /emergency/photometry_reference is in predicateBackedPaths, TestRubricPathsResolve skips
// its schema-resolution check, so a renamed or removed schema field would silently make
// hasEmergencyPhotometryReference always-false and fire the "not attached" nudge on every
// dual-mode record, undetected. Assert the path resolves. Mirrors the reference-illuminant guard.
func TestEmergencyPhotometryReferencePathResolves(t *testing.T) {
	ulc := loadJSONSchema(t, "ulc.schema.json")
	ulcDefs, _ := mapOf(ulc["$defs"])
	comps := []string{"emergency", "photometry_reference"}
	if _, ok := resolveDataPath(ulc, comps, ulcDefs); !ok {
		t.Error("emergency.photometry_reference does not resolve in ulc.schema.json (a forgotten wire)")
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
// v0.9.0 pruned this list: the enrichment roadmap (Phase C) wired the flicker,
// chromaticity-shift, thermal, test-condition, operating-point, lumen-maintenance, and
// descriptive-label enums as rubric rows, so they moved from here to rubricTaxonomies().
// The allowlist-disjointness guard (TestAllowlistDisjointFromRubric) fires if an enum
// is ever both rubric-covered and allowlisted, keeping this registry a conscious record
// of what is deliberately NOT on a roadmap.
var descriptiveAllowlist = map[string]bool{
	// Record + provenance + file plumbing.
	"RecordStatus":        true,
	"ProvenanceSource":    true,
	"ProvenanceMethod":    true,
	"RegulatoryValueType": true,
	"ComparisonOperator":  true,
	"SourceFileType":      true,
	// ReferenceIlluminantType is wired as an optional field (colorimetry.tm_30) but
	// deliberately NOT nudged: it is derivable by definition from the test-source CCT,
	// so a roadmap entry would push authors to hand-enter a derivable constant. Its path
	// resolving is guarded by TestReferenceIlluminantTypePathResolves.
	"ReferenceIlluminantType": true,
	// Attestation sub-structure (the AttestationProgram itself is graded; these qualify it).
	"AttestationStatus":           true,
	"AttestationVerificationType": true,
	// Lab depth (the measurement_regime gate is graded, not the specific enum).
	"LaboratoryCertification":       true,
	"LaboratoryAccreditationScheme": true,
	"GoniometerType":                true,
	// Required members of their array items: surfaced by the reclassified parent
	// enrichment row (/alpha_opic_metrics, /flicker_measurements), so they can never be
	// applicable-and-absent on a valid record and get no own row. This is their one home.
	"AlphaOpicChannel": true,
	"FlickerMetric":    true,
	// Sustainability block: the /sustainability_declaration row is a non-nudged
	// observation (empty taxonomy token), so these sub-enums stay descriptive.
	"SustainabilityDeclarationType": true,
	"IngredientRedListStatus":       true,

	// --- v0.10.0 emergency & exit-sign class ---
	// The 9 exit-sign / emergency enums that back rubric rows (7 gating in Phase B, plus
	// ExitSignIlluminationTechnology and RatedViewingDistance on Phase C enrichment rows)
	// are rubric-covered and NOT allowlisted. EmergencyRole stays permanently: it is
	// schema-required within the emergency block, so it can never be applicable-and-absent
	// on a valid record and gets no rubric row (§2.4). Its two dual-mode tokens are read by
	// the emergency-mode enrichment predicates (emergencyModeRoles) but carried as no row's
	// taxonomy, so the disjointness guard stays clean.
	"EmergencyRole": true,
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
		{"nonControllableDrivers", nonControllableDrivers, "DimmingProtocol"},
		{"photometrySourceFileTypes", photometrySourceFileTypes, "SourceFileType"},
		{"naSafetyListings", naSafetyListings, "AttestationProgram"},
		{"anySafetyListings", anySafetyListings, "AttestationProgram"},
		{"naRegions", naRegions, "TechnicalRegion"},
		{"poleMountedTypes", poleMountedTypes, "MountingType"},
		{"dedicatedCategories", dedicatedCategories, "PrimaryCategory"},
		{"emergencyModeRoles", emergencyModeRoles, "EmergencyRole"},
		// v0.10.0: the rubric's signMode rows, hasIntegralBattery, and testReportBacked compare
		// against bare string literals rather than a shared set, so they are invisible to the
		// row-level exhaustiveness guard (ProvenanceSource is also allowlisted). Pin the exact
		// tokens the grader relies on against their enums: a taxonomy rename would otherwise
		// silently make the comparison always-false and waive the mode-conditional sign gates,
		// the battery trio, or the entire sign full tier, with no test going red.
		{"signMode tokens", map[string]bool{
			"internally_illuminated": true, "externally_illuminated": true,
			"photoluminescent": true, "self_luminous": true,
		}, "ExitSignIlluminationMode"},
		{"hasIntegralBattery token", map[string]bool{"integral_battery": true}, "EmergencyPowerSource"},
		{"testReportBacked source token", map[string]bool{"test_report": true}, "ProvenanceSource"},
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

// TestDimmingRangeRejectsInverted proves hasDimmingRange requires min <= max: a
// schema-valid but nonsensical inverted range (the schema bounds min and max to 0-100
// independently but cannot relate them) must not satisfy the standard dimming gate.
func TestDimmingRangeRejectsInverted(t *testing.T) {
	mk := func(lo, hi float64) map[string]any {
		return map[string]any{"electrical": map[string]any{"dimming_range_percent": map[string]any{"min": lo, "max": hi}}}
	}
	if hasDimmingRange(mk(100, 3)) {
		t.Error("hasDimmingRange accepted an inverted range (min=100, max=3) (over-lenient)")
	}
	if !hasDimmingRange(mk(3, 100)) {
		t.Error("hasDimmingRange rejected a valid range (min=3, max=100)")
	}
	if !hasDimmingRange(mk(50, 50)) {
		t.Error("hasDimmingRange rejected a degenerate-but-valid range (min=max=50)")
	}
}

// TestTightenedObjectGatesRejectEmptyAndUnrecognized proves the over-leniency class
// is closed for the object gates: each closure returns FALSE for an empty object and
// for an object holding only an unrecognized key, and TRUE only once a single
// recognized content field of the correct type is present. wrap builds a full record
// by placing the gated-object fragment at the exact path each closure reads. The old
// obj(...) / hasMap closures returned TRUE for both the empty and the
// unrecognized-only object (any non-empty map satisfied hasMap), so every FALSE
// assertion below would have been RED before this change.
func TestTightenedObjectGatesRejectEmptyAndUnrecognized(t *testing.T) {
	cases := []struct {
		name    string
		wrap    func(fragment map[string]any) map[string]any // place fragment at the gated path
		closure func(map[string]any) bool
		good    map[string]any // one recognized content field of correct type
	}{
		{
			"corrections_applied",
			func(f map[string]any) map[string]any { return map[string]any{"corrections_applied": f} },
			hasCorrectionsApplied,
			map[string]any{"self_absorption_corrected": true},
		},
		{
			"per_length_normalized",
			func(f map[string]any) map[string]any {
				return map[string]any{"photometry": map[string]any{"per_length_normalized": f}}
			},
			hasPerLengthNormalized,
			map[string]any{"lumens_per_meter": map[string]any{"value": float64(800)}},
		},
		{
			"bug_rating",
			func(f map[string]any) map[string]any {
				return map[string]any{"outdoor_classification": map[string]any{"bug_rating": f}}
			},
			hasBugRating,
			map[string]any{"b": float64(1), "u": float64(0), "g": float64(1)},
		},
		{
			"dimming_range_percent",
			func(f map[string]any) map[string]any {
				return map[string]any{"electrical": map[string]any{"dimming_range_percent": f}}
			},
			hasDimmingRange,
			map[string]any{"min": float64(1), "max": float64(100)},
		},
		{
			"lumen_maintenance_luminaire",
			func(f map[string]any) map[string]any { return map[string]any{"lumen_maintenance_luminaire": f} },
			hasLumenMaintenance,
			map[string]any{"manufacturer_rated_claim": map[string]any{"claim_type": "L70"}},
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if c.closure(c.wrap(map[string]any{})) {
				t.Errorf("%s: closure accepted an empty object (over-lenient)", c.name)
			}
			if c.closure(c.wrap(map[string]any{"zzz_unrecognized": float64(1)})) {
				t.Errorf("%s: closure accepted an object with only an unrecognized key (over-lenient)", c.name)
			}
			if !c.closure(c.wrap(c.good)) {
				t.Errorf("%s: closure rejected an object with a recognized content field: %+v", c.name, c.good)
			}
		})
	}
}

// TestTightenedLumenMaintenancePackageRejectsEmptyEntry proves the package arm of
// hasLumenMaintenance is closed: a non-empty array of empty entries (the [{}] case
// the old len>0 check accepted, since LumenMaintenancePackageEntry has no required
// fields) is rejected, an entry whose recognized keys carry the wrong leaf type (a
// package_identifier:"" empty string, a value-less test_hours:{} object where a
// ProvenancedNumber is meant) is rejected, while an entry carrying a recognized field
// of the correct type is accepted. The old key-presence check accepted all three junk
// forms, so every FALSE assertion below would have been RED before this change.
func TestTightenedLumenMaintenancePackageRejectsEmptyEntry(t *testing.T) {
	reject := []struct {
		name  string
		entry map[string]any
	}{
		{"empty entry", map[string]any{}},
		{"unrecognized key only", map[string]any{"zzz_unrecognized": float64(1)}},
		{"empty-string package_identifier", map[string]any{"package_identifier": ""}},
		{"value-less test_hours object", map[string]any{"test_hours": map[string]any{}}},
		{"value-less drive_current_ma object", map[string]any{"drive_current_ma": map[string]any{"junk": float64(1)}}},
		{"empty-string enum field", map[string]any{"tm_21_interpolation_type": ""}},
	}
	for _, c := range reject {
		c := c
		t.Run("reject/"+c.name, func(t *testing.T) {
			rec := map[string]any{"lumen_maintenance_package": []any{c.entry}}
			if hasLumenMaintenance(rec) {
				t.Errorf("lumen_maintenance_package: closure accepted a hollow entry (%s) (over-lenient)", c.name)
			}
		})
	}
	accept := []struct {
		name  string
		entry map[string]any
	}{
		{"ProvenancedNumber projection hours", map[string]any{"tm_21_projection_hours": map[string]any{"value": float64(60000)}}},
		{"enum field populated", map[string]any{"flux_maintenance_threshold": "L70"}},
		{"package_identifier string", map[string]any{"package_identifier": "PKG-A"}},
	}
	for _, c := range accept {
		c := c
		t.Run("accept/"+c.name, func(t *testing.T) {
			rec := map[string]any{"lumen_maintenance_package": []any{c.entry}}
			if !hasLumenMaintenance(rec) {
				t.Errorf("lumen_maintenance_package: closure rejected an entry with a recognized field (%s)", c.name)
			}
		})
	}
}

// TestTightenedLumenMaintenanceLuminaireRejectsHollowSubBlock proves the luminaire arm
// of hasLumenMaintenance is closed at the SUB-BLOCK level: a manufacturer_rated_claim,
// tm_28, or cie_97_lmf_table that is present as a non-empty map but carries only
// unrecognized keys (or recognized keys of the wrong leaf type) does not satisfy the
// gate. The prior hasMap-based check accepted any non-empty sub-block, so a
// manufacturer_rated_claim:{<only-unrecognized-key>} passed: every FALSE assertion
// below would have been RED before this change. The accept cases pin that a real
// claim_type enum, a real claimed_hours number, a tm_28 projection, and a populated
// CIE 97 grid each still lift the gate.
func TestTightenedLumenMaintenanceLuminaireRejectsHollowSubBlock(t *testing.T) {
	wrap := func(sub map[string]any) map[string]any {
		return map[string]any{"lumen_maintenance_luminaire": sub}
	}
	reject := []struct {
		name string
		sub  map[string]any
	}{
		{"manufacturer_rated_claim unrecognized key only",
			map[string]any{"manufacturer_rated_claim": map[string]any{"zzz_unrecognized": float64(1)}}},
		{"manufacturer_rated_claim empty-string claim_type",
			map[string]any{"manufacturer_rated_claim": map[string]any{"claim_type": ""}}},
		{"manufacturer_rated_claim value-less claimed_hours",
			map[string]any{"manufacturer_rated_claim": map[string]any{"claimed_hours": map[string]any{"junk": float64(1)}}}},
		{"tm_28 unrecognized key only",
			map[string]any{"tm_28": map[string]any{"zzz_unrecognized": float64(1)}}},
		{"tm_28 value-less projection hours",
			map[string]any{"tm_28": map[string]any{"tm_28_projection_hours": map[string]any{}}}},
		{"cie_97_lmf_table empty grids",
			map[string]any{"cie_97_lmf_table": map[string]any{"lmf_by_cleanliness_and_interval": []any{}, "llmf_by_hours": []any{}}}},
		{"cie_97_lmf_table unrecognized key only",
			map[string]any{"cie_97_lmf_table": map[string]any{"zzz_unrecognized": float64(1)}}},
	}
	for _, c := range reject {
		c := c
		t.Run("reject/"+c.name, func(t *testing.T) {
			if hasLumenMaintenance(wrap(c.sub)) {
				t.Errorf("lumen_maintenance_luminaire: closure accepted a hollow sub-block (%s) (over-lenient)", c.name)
			}
		})
	}
	accept := []struct {
		name string
		sub  map[string]any
	}{
		{"manufacturer_rated_claim claim_type enum",
			map[string]any{"manufacturer_rated_claim": map[string]any{"claim_type": "L70"}}},
		{"manufacturer_rated_claim numeric claimed_hours",
			map[string]any{"manufacturer_rated_claim": map[string]any{"claimed_hours": map[string]any{"value": float64(50000)}}}},
		{"tm_28 projection hours",
			map[string]any{"tm_28": map[string]any{"tm_28_projection_hours": map[string]any{"value": float64(60000)}}}},
		{"cie_97_lmf_table populated llmf grid",
			map[string]any{"cie_97_lmf_table": map[string]any{"llmf_by_hours": []any{map[string]any{"hours": float64(6000), "llmf": float64(0.95)}}}}},
	}
	for _, c := range accept {
		c := c
		t.Run("accept/"+c.name, func(t *testing.T) {
			if !hasLumenMaintenance(wrap(c.sub)) {
				t.Errorf("lumen_maintenance_luminaire: closure rejected a populated sub-block (%s)", c.name)
			}
		})
	}
}

// TestProvenancedNumberGatesRejectBareScalar proves the ProvenancedNumber-leaf gates
// are closed: a bare scalar and a value-less {junk:1} object both read as absent, and
// only a {value:N, value_type:...} ProvenancedNumber satisfies the gate. The old
// hasUncertainty / hasOperatingPoint checked key-presence (or len>0 on the map), so a
// bare scalar at an expanded_* field, or a {junk:1} qualifier map, passed; every FALSE
// assertion below would have been RED before this change.
func TestProvenancedNumberGatesRejectBareScalar(t *testing.T) {
	// hasUncertainty: coverage_factor_k must be numeric AND an expanded_* must be a
	// ProvenancedNumber.
	t.Run("uncertainty_expanded_bare_scalar", func(t *testing.T) {
		bare := map[string]any{"uncertainty": map[string]any{
			"coverage_factor_k":                       float64(2),
			"expanded_uncertainty_total_flux_percent": float64(5), // bare scalar, not a ProvenancedNumber
		}}
		if hasUncertainty(bare) {
			t.Error("hasUncertainty accepted a bare scalar at an expanded_* field (over-lenient)")
		}
		junk := map[string]any{"uncertainty": map[string]any{
			"coverage_factor_k":                       float64(2),
			"expanded_uncertainty_total_flux_percent": map[string]any{"junk": float64(1)}, // value-less map
		}}
		if hasUncertainty(junk) {
			t.Error("hasUncertainty accepted a value-less map at an expanded_* field (over-lenient)")
		}
		good := map[string]any{"uncertainty": map[string]any{
			"coverage_factor_k":                       float64(2),
			"expanded_uncertainty_total_flux_percent": map[string]any{"value": float64(5), "value_type": "measured"},
		}}
		if !hasUncertainty(good) {
			t.Error("hasUncertainty rejected a real coverage_factor_k + ProvenancedNumber expanded value")
		}
		// coverage_factor_k must itself be numeric, not a bare-but-present junk value.
		nonNumK := map[string]any{"uncertainty": map[string]any{
			"coverage_factor_k":                       "two",
			"expanded_uncertainty_total_flux_percent": map[string]any{"value": float64(5)},
		}}
		if hasUncertainty(nonNumK) {
			t.Error("hasUncertainty accepted a non-numeric coverage_factor_k (over-lenient)")
		}
	})

	// hasOperatingPoint: a numeric qualifier must be a ProvenancedNumber; a {junk:1}
	// qualifier map reads as absent.
	t.Run("operating_point_qualifier_bare_scalar", func(t *testing.T) {
		bare := map[string]any{"operating_point": map[string]any{
			"drive_current_ma": float64(350), // bare scalar, not a ProvenancedNumber
		}}
		if hasOperatingPoint(bare) {
			t.Error("hasOperatingPoint accepted a bare scalar drive_current_ma (over-lenient)")
		}
		junk := map[string]any{"operating_point": map[string]any{
			"drive_current_ma": map[string]any{"junk": float64(1)}, // value-less map
		}}
		if hasOperatingPoint(junk) {
			t.Error("hasOperatingPoint accepted a value-less qualifier map (over-lenient)")
		}
		good := map[string]any{"operating_point": map[string]any{
			"drive_current_ma": map[string]any{"value": float64(350), "value_type": "measured"},
		}}
		if !hasOperatingPoint(good) {
			t.Error("hasOperatingPoint rejected a real ProvenancedNumber qualifier")
		}
	})
}

// TestCapstoneJunkObjectsCapAtIncomplete is the class-closed invariant: a record that
// satisfies the photometric anchors but populates every gated object path with only an
// unrecognized {"zzz":1} key reaches at most LevelIncomplete. Under the old presence
// closures these junk objects satisfied their gates, so such a record could have
// climbed past incomplete; this asserts the whole class of meaningless-but-non-empty
// content no longer lifts a record.
func TestCapstoneJunkObjectsCapAtIncomplete(t *testing.T) {
	junk := map[string]any{"zzz": float64(1)}
	rec := map[string]any{
		// Photometric anchors present; the record still floors at incomplete because
		// the gated object paths carry only junk.
		"photometry":     map[string]any{"total_luminous_flux_lm": map[string]any{"value": float64(1200)}},
		"electrical":     map[string]any{"input_power_w": map[string]any{"value": float64(10)}},
		"product_family": map[string]any{"primary_category": "panel_troffer"},
		// Every gated object path carries only an unrecognized key.
		"corrections_applied":         junk,
		"uncertainty":                 junk,
		"operating_point":             junk,
		"outdoor_classification":      map[string]any{"bug_rating": junk},
		"lumen_maintenance_luminaire": junk,
		"lumen_maintenance_package":   []any{map[string]any{"zzz": float64(1)}},
	}
	rec["photometry"].(map[string]any)["per_length_normalized"] = junk
	rec["electrical"].(map[string]any)["dimming_range_percent"] = junk
	if got := AchievedLevel(rec); got > LevelIncomplete {
		t.Errorf("record with only junk in every gated object reached %s, want at most incomplete", got)
	}
}

// TestTightenedObservationGatesRejectHollow proves the over-leniency class is closed
// for the object-shaped non-gating rows (now mostly enrichment closures, plus the
// sustainability observation): each closure returns FALSE for a map
// holding only an unrecognized key and for a map whose recognized key carries the wrong
// leaf type (a bare scalar where a ProvenancedNumber is meant, a number where an enum
// string is meant, a DualUnit block with only value_type metadata and no numeric leaf),
// and TRUE once a single recognized field of the correct type is present. wrap places
// the fragment at the exact path the closure reads. The old obj(...) / hasMap closures
// returned TRUE for any non-empty map, so every FALSE assertion below would have been
// RED before this change, wrongly suppressing the "... not disclosed" nudge on a hollow
// block.
func TestTightenedObservationGatesRejectHollow(t *testing.T) {
	cases := []struct {
		name    string
		wrap    func(fragment map[string]any) map[string]any // place fragment at the observed path
		closure func(map[string]any) bool
		reject  []map[string]any // wrong-leaf-type forms a non-empty map could take
		good    map[string]any   // one recognized content field of the correct type
	}{
		{
			"ambient_operating_range",
			func(f map[string]any) map[string]any {
				return map[string]any{"product_family": map[string]any{"shared_mechanical": map[string]any{"ambient_operating_range": f}}}
			},
			hasAmbientOperatingRange,
			[]map[string]any{
				{"min": map[string]any{"value_type": "rated"}}, // DualUnitTemperature with no numeric c/f
				{"max": map[string]any{"c": "cold"}},           // c present but not a number
			},
			map[string]any{"min": map[string]any{"c": float64(-30), "f": float64(-22)}},
		},
		{
			"thermal_derating",
			func(f map[string]any) map[string]any { return map[string]any{"thermal_derating": f} },
			hasThermalDerating,
			[]map[string]any{
				{"thermal_control_method": float64(1)},      // enum field carrying a number
				{"curves": map[string]any{"x": float64(1)}}, // curves as an object, not an array
			},
			map[string]any{"thermal_control_method": "active_fan"},
		},
		{
			"flicker_measurements",
			func(f map[string]any) map[string]any { return map[string]any{"flicker_measurements": f} },
			hasFlickerMeasurements,
			[]map[string]any{
				{"risk_level": float64(1)}, // enum field carrying a number
				{"metrics": float64(1)},    // metrics as a scalar, not an array
				{"metrics": []any{}},       // empty metrics array
			},
			map[string]any{"risk_level": "no_risk"},
		},
		{
			"alpha_opic_metrics",
			func(f map[string]any) map[string]any { return map[string]any{"alpha_opic_metrics": f} },
			hasAlphaOpicMetrics,
			[]map[string]any{
				{"melanopic_der": float64(1)},                      // bare scalar, not a ProvenancedNumber
				{"melanopic_der": map[string]any{"x": float64(1)}}, // value-less map
				{"per_channel": map[string]any{"x": float64(1)}},   // per_channel as an object, not an array
			},
			map[string]any{"melanopic_der": map[string]any{"value": float64(0.9)}},
		},
		{
			"chromaticity_shift_projection",
			func(f map[string]any) map[string]any { return map[string]any{"chromaticity_shift_projection": f} },
			hasChromaticityShiftProjection,
			[]map[string]any{
				{"projected_hours": float64(1)}, // bare scalar, not a ProvenancedNumber
				{"shift_metric": float64(1)},    // enum field carrying a number
			},
			map[string]any{"projected_hours": map[string]any{"value": float64(50000)}},
		},
		{
			"sustainability_declaration",
			func(f map[string]any) map[string]any { return map[string]any{"sustainability_declaration": f} },
			hasSustainabilityDeclaration,
			[]map[string]any{
				{"declaration_type": float64(1)},      // enum field carrying a number
				{"life_expectancy_years": "ten"},      // numeric field carrying a string
				{"ingredient_list": map[string]any{}}, // array field carrying an object
				{"end_of_life_options": []any{}},      // empty array
			},
			map[string]any{"declaration_type": "declare_label"},
		},
		{
			"epa",
			func(f map[string]any) map[string]any {
				return map[string]any{"product_family": map[string]any{"physical_dimensions": map[string]any{"epa": f}}}
			},
			hasEPA,
			[]map[string]any{
				{"value_type": "rated"}, // only DualUnitArea metadata, no numeric area
				{"m2": "0.5"},           // m2 present but not a number
			},
			map[string]any{"m2": float64(0.5), "ft2": float64(5.4)},
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if c.closure(c.wrap(map[string]any{"zzz_unrecognized": float64(1)})) {
				t.Errorf("%s: closure accepted a map with only an unrecognized key (over-lenient)", c.name)
			}
			for i, bad := range c.reject {
				if c.closure(c.wrap(bad)) {
					t.Errorf("%s: closure accepted a wrong-leaf-type form #%d %+v (over-lenient)", c.name, i, bad)
				}
			}
			if !c.closure(c.wrap(c.good)) {
				t.Errorf("%s: closure rejected a map with a recognized content field: %+v", c.name, c.good)
			}
		})
	}
}

// TestLevelStringTokens pins the full Level set and their String() tokens: the four
// ordered grades plus the two non-ordered sentinels. A future sentinel added to the
// iota block without a String() case would fall through to "incomplete"; this catches
// that (both by asserting each token and by pinning LevelEnrichment as the highest
// defined value).
func TestLevelStringTokens(t *testing.T) {
	cases := []struct {
		level Level
		want  string
	}{
		{LevelIncomplete, "incomplete"},
		{LevelCore, "core"},
		{LevelStandard, "standard"},
		{LevelFull, "full"},
		{LevelObservation, "observation"},
		{LevelEnrichment, "enrichment"},
	}
	for _, c := range cases {
		if got := c.level.String(); got != c.want {
			t.Errorf("Level(%d).String() = %q, want %q", int(c.level), got, c.want)
		}
	}
	if LevelEnrichment != LevelObservation+1 {
		t.Errorf("LevelEnrichment = %d, want LevelObservation+1 (%d): a new level slipped into the iota block unpinned",
			int(LevelEnrichment), int(LevelObservation+1))
	}
}

// TestNoDuplicateLevelPath guards that no two rubric rows share a (level, path) pair.
// The appendix is de-collided to field-precise paths so this passes; a copy-paste row
// collision (two rows at the same level and path) would double-emit or mask each other
// and is caught here. Two rows of one taxonomy at DIFFERENT paths (a double-path enum)
// are allowed: the key includes the path.
func TestNoDuplicateLevelPath(t *testing.T) {
	seen := map[string]bool{}
	for _, ru := range rubric {
		key := ru.level.String() + "\x00" + ru.path
		if seen[key] {
			t.Errorf("duplicate rubric row at (level %s, path %q)", ru.level, ru.path)
		}
		seen[key] = true
	}
}

// TestAllowlistDisjointFromRubric guards that no taxonomy enum is simultaneously
// rubric-covered (some row carries its token) and listed in descriptiveAllowlist. The
// allowlist is the "consciously NOT on a roadmap" registry; an enum in both places is a
// stale allowlist entry left behind when its row landed. Multiple rows sharing one
// token (the two double-path enums) is fine: coverage is by token, so pruning the
// allowlist entry once satisfies this.
func TestAllowlistDisjointFromRubric(t *testing.T) {
	covered := rubricTaxonomies()
	for name := range descriptiveAllowlist {
		if covered[name] {
			t.Errorf("taxonomy %q is both rubric-covered and in descriptiveAllowlist; remove the allowlist entry", name)
		}
	}
}
