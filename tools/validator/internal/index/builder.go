// Package index is the canonical builder for a ULC record's `index` block.
//
// The ULC specification forbids hand-authoring the index; this package is the
// single authority. Any record whose stored `index` does not exactly match
// Build() output is considered stale.
//
// This is the Go port of tools/build-index.py. The two implementations must
// stay in lockstep during the transition; once the `ulc` CLI is authoritative,
// the Python script retires.
package index

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
)

// BuilderVersion is stamped into every index block under `builder_version`.
// Bump on any change to the projection logic so stale indices are detectable.
//
// 0.2.0: matches tools/build-index.py 0.2.0 — removed `nominal_cct_k` from
// RequiredKeys to allow color-changing fixtures (RGB, RGBW, RGBA) to pass
// validation without a placeholder CCT value.
const BuilderVersion = "0.2.0"

// RequiredKeys mirrors schema/ulc.schema.json#/$defs/Index.required. The Go
// validator enforces this set directly; the legacy Python builder-parity-guard
// becomes redundant once the Go CLI is the authoritative builder.
var RequiredKeys = []string{
	"x-ulc-generated",
	"builder_version",
	"manufacturer_slug",
	"catalog_model",
	"primary_category",
	"nominal_total_lumens",
	"nominal_input_power_w",
}

// RequiredKeySources gives the source path for each required key, used when
// the builder refuses to emit an invalid index because a deep block is sparse.
var RequiredKeySources = map[string]string{
	"manufacturer_slug":     "product_family.manufacturer.slug",
	"catalog_model":         "product_family.catalog_model",
	"primary_category":      "product_family.primary_category",
	"nominal_total_lumens":  "photometry.total_luminous_flux_lm.value",
	"nominal_input_power_w": "electrical.input_power_w.value",
}

// Record is the in-memory representation of a parsed .ulc file.
type Record = map[string]any

// Index is the projected index block.
type Index = map[string]any

// Build projects a record's deep blocks into the denormalized index block.
// Deterministic: repeated runs over the same record produce identical output.
func Build(record Record) Index {
	pf := getMap(record, "product_family")
	cfg := getMap(record, "configuration")
	elec := getMap(record, "electrical")
	phot := getMap(record, "photometry")
	color := getMap(record, "colorimetry")
	outdoor := getMap(record, "outdoor_classification")

	idx := Index{
		"x-ulc-generated": true,
		"builder_version": BuilderVersion,
	}

	if v := getString(pf, "manufacturer", "slug"); v != "" {
		idx["manufacturer_slug"] = v
	}
	if v := getString(pf, "catalog_model"); v != "" {
		idx["catalog_model"] = v
	}
	if v := getString(pf, "primary_category"); v != "" {
		idx["primary_category"] = v
	}

	// Catalog number: SKU-specific authored in configuration wins; fall back to
	// record_id for Pattern B records whose scenario covers a catalog_pattern
	// rather than a single SKU.
	if v := getString(cfg, "catalog_number"); v != "" {
		idx["catalog_number"] = v
	} else if v := getString(record, "record_id"); v != "" {
		idx["catalog_number"] = v
	}

	// Display name: scenario label wins over family display name so Pattern B
	// and Pattern C records land with the scenario-specific label.
	if v := getString(cfg, "scenario_label"); v != "" {
		idx["display_name"] = v
	} else if v := getString(pf, "family_display_name"); v != "" {
		idx["display_name"] = v
	}

	// Classification projections. Dedupe + sort semantically unordered arrays
	// so repeated builder runs produce byte-identical output.
	if arr := dedupeSortedStrings(getStringSlice(pf, "secondary_function")); len(arr) > 0 {
		idx["secondary_function"] = arr
	}
	if v := getString(pf, "indoor_outdoor"); v != "" {
		idx["indoor_outdoor"] = v
	}
	if arr := dedupeSortedStrings(getStringSlice(pf, "mounting_types")); len(arr) > 0 {
		idx["mounting_types"] = arr
	}
	if v := getString(pf, "environment_rating"); v != "" {
		idx["environment_rating"] = v
	}

	// Colorimetric and electrical baseline projections.
	// nominal_cct_at_test is typed as NominalCCT enum (string like "4000") in
	// the schema, so copy it verbatim — any downstream consumer that wants a
	// numeric CCT can parse it. This mirrors how tools/build-index.py behaves.
	if v, ok := getValue(cfg, "tested_conditions", "nominal_cct_at_test"); ok && v != nil {
		idx["nominal_cct_k"] = v
	}
	flux, fluxOK := getNumber(phot, "total_luminous_flux_lm", "value")
	if fluxOK {
		idx["nominal_total_lumens"] = numberToJSON(flux)
	}
	watts, wattsOK := getNumber(elec, "input_power_w", "value")
	if wattsOK {
		idx["nominal_input_power_w"] = numberToJSON(watts)
	}
	if v, ok := getNumber(color, "cri_ra", "value"); ok {
		idx["nominal_cri_ra"] = numberToJSON(v)
	}
	if eff, ok := getNumber(phot, "luminaire_efficacy_lm_per_w", "value"); ok {
		idx["nominal_efficacy_lm_per_w"] = numberToJSON(eff)
	} else if fluxOK && wattsOK && watts != 0 {
		idx["nominal_efficacy_lm_per_w"] = round2(flux / watts)
	}

	// Distribution projections.
	if v := getString(phot, "distribution_type"); v != "" {
		idx["primary_distribution"] = v
	}
	if v := getString(outdoor, "outdoor_distribution_type"); v != "" {
		idx["outdoor_distribution"] = v
	}
	if v := getString(phot, "beam_family"); v != "" {
		idx["beam_family"] = v
	}

	// Capability projections.
	if v := getString(cfg, "tested_axes", "color_tunability"); v != "" {
		idx["color_tunability"] = v
	}
	if v := getString(elec, "driver_protocol"); v != "" {
		idx["dimming_protocols"] = []any{v}
	}

	// Ingress and impact ratings.
	if v := getString(pf, "shared_mechanical", "ip_rating"); v != "" {
		idx["ip_rating"] = v
	}
	if v := getString(pf, "shared_mechanical", "ik_rating"); v != "" {
		idx["ik_rating"] = v
	}

	// BUG and glare.
	if bug := formatBUG(getMap(outdoor, "bug_rating")); bug != "" {
		idx["bug_rating"] = bug
	}
	if v, ok := getNumber(phot, "ugr_4h_8h", "value"); ok {
		idx["ugr_4h_8h"] = numberToJSON(v)
	}

	// Attestations rollup.
	if programs := collectAttestationPrograms(record); len(programs) > 0 {
		idx["attestation_programs"] = stringsToAny(programs)
	}

	// Search keywords: dedupe + sort, skip whitespace-only values so trailing
	// whitespace does not land as an empty entry after TrimSpace.
	keywordSet := map[string]struct{}{}
	for _, raw := range []string{
		getString(pf, "family_display_name"),
		getString(pf, "catalog_line"),
		getString(cfg, "scenario_label"),
	} {
		if trim := strings.TrimSpace(raw); trim != "" {
			keywordSet[trim] = struct{}{}
		}
	}
	if len(keywordSet) > 0 {
		keys := make([]string, 0, len(keywordSet))
		for k := range keywordSet {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		idx["search_keywords"] = stringsToAny(keys)
	}

	// Source-file types rollup.
	if types := sourceFileTypes(record); len(types) > 0 {
		idx["source_file_types_present"] = stringsToAny(types)
	}

	return idx
}

// MissingRequiredKeys returns the sorted list of RequiredKeys absent from a
// built index. Used by the CLI to refuse records whose deep blocks are too
// sparse to produce a schema-valid index.
func MissingRequiredKeys(built Index) []string {
	missing := []string{}
	for _, k := range RequiredKeys {
		if _, ok := built[k]; !ok {
			missing = append(missing, k)
		}
	}
	sort.Strings(missing)
	return missing
}

// Diff returns a shallow per-key diff between a stored and a computed index.
// Matches the behavior of tools/build-index.py's _diff() closely enough for
// drift reporting.
func Diff(stored, built Index) []string {
	keys := map[string]struct{}{}
	for k := range stored {
		keys[k] = struct{}{}
	}
	for k := range built {
		keys[k] = struct{}{}
	}
	ordered := make([]string, 0, len(keys))
	for k := range keys {
		ordered = append(ordered, k)
	}
	sort.Strings(ordered)
	diffs := []string{}
	for _, k := range ordered {
		s, sOK := stored[k]
		b, bOK := built[k]
		if !valuesEqual(s, b, sOK, bOK) {
			diffs = append(diffs, fmt.Sprintf("  %s: stored=%s != computed=%s", k, formatValue(s, sOK), formatValue(b, bOK)))
		}
	}
	return diffs
}

// --- Internal helpers ---

// getValue returns the value at a nested path, present flag tracks existence
// separately from nil so "absent" and "explicit null" are distinguishable.
func getValue(parent map[string]any, keys ...string) (any, bool) {
	if parent == nil || len(keys) == 0 {
		return nil, false
	}
	node := any(parent)
	for i, k := range keys {
		m, ok := node.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := m[k]
		if !ok {
			return nil, false
		}
		if i == len(keys)-1 {
			return v, true
		}
		node = v
	}
	return nil, false
}

func getMap(parent map[string]any, keys ...string) map[string]any {
	node := parent
	for _, k := range keys {
		if node == nil {
			return nil
		}
		v, ok := node[k]
		if !ok {
			return nil
		}
		m, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		node = m
	}
	return node
}

func getString(parent map[string]any, keys ...string) string {
	if parent == nil {
		return ""
	}
	if len(keys) == 0 {
		return ""
	}
	node := any(parent)
	for i, k := range keys {
		m, ok := node.(map[string]any)
		if !ok {
			return ""
		}
		v, ok := m[k]
		if !ok {
			return ""
		}
		if i == len(keys)-1 {
			s, ok := v.(string)
			if !ok {
				return ""
			}
			return s
		}
		node = v
	}
	return ""
}

func getNumber(parent map[string]any, keys ...string) (float64, bool) {
	if parent == nil || len(keys) == 0 {
		return 0, false
	}
	node := any(parent)
	for i, k := range keys {
		m, ok := node.(map[string]any)
		if !ok {
			return 0, false
		}
		v, ok := m[k]
		if !ok {
			return 0, false
		}
		if i == len(keys)-1 {
			switch n := v.(type) {
			case float64:
				return n, true
			case int:
				return float64(n), true
			case int64:
				return float64(n), true
			default:
				return 0, false
			}
		}
		node = v
	}
	return 0, false
}

func getStringSlice(parent map[string]any, keys ...string) []string {
	if parent == nil || len(keys) == 0 {
		return nil
	}
	node := any(parent)
	for i, k := range keys {
		m, ok := node.(map[string]any)
		if !ok {
			return nil
		}
		v, ok := m[k]
		if !ok {
			return nil
		}
		if i == len(keys)-1 {
			arr, ok := v.([]any)
			if !ok {
				return nil
			}
			out := make([]string, 0, len(arr))
			for _, item := range arr {
				if s, ok := item.(string); ok {
					out = append(out, s)
				}
			}
			return out
		}
		node = v
	}
	return nil
}

func dedupeSortedStrings(in []string) []any {
	if len(in) == 0 {
		return nil
	}
	set := map[string]struct{}{}
	for _, s := range in {
		set[s] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return stringsToAny(out)
}

func stringsToAny(in []string) []any {
	out := make([]any, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

// numberToJSON preserves the integer-ness of a number so the emitted JSON
// looks identical to what Python's json.dumps produces (3000 not 3000.0).
// Returns an int64 when the value is a whole number and fits, float64 otherwise.
func numberToJSON(v float64) any {
	if v == math.Trunc(v) && !math.IsInf(v, 0) && v >= math.MinInt64 && v <= math.MaxInt64 {
		return int64(v)
	}
	return v
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

func formatBUG(bug map[string]any) string {
	if bug == nil {
		return ""
	}
	b, bOK := bug["b"]
	u, uOK := bug["u"]
	g, gOK := bug["g"]
	if !bOK || !uOK || !gOK {
		return ""
	}
	bn, bOKn := numberAsInt(b)
	un, uOKn := numberAsInt(u)
	gn, gOKn := numberAsInt(g)
	if !bOKn || !uOKn || !gOKn {
		return ""
	}
	return fmt.Sprintf("B%d-U%d-G%d", bn, un, gn)
}

func numberAsInt(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int:
		return int64(n), true
	case int64:
		return n, true
	default:
		return 0, false
	}
}

// collectAttestationPrograms rolls up program identifiers from the three places
// they can appear: product_family.shared_attestations (Pattern A/C/D),
// record-scope attestations (Pattern B and scenario-scope overrides), and the
// sustainability_declaration block (Declare / LBC Red List status maps into
// the AttestationProgram enum).
func collectAttestationPrograms(record Record) []string {
	set := map[string]struct{}{}
	if pf := getMap(record, "product_family"); pf != nil {
		if arr, ok := pf["shared_attestations"].([]any); ok {
			for _, a := range arr {
				if m, ok := a.(map[string]any); ok {
					if p, ok := m["program"].(string); ok && p != "" {
						set[p] = struct{}{}
					}
				}
			}
		}
	}
	if arr, ok := record["attestations"].([]any); ok {
		for _, a := range arr {
			if m, ok := a.(map[string]any); ok {
				if p, ok := m["program"].(string); ok && p != "" {
					set[p] = struct{}{}
				}
			}
		}
	}
	if sd, ok := record["sustainability_declaration"].(map[string]any); ok {
		if dt, ok := sd["declaration_type"].(string); ok {
			mapping := map[string]string{
				"ilfi_declare":       "declare",
				"red_list_free":      "lbc_red_list_free",
				"red_list_approved":  "lbc_red_list_approved",
				"red_list_declared": "lbc_red_list_declared",
			}
			if mapped, ok := mapping[dt]; ok {
				set[mapped] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func sourceFileTypes(record Record) []string {
	arr, ok := record["source_files"].([]any)
	if !ok {
		return nil
	}
	set := map[string]struct{}{}
	for _, entry := range arr {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		ft, ok := m["file_type"].(string)
		if !ok || ft == "" {
			continue
		}
		set[ft] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// valuesEqual compares two values from parsed JSON trees with presence tracked
// separately so (absent vs explicit nil) and (absent vs zero) are distinguishable.
func valuesEqual(a, b any, aPresent, bPresent bool) bool {
	if aPresent != bPresent {
		return false
	}
	if !aPresent {
		return true
	}
	// Number-to-number comparison is float64-safe because the JSON parser
	// always hands us float64, and numberToJSON produces int64 for whole
	// numbers. Normalize both sides to float64 for comparison.
	if an, aok := coerceFloat(a); aok {
		if bn, bok := coerceFloat(b); bok {
			return an == bn
		}
	}
	return reflect.DeepEqual(a, b)
}

func coerceFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	default:
		return 0, false
	}
}

func formatValue(v any, present bool) string {
	if !present {
		return "<absent>"
	}
	return fmt.Sprintf("%v", v)
}
