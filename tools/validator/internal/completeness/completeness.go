// Package completeness implements the ULC conformance-level grading rubric.
//
// Grading is a pure function of an already-parsed record map. It reads only
// fields that are present in the record (no I/O, no external data, no schema
// access) and never mutates the record. The companion JSON Schema validation
// runs first and catches structural defects; grading layers the conformance
// rubric on top.
//
// A record is not assigned a conformance grade by hand: it simply IS whatever
// grade its populated fields achieve. AchievedLevel is the pure computation the
// reference builder calls to stamp index.conformance_level, and the build-parity
// check then guards that stored value like any other index field. Because there
// is no declaration to fall short of, conformance grading produces no WARNINGs.
//
// The rubric is a declarative table (var rubric): one row per graded field,
// carrying its level, JSON Pointer path, source document, governing standard, a
// present-closure (is the field populated), and an applicability-predicate (does
// the rule apply to this record at all). The grammar is a cumulative gate: walk
// the tiers low to high and stop at the first tier with an unmet hard
// requirement. Conditional rules whose applicability-predicate is false are
// dropped entirely, never reported missing.
//
// Three grades above an incomplete floor:
//
//	incomplete  the floor: a record that has not yet met a core requirement. It is
//	            not a grade. It always gets an index and a roadmap to core, and the
//	            tooling never refuses it. It is the zero value of Level.
//	core        a complete, identifiable, legally-sellable luminaire with an
//	            attached cutsheet, headline photometric/electrical numbers, one-line
//	            colorimetry, and a market safety listing.
//	standard    core plus the fuller specification an LM-79 report produces.
//	full        standard plus exhaustive accredited characterization.
//
// The table also carries two non-ordered sentinels for non-gating rows, both
// excluded from grading: LevelEnrichment for the enrichment roadmap (optional
// dimensions a record could disclose to deepen the datasheet, surfaced as
// conformance/enrichment) and LevelObservation for the residual data-quality and
// tracked-declaration notes (surfaced as conformance/observation).
//
// Compute walks the rubric once and returns a structured Result (the achieved
// level plus the tier roadmap, the enrichment roadmap, and the observation rows);
// render reproduces the human-facing INFO emission from it. AchievedLevel stays
// the independent, cheap level ladder the index builder calls.
//
// Severity model:
//
//	INFO    everything grading emits: the achieved-grade summary, the per-grade
//	        roadmap (each missing item naming its source document and standard,
//	        plus satisfied-grade and gated-grade markers), and, at core and above,
//	        the non-gating enrichment roadmap and observation notes.
package completeness

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// Level is a conformance grade, ordered incomplete < core < standard < full.
type Level int

const (
	// LevelIncomplete is the floor: a record that has not yet met core. It is not a
	// grade; it always carries a roadmap to core, and the tooling never refuses it.
	// It is the zero value, so an unset Level reads as the floor.
	LevelIncomplete Level = iota
	LevelCore
	LevelStandard
	LevelFull
	// LevelObservation: sentinel for the residual non-gating notes (sustainability
	// declaration, deprecated legacy cutoff). Excluded from AchievedLevel /
	// allRulesMet / missingAt; collected into Result.Observations by Compute.
	LevelObservation
	// LevelEnrichment: sentinel for the non-gating enrichment roadmap (optional
	// dimensions a record could disclose). Excluded from AchievedLevel /
	// allRulesMet / missingAt; collected into Result.Enrichment by Compute.
	LevelEnrichment
)

// String returns the canonical lowercase conformance_level token.
func (l Level) String() string {
	switch l {
	case LevelCore:
		return "core"
	case LevelStandard:
		return "standard"
	case LevelFull:
		return "full"
	case LevelObservation:
		return "observation"
	case LevelEnrichment:
		return "enrichment"
	default:
		return "incomplete" // floor fallback; the zero value renders here
	}
}

// predicate decides whether a rule applies to a record, or whether its field is
// present. nil in the applicable slot means universal.
type predicate func(map[string]any) bool

// rule is one row in the declarative rubric table. document and standard are
// plain strings (document is a SourceFileType token); they compose the roadmap
// message and the structured Finding fields. message is set on observation rows
// only; gating rows derive their message from path/document/standard.
type rule struct {
	level      Level
	path       string
	taxonomy   string
	document   string
	standard   string
	message    string // observation rows only
	present    predicate
	applicable predicate // nil => universal
}

// --- present-closure constructors (all route through the guarded accessors) ---

// All closures route through getMap + a comma-ok leaf; none does a raw
// multi-index assertion, because AchievedLevel is reachable from the from-sheet
// path before full schema validation and there is no recover() in the validator.

func num(keys ...string) predicate {
	return func(r map[string]any) bool { return hasNumberValue(r, keys...) }
}
func str(keys ...string) predicate {
	return func(r map[string]any) bool { return getString(r, keys...) != "" }
}

// arr: a non-empty array at a nested path. Built on getMap so a mistyped parent
// returns false rather than panicking. getMap with zero leading keys returns the
// record itself, so arr("compatible_accessories") works as a single-key call.
func arr(keys ...string) predicate {
	return func(r map[string]any) bool {
		if len(keys) == 0 {
			return false
		}
		parent, ok := getMap(r, keys[:len(keys)-1]...)
		if !ok {
			return false
		}
		a, ok := parent[keys[len(keys)-1]].([]any)
		return ok && len(a) > 0
	}
}

// scalarNum: a bare JSON number (not a ProvenancedNumber) at a nested path.
func scalarNum(keys ...string) predicate {
	return func(r map[string]any) bool {
		if len(keys) == 0 {
			return false
		}
		parent, ok := getMap(r, keys[:len(keys)-1]...)
		if !ok {
			return false
		}
		switch parent[keys[len(keys)-1]].(type) {
		case float64, int, int64:
			return true
		default:
			return false
		}
	}
}

func anyOf(ps ...predicate) predicate {
	return func(r map[string]any) bool {
		for _, p := range ps {
			if p(r) {
				return true
			}
		}
		return false
	}
}

// both builds an AND of two predicates: satisfied only when both hold. Used by the
// pvf_code enrichment row (white-light-primary AND a tm_30 block present).
func both(a, b predicate) predicate {
	return func(r map[string]any) bool { return a(r) && b(r) }
}

// arrItemHas builds a present-closure for a field inside array items: satisfied when
// the array at arrayPath carries at least ONE map entry whose leafKey holds real
// content. The nudge is "start disclosing this dimension", not "complete every
// entry", so any qualifying entry is enough. Total on hostile input (the grader runs
// pre-schema-validation): a non-array parent, a non-map entry, or a wrong-typed leaf
// reads as absent rather than panicking.
func arrItemHas(arrayPath []string, leafKey string) predicate {
	return func(r map[string]any) bool {
		if len(arrayPath) == 0 {
			return false // no array to inspect; matches arr/scalarNum's zero-key guard
		}
		parent := r
		if len(arrayPath) > 1 {
			p, ok := getMap(r, arrayPath[:len(arrayPath)-1]...)
			if !ok {
				return false
			}
			parent = p
		}
		a, ok := parent[arrayPath[len(arrayPath)-1]].([]any)
		if !ok {
			return false
		}
		for _, e := range a {
			m, ok := e.(map[string]any)
			if !ok {
				continue
			}
			if leafHasContent(m, leafKey) {
				return true
			}
		}
		return false
	}
}

// leafHasContent reports whether m[key] holds recognized real content: a non-empty
// string (enum/string leaves), a bare JSON number, or a ProvenancedNumber-shaped
// object carrying a numeric value. A missing key, an empty string, or a wrong-typed
// value reads as absent.
func leafHasContent(m map[string]any, key string) bool {
	switch v := m[key].(type) {
	case string:
		return v != ""
	case float64, int, int64:
		return true
	case map[string]any:
		switch v["value"].(type) {
		case float64, int, int64:
			return true
		}
	}
	return false
}

// hasUncertainty: coverage_factor_k present as a real number, plus at least one
// expanded_uncertainty_* carrying a ProvenancedNumber value. Both halves require
// real content: a bare scalar (or a value-less map) at an expanded_* field, or a
// non-numeric coverage_factor_k, reads as absent rather than satisfying the gate.
func hasUncertainty(r map[string]any) bool {
	m, ok := getMap(r, "uncertainty")
	if !ok {
		return false
	}
	switch m["coverage_factor_k"].(type) {
	case float64, int, int64:
	default:
		return false
	}
	for _, k := range []string{
		"expanded_uncertainty_total_flux_percent", "expanded_uncertainty_input_power_percent",
		"expanded_uncertainty_cct_k", "expanded_uncertainty_efficacy_percent",
	} {
		if hasNumberValue(m, k) {
			return true
		}
	}
	return false
}

// hasCorrectionsApplied reports whether corrections_applied carries at least one
// recognized correction of correct leaf type: one of the three *_corrected booleans
// set to a bool, or an f1_prime_value carrying a ProvenancedNumber. A notes string
// alone, or an object holding only unrecognized keys, does not count.
func hasCorrectionsApplied(r map[string]any) bool {
	m, ok := getMap(r, "corrections_applied")
	if !ok {
		return false
	}
	for _, k := range []string{
		"self_absorption_corrected", "stray_light_corrected", "spectral_mismatch_corrected",
	} {
		if _, isBool := m[k].(bool); isBool {
			return true
		}
	}
	return hasNumberValue(m, "f1_prime_value")
}

// hasPerLengthNormalized reports whether photometry.per_length_normalized carries at
// least one per-unit-length RATE as a ProvenancedNumber. The reference_length alone
// (a DualUnitLength describing the basis, not a rate) does not satisfy the gate.
func hasPerLengthNormalized(r map[string]any) bool {
	m, ok := getMap(r, "photometry", "per_length_normalized")
	if !ok {
		return false
	}
	for _, k := range []string{
		"lumens_per_foot", "lumens_per_meter", "watts_per_foot", "watts_per_meter",
	} {
		if hasNumberValue(m, k) {
			return true
		}
	}
	return false
}

// hasBugRating reports whether outdoor_classification.bug_rating carries all three
// of b, u, and g as whole-number (integer-typed) values. A partial rating, or an
// object holding only unrecognized keys, does not satisfy the gate.
func hasBugRating(r map[string]any) bool {
	m, ok := getMap(r, "outdoor_classification", "bug_rating")
	if !ok {
		return false
	}
	for _, k := range []string{"b", "u", "g"} {
		if !isWholeNumber(m[k]) {
			return false
		}
	}
	return true
}

// hasDimmingRange reports whether electrical.dimming_range_percent carries both a
// numeric min and a numeric max. A partial range (only one bound), or an object
// holding only unrecognized keys, does not satisfy the gate.
func hasDimmingRange(r map[string]any) bool {
	m, ok := getMap(r, "electrical", "dimming_range_percent")
	if !ok {
		return false
	}
	lo, okLo := asFloat(m["min"])
	hi, okHi := asFloat(m["max"])
	return okLo && okHi && lo <= hi
}

// isNumber reports whether v is one of the JSON-decoded numeric Go types the grader
// sees (float64 from the standard decoder, int / int64 after integral normalization).
func isNumber(v any) bool {
	switch v.(type) {
	case float64, int, int64:
		return true
	default:
		return false
	}
}

// isWholeNumber reports whether v is an integer-typed value: an int / int64 (an
// integral JSON number after normalization), or a float64 with no fractional part
// (an integral number from the un-normalized decoder). BUG codes are whole 0-5.
func isWholeNumber(v any) bool {
	switch n := v.(type) {
	case int, int64:
		return true
	case float64:
		return n == float64(int64(n))
	default:
		return false
	}
}

// asFloat returns v as a float64 when it is one of the JSON-decoded numeric Go
// types the grader sees (float64, int, int64), reporting false otherwise.
func asFloat(v any) (float64, bool) {
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

// --- applicability predicates ---
//
// Determinism note: GATING-row (Core/Standard/Full) applicability predicates read
// ONLY core fields, so the gating walk is order-independent (enforced by
// TestPredicatesReadOnlyCoreFields). Enrichment and observation rows are non-gating,
// so their applicability predicates MAY read parent-block presence (blockPresent and
// the has* block closures): a sub-field nudge fires only when its parent block is
// genuinely present, never affecting the achieved level.

func category(r map[string]any) string { return getString(r, "product_family", "primary_category") }

// blockPresent builds an applicability predicate for enrichment sub-field rows:
// satisfied when the block at keys is present AND a non-empty map. Present-and-empty
// is deliberately excluded: a schema-valid empty block ({}) has no minProperties, so
// bare key-presence would spam every sub-field row on a hollow block. Requiring at
// least one property means an absent or hollow parent draws only the block-level
// nudge (or none), never sub-field spam. Total on hostile input.
func blockPresent(keys ...string) predicate {
	return func(r map[string]any) bool {
		m, ok := getMap(r, keys...)
		return ok && len(m) > 0
	}
}

// directionalCategories is the set of PrimaryCategory values that throw a defined
// beam (so a beam angle is meaningful): downlights, track/spot heads, cylinders,
// wall grazers and washers, facade projectors, aimed in-ground uplights, aimed
// sports floods. Confirmed against taxonomy.schema.json PrimaryCategory.
var directionalCategories = map[string]bool{
	"downlight": true, "tracklight": true, "cylinder": true, "wall_washer": true,
	"grazer": true, "facade_projector": true, "in_ground_uplight": true, "sports_flood": true,
}

// outdoorSiteCategories is the narrowed set of outdoor area/roadway categories
// that carry a Type I-V outdoor distribution and a BUG rating. Architectural
// uplights (in_ground_uplight, facade_projector) are beam-characterized and live
// in directionalCategories; broad landscape/markers carry no Type I-V; tunnel is
// luminance-characterized with BUG undefined. All excluded on purpose.
var outdoorSiteCategories = map[string]bool{
	"flood_area_site": true, "roadway_street": true, "walkway": true,
	"bulkhead_wall_pack": true, "sports_flood": true,
}
var linearCategories = map[string]bool{"linear": true, "cove": true}

// hasWhitePoint gates the CCT family (nominal_cct_k; Duv / chromaticity
// observations). True for any color mode that defines a white point, including
// RGBW / RGBWW whose white channel has a CCT.
func hasWhitePoint(r map[string]any) bool {
	switch getString(r, "configuration", "tested_axes", "color_tunability") {
	case "static_white", "tunable_white", "dim_to_warm", "rgbw", "rgbww":
		return true
	}
	return false
}

// isWhiteLightPrimary gates the white-light-quality family: cri_ra (core),
// cri_tier + sdcm_step (standard), TM-30 (full). All color-mixing modes
// (including rgbw) waive it: their quality is not characterized by CRI / SDCM.
func isWhiteLightPrimary(r map[string]any) bool {
	switch getString(r, "configuration", "tested_axes", "color_tunability") {
	case "static_white", "tunable_white", "dim_to_warm":
		return true
	}
	return false
}

func directional(r map[string]any) bool { return directionalCategories[category(r)] }
func outdoorSite(r map[string]any) bool { return outdoorSiteCategories[category(r)] }
func linear(r map[string]any) bool      { return linearCategories[category(r)] }

// analogPhaseDimming is the protocol set whose driver publishes a dim floor and an
// electrical method on the cutsheet. `pwm` here is the PWM control-input protocol
// (the controller feeds the driver a PWM duty cycle), distinct from the `pwm`
// electrical dimming_method.
var analogPhaseDimming = map[string]bool{
	"0-10v": true, "1-10v": true, "phase_forward": true, "phase_reverse": true, "pwm": true,
}

// requiresDimmingDetail is true for the analog and phase-cut dimming protocols
// whose dim floor and method (CCR/PWM/hybrid) are published driver specs a designer
// selects on, so they are standard gates for these fixtures. Digital-command
// protocols (DALI family, DMX, DSI), wireless protocols, and non_dimming are exempt:
// their dim behavior is commanded externally or not conventionally published, so
// gating standard on it would over-gate. Reads only the core field driver_protocol,
// so grading stays order-independent.
func requiresDimmingDetail(r map[string]any) bool {
	return analogPhaseDimming[getString(r, "electrical", "driver_protocol")]
}

// nonControllableDrivers are the DimmingProtocol tokens for which the fixture cannot be
// commanded to change output, so a built-in adaptive-lighting-mode capability is not
// meaningful. Every other protocol is controllable/dimmable. Kept as an explicit set
// (verified against the DimmingProtocol enum by TestPredicateSetsAreRealEnumMembers) so
// a future non-controllable token is a conscious add rather than a silent gap.
var nonControllableDrivers = map[string]bool{
	"non_dimming": true,
}

// controllableDriver reports whether the record declares a controllable/dimmable
// driver_protocol (any protocol other than non_dimming), the precondition for a
// built-in AdaptiveLightingMode nudge. Reads only the core field driver_protocol, so
// even as an enrichment applicability predicate it stays order-independent.
func controllableDriver(r map[string]any) bool {
	p := getString(r, "electrical", "driver_protocol")
	return p != "" && !nonControllableDrivers[p]
}

// photometrySourceFileTypes are the SourceFileType tokens that carry a versioned
// photometric data format (so a photometry_format is meaningful on the entry).
var photometrySourceFileTypes = map[string]bool{
	"ies": true, "ldt": true, "tm33": true,
}

// hasPhotometrySourceFile reports whether any source_files[] entry is a photometric
// data file (file_type ies / ldt / tm33), the applicability for the PhotometryFormat
// enrichment row. Total on hostile input: a non-array source_files, or a non-map entry,
// reads as absent.
func hasPhotometrySourceFile(record map[string]any) bool {
	arr, ok := record["source_files"].([]any)
	if !ok {
		return false
	}
	for _, e := range arr {
		if m, ok := e.(map[string]any); ok {
			if s, ok := m["file_type"].(string); ok && photometrySourceFileTypes[s] {
				return true
			}
		}
	}
	return false
}

// wetOrExposed reads only core fields (environment_rating, indoor_outdoor). It
// excludes "damp": a damp-location indoor fixture carries a UL damp-location
// listing, not an ingress (IP) rating, so requiring IP for it would over-gate.
// True ingress contexts (wet / marine_coastal / outdoor_rated, or any outdoor
// fixture) still require an IP rating at standard. It does NOT read ip_rating (a
// standard rule), so the predicate layer never reads a non-core field.
func wetOrExposed(r map[string]any) bool {
	switch getString(r, "product_family", "environment_rating") {
	case "wet", "marine_coastal", "outdoor_rated":
		return true
	}
	io := getString(r, "product_family", "indoor_outdoor")
	return io == "outdoor" || io == "both"
}

func impactPublic(r map[string]any) bool {
	return getString(r, "product_family", "environment_rating") == "vandal_resistant"
}

// poleMountedTypes are the MountingType tokens for which a pole or mast wind-load
// EPA is meaningful. An exact set keeps the intent explicit (rather than a "pole" /
// "mast" substring match) and lets the drift guard verify the tokens against the enum.
var poleMountedTypes = map[string]bool{
	"pole_top": true, "pole_side_entry": true, "utility_pole_mount": true,
	"mast_arm_bracket": true, "high_mast": true,
}

func poleMounted(r map[string]any) bool {
	parent, ok := getMap(r, "product_family")
	if !ok {
		return false
	}
	mts, ok := parent["mounting_types"].([]any)
	if !ok {
		return false
	}
	for _, m := range mts {
		if s, ok := m.(string); ok && poleMountedTypes[s] {
			return true
		}
	}
	return false
}

// Safety-listing acceptance, by region. North American records require an
// NA-recognized listing; every other region accepts any recognized listing
// (including IEC 60598 / CE). naRegions enumerates the TechnicalRegion tokens
// that take the stricter NA gate; add future North American voltage variants
// (for example a 277 V or 347 V token) here.
var naRegions = map[string]bool{
	"120v_60hz_north_america": true,
}
var naSafetyListings = map[string]bool{
	"ul_listed": true, "c_ul_listed": true, "etl": true, "csa_listed": true,
	"met_listed": true, "nrtl_osha_recognized": true, "ul_1598": true,
	// ul_924 is the NRTL product-safety listing for emergency lighting and exit signs.
	// A dedicated exit sign whose only listing token is ul_924 must satisfy the core
	// safety gate rather than being stranded (the stranding this release exists to fix).
	"ul_924": true,
}

// anySafetyListings accepts any recognized listing, North American or otherwise. The
// US/Canada NRTL marks (ul_listed, c_ul_listed, etl, csa_listed, met_listed,
// nrtl_osha_recognized, ul_1598) are all included alongside the international schemes
// (CE / ENEC / IEC 60598 / CB and the regional marks), so a non-NA record listing a
// specific NRTL mark satisfies the gate consistently with nrtl_osha_recognized.
var anySafetyListings = map[string]bool{
	"ul_listed": true, "c_ul_listed": true, "etl": true, "csa_listed": true,
	"met_listed": true, "nrtl_osha_recognized": true, "ul_1598": true, "tuv": true,
	"cb_scheme": true, "ce": true, "ukca": true, "enec": true, "iec_60598": true,
	"nom": true, "ccc": true, "rcm_australia": true, "saa_australia": true,
	"kc_korea": true, "pse_japan": true,
	// ul_924 (emergency lighting / exit sign safety listing), so a non-NA dedicated
	// product listing only ul_924 also satisfies the general safety gate.
	"ul_924": true,
}

// hasMarketSafetyListing checks for the PRESENCE OF A CLAIM, not third-party
// verification. A self-asserted listing token satisfies the core gate; see
// docs/compliance-attestation.md. North American records require an NA-recognized
// listing; everywhere else any recognized listing (including IEC 60598 / CE)
// satisfies the gate.
func hasMarketSafetyListing(r map[string]any) bool {
	accept := anySafetyListings
	if naRegions[getString(r, "product_family", "technical_region")] {
		accept = naSafetyListings
	}
	for _, p := range attestationPrograms(r) {
		if accept[p] {
			return true
		}
	}
	return false
}

// hasCutsheet checks that the product-family cutsheet is attached with a content
// hash. The cutsheet is a FileReference object whose sha256 is schema-required, so
// keying on sha256 means a placeholder object (no hash) reads as absent. The
// cutsheet moved from schema-required to a graded core item, so an unattached
// cutsheet now grades incomplete with a roadmap entry instead of failing schema
// validation.
func hasCutsheet(r map[string]any) bool {
	m, ok := getMap(r, "product_family", "cutsheet")
	if !ok {
		return false
	}
	return getString(m, "sha256") != ""
}

// --- v0.10.0 emergency & exit-sign class predicates ---
//
// Class derivation is primary_category-first (§2.1): every profile selector reads a core
// field, so a gating row that selects a profile holds the read-only-core invariant by
// construction. All predicates are total on hostile input (grading runs pre-schema).

// dedicatedCategories is the set of PrimaryCategory tokens that take a dedicated
// (non-normal) grading profile: the exit sign and the dedicated emergency luminaire.
var dedicatedCategories = map[string]bool{
	"exit_sign": true, "emergency_luminaire": true,
}

// emergencyModeRoles is the set of EmergencyRole tokens whose products have a DISTINCT
// emergency operating mode (a normal mode plus a different emergency mode). The three
// emergency-mode enrichment nudges (§4.5) share it; category b
// (dedicated_emergency_luminaire, whose single mode IS its photometry) and plain signs
// (exit_sign_only) are excluded, preventing double-reporting (§2.5/§2.9).
var emergencyModeRoles = map[string]bool{
	"combo_exit_emergency": true, "emergency_power_option": true,
}

// isExitSign selects the sign profile (mirrors the directional one-liner).
func isExitSign(r map[string]any) bool { return category(r) == "exit_sign" }

// isDedicatedEmergency selects the dedicated-emergency-luminaire profile.
func isDedicatedEmergency(r map[string]any) bool { return category(r) == "emergency_luminaire" }

// isDedicatedClass is the union of the two dedicated classes (exit sign and dedicated
// emergency luminaire). It scopes the UL 924 core listing row (via naDedicatedClass) and,
// through notDedicatedClass, waives luminaire efficacy for a battery-operated dedicated
// product. The battery trio scopes on emergencyPowerCoreClass, not this, so an externally
// illuminated sign is dedicated-class yet battery-nudged rather than battery-gated.
func isDedicatedClass(r map[string]any) bool { return dedicatedCategories[category(r)] }

// notExitSign is the LITERAL negation of isExitSign, marking the universal rows the sign
// profile excludes. Kept as an explicit negation so the input-power single-gap guarantee
// holds by construction: the core input-power row is notExitSign and the standard re-gate
// is (isExitSign AND internally illuminated), so at most one applies to any record.
// TestNotExitSignIsLiteralNegation pins the exclusivity.
func notExitSign(r map[string]any) bool { return !isExitSign(r) }

// notDedicatedClass is the literal negation of isDedicatedClass: the single applicable
// slot for the efficacy row (lm per AC-charging-watt is not meaningful for a
// battery-operated dedicated product, §2.5).
func notDedicatedClass(r map[string]any) bool { return !isDedicatedClass(r) }

// naDedicatedClass applies the UL 924 core listing row: a dedicated-class product in an
// NA technical region (US-first, §2.3/§2.11). Non-NA dedicated products face only the
// general any-recognized-listing safety row.
func naDedicatedClass(r map[string]any) bool {
	return isDedicatedClass(r) && naRegions[getString(r, "product_family", "technical_region")]
}

// hasIntegralBattery is a class-AGNOSTIC leaf check: emergency.power_source ==
// "integral_battery", nothing else. Class scope (the power-source-core gate versus the
// battery nudge) is composed per-row. It is NOT block-presence semantics: an ac_only /
// inverter / generator unit reads false, so a disclosed emergency block never over-gates.
func hasIntegralBattery(r map[string]any) bool {
	return getString(r, "emergency", "power_source") == "integral_battery"
}

// signMode builds a present-style predicate satisfied when exit_sign.illumination_mode
// equals want. Absent or junk mode returns "" (getString), so signMode is false and its
// negation true, which is how the "mode != externally_illuminated" arms cover absent and
// junk modes (§2.2).
func signMode(want string) predicate {
	return func(r map[string]any) bool { return getString(r, "exit_sign", "illumination_mode") == want }
}

// selfEmittingSign is an exit sign in a self-emitting mode: any mode other than
// externally_illuminated (absent and junk modes included, §2.2). It backs the sign full
// luminance row and the illumination-technology enrichment nudge, both of which apply to
// every sign whose own face emits (internally illuminated, photoluminescent, self-luminous)
// and not to a sign lit by a separate external luminaire.
func selfEmittingSign(r map[string]any) bool {
	return isExitSign(r) && !signMode("externally_illuminated")(r)
}

// emergencyPowerCoreClass is the class for which emergency.power_source is a CORE field: a
// dedicated emergency luminaire, or an internally illuminated exit sign (both carry a
// powered-face / dedicated-emergency story). It is exactly the /emergency/power_source core
// row's scope (§2.4). The battery trio gates WITHIN this class and nudges as enrichment
// OUTSIDE it, so hasIntegralBattery (which reads power_source) is only ever consulted by a
// gating row where power_source is core-guaranteed. That keeps the read-only-core invariant
// by construction: an externally illuminated combo sign, whose power_source is not core, is
// battery-nudged, not battery-gated, so disclosing its emergency block never lowers its grade.
func emergencyPowerCoreClass(r map[string]any) bool {
	return isDedicatedEmergency(r) || (isExitSign(r) && signMode("internally_illuminated")(r))
}

// hasUL924Listing reports whether the merged attestation program list contains ul_924,
// reading the same list hasMarketSafetyListing reads (top-level attestations[] plus
// product_family.shared_attestations, via attestationPrograms).
func hasUL924Listing(r map[string]any) bool {
	for _, p := range attestationPrograms(r) {
		if p == "ul_924" {
			return true
		}
	}
	return false
}

// testReportBacked builds a present-closure satisfied only when the field at keys is a
// present ProvenancedNumber whose provenance.source is "test_report". This is the
// package's first provenance-reading gate (§2.6): at the sign full tier the authored
// provenance object IS the evidence, so a measured value with NO provenance object
// (ProvenancedNumber does not require one) deliberately fails. Total on hostile input:
// a missing field, a non-map provenance, or a wrong-typed source all read false.
func testReportBacked(keys ...string) predicate {
	return func(r map[string]any) bool {
		if !hasNumberValue(r, keys...) {
			return false // not a present ProvenancedNumber
		}
		field, ok := getMap(r, keys...)
		if !ok {
			return false
		}
		prov, ok := getMap(field, "provenance")
		if !ok {
			return false // no authored provenance object: fails full deliberately (§2.6)
		}
		return getString(prov, "source") == "test_report"
	}
}

// hasEmergencyModeRole is satisfied when the emergency block carries a role with a
// DISTINCT emergency operating mode (combo_exit_emergency or emergency_power_option). It
// backs the three emergency-mode enrichment nudges (§2.9). getString returns "" for an
// absent block, so a block-absent record reads false. Enrichment rows may read block
// fields; they never gate.
func hasEmergencyModeRole(r map[string]any) bool {
	return emergencyModeRoles[getString(r, "emergency", "emergency_role")]
}

// batteryNudgeClass surfaces the battery trio as NON-gating enrichment: a record carrying
// an integral battery whose class does NOT core-gate power_source, namely a category-d
// fixture with a factory emergency option or an externally illuminated combo sign. Where
// power_source IS core (emergencyPowerCoreClass) the trio gates at standard instead, so the
// two scopes partition on the power_source-core boundary and a field is never reported twice
// (§2.9). hasIntegralBattery already implies the block is present.
func batteryNudgeClass(r map[string]any) bool {
	return hasIntegralBattery(r) && !emergencyPowerCoreClass(r)
}

// hasEmergencyPhotometryReference is satisfied when the emergency-mode photometry file is
// attached with a content hash, keying on sha256 like hasCutsheet (the cutsheet
// two-places precedent, §2.5).
func hasEmergencyPhotometryReference(r map[string]any) bool {
	m, ok := getMap(r, "emergency", "photometry_reference")
	if !ok {
		return false
	}
	return getString(m, "sha256") != ""
}

// --- the rubric table ---

// One row per section-3 item. Gating rows omit message (the roadmap derives it);
// observation rows carry an explicit message. Paths that are prose labels rather
// than single resolvable fields (safety listing, LM-79 attestation, input
// voltage, lumen maintenance, operating point, instrumentation, method-backed
// maintenance) use a predicate-backed present-closure and are skipped by the
// shape-guard test.
var rubric = []rule{
	// --- CORE ---
	{LevelCore, "/product_family/manufacturer/slug", "", "datasheet_pdf", "identity", "", str("product_family", "manufacturer", "slug"), nil},
	{LevelCore, "/product_family/manufacturer/display_name", "", "datasheet_pdf", "identity", "", str("product_family", "manufacturer", "display_name"), nil},
	{LevelCore, "/product_family/catalog_model", "", "datasheet_pdf", "identity", "", str("product_family", "catalog_model"), nil},
	{LevelCore, "/product_family/cutsheet", "", "datasheet_pdf", "identity", "", hasCutsheet, nil},
	{LevelCore, "/product_family/primary_category", "PrimaryCategory", "datasheet_pdf", "identity", "", str("product_family", "primary_category"), nil},
	{LevelCore, "/product_family/indoor_outdoor", "IndoorOutdoor", "datasheet_pdf", "identity", "", str("product_family", "indoor_outdoor"), nil},
	{LevelCore, "/product_family/secondary_function", "SecondaryFunction", "datasheet_pdf", "identity", "", arr("product_family", "secondary_function"), notExitSign},
	{LevelCore, "/product_family/mounting_types", "MountingType", "datasheet_pdf", "identity", "", arr("product_family", "mounting_types"), nil},
	{LevelCore, "/product_family/environment_rating", "EnvironmentRating", "datasheet_pdf", "identity", "", str("product_family", "environment_rating"), nil},
	{LevelCore, "/product_family/shape", "Shape", "datasheet_pdf", "identity", "", str("product_family", "shape"), nil},
	{LevelCore, "/product_family/technical_region", "TechnicalRegion", "datasheet_pdf", "identity", "", str("product_family", "technical_region"), nil},
	{LevelCore, "/photometry/distribution_type", "DistributionType", "ies", "LM-79", "", str("photometry", "distribution_type"), notExitSign},
	{LevelCore, "/configuration/tested_axes/color_tunability", "ColorTunabilityCapability", "datasheet_pdf", "identity", "", str("configuration", "tested_axes", "color_tunability"), notExitSign},
	{LevelCore, "/electrical/driver_protocol", "DimmingProtocol", "datasheet_pdf", "identity", "", str("electrical", "driver_protocol"), notExitSign},
	{LevelCore, "/photometry/total_luminous_flux_lm", "", "ies", "LM-79", "", num("photometry", "total_luminous_flux_lm"), notExitSign},
	{LevelCore, "/electrical/input_power_w", "", "ies", "LM-79", "", num("electrical", "input_power_w"), notExitSign},
	{LevelCore, "/photometry/luminaire_efficacy_lm_per_w", "", "ies", "LM-79", "", num("photometry", "luminaire_efficacy_lm_per_w"), notDedicatedClass},
	{LevelCore, "/electrical/input_voltage_v (or input_voltage_class)", "", "datasheet_pdf", "LM-79", "", anyOf(num("electrical", "input_voltage_v"), str("electrical", "input_voltage_class")), notExitSign},
	{LevelCore, "/colorimetry/nominal_cct_k", "NominalCCT", "datasheet_pdf", "ANSI C78.377", "", str("colorimetry", "nominal_cct_k"), hasWhitePoint},
	{LevelCore, "/colorimetry/cri_ra", "", "datasheet_pdf", "CIE 13.3", "", num("colorimetry", "cri_ra"), isWhiteLightPrimary},
	{LevelCore, "safety listing (UL/cUL/ETL/CSA for NA; CE/ENEC/IEC 60598 otherwise)", "AttestationProgram", "compliance_documents", "UL 1598 / IEC 60598", "", hasMarketSafetyListing, nil},

	// v0.10.0 exit-sign & emergency CORE rows (§4.2). illumination_mode is the sign
	// profile's conditional driver (§2.2); power_source demands the emergency block for
	// powered signs and dedicated emergency luminaires (§2.4); the UL 924 listing row is
	// US-first (naDedicatedClass, §2.3/§2.11) and its prose-label path is shape-guard-exempt.
	{LevelCore, "/exit_sign/illumination_mode", "ExitSignIlluminationMode", "datasheet_pdf", "UL 924", "", str("exit_sign", "illumination_mode"), isExitSign},
	{LevelCore, "/exit_sign/legend_color", "LegendColor", "datasheet_pdf", "NFPA 101", "", str("exit_sign", "legend_color"), isExitSign},
	{LevelCore, "/emergency/power_source", "EmergencyPowerSource", "datasheet_pdf", "UL 924", "", str("emergency", "power_source"), emergencyPowerCoreClass},
	{LevelCore, "UL 924 listing", "AttestationProgram", "compliance_documents", "UL 924", "", hasUL924Listing, naDedicatedClass},

	// --- STANDARD ---
	{LevelStandard, "/photometry/maximum_intensity_cd", "", "ies", "LM-79", "", num("photometry", "maximum_intensity_cd"), notExitSign},
	{LevelStandard, "/photometry/symmetry_type", "SymmetryType", "ies", "LM-75", "", str("photometry", "symmetry_type"), notExitSign},
	{LevelStandard, "/photometry/photometric_coordinate_system", "PhotometricCoordinateSystem", "ies", "LM-75", "", str("photometry", "photometric_coordinate_system"), notExitSign},
	{LevelStandard, "/electrical/control_gear_type", "ControlGearType", "datasheet_pdf", "LM-79", "", str("electrical", "control_gear_type"), notExitSign},
	{LevelStandard, "/product_family/shared_mechanical/housing_material", "HousingMaterial", "datasheet_pdf", "identity", "", str("product_family", "shared_mechanical", "housing_material"), nil},
	{LevelStandard, "/product_family/shared_mechanical/lens_material", "LensMaterial", "datasheet_pdf", "identity", "", str("product_family", "shared_mechanical", "lens_material"), nil},
	{LevelStandard, "/test_conditions/photometry_basis", "PhotometryBasis", "ies", "LM-79", "", str("test_conditions", "photometry_basis"), notExitSign},
	{LevelStandard, "/instrumentation/measurement_regime", "MeasurementRegime", "ies", "LM-79", "", str("instrumentation", "measurement_regime"), notExitSign},
	{LevelStandard, "LM-79 attestation", "AttestationProgram", "test_report", "LM-79", "", hasLM79Attestation, notExitSign},
	{LevelStandard, "/lumen_maintenance_luminaire (or /lumen_maintenance_package)", "", "datasheet_pdf", "TM-21", "", hasLumenMaintenance, notExitSign},
	{LevelStandard, "/photometry/beam_angle_deg", "", "ies", "LM-79", "", num("photometry", "beam_angle_deg"), directional},
	{LevelStandard, "/photometry/per_length_normalized", "", "datasheet_pdf", "LM-79", "", hasPerLengthNormalized, linear},
	{LevelStandard, "/photometry/declared_by_length", "", "datasheet_pdf", "LM-79", "", arr("photometry", "declared_by_length"), linear},
	{LevelStandard, "/configuration/tested_axes/cri_tier", "CriTier", "datasheet_pdf", "CIE 13.3", "", str("configuration", "tested_axes", "cri_tier"), isWhiteLightPrimary},
	{LevelStandard, "/colorimetry/sdcm_step", "", "datasheet_pdf", "ANSI C78.377", "", num("colorimetry", "sdcm_step"), isWhiteLightPrimary},
	{LevelStandard, "/electrical/dimming_method", "DimmingMethod", "datasheet_pdf", "identity", "", str("electrical", "dimming_method"), requiresDimmingDetail},
	{LevelStandard, "/electrical/dimming_range_percent", "", "datasheet_pdf", "identity", "", hasDimmingRange, requiresDimmingDetail},
	{LevelStandard, "/product_family/shared_mechanical/ip_rating", "", "compliance_documents", "IEC 60529", "", str("product_family", "shared_mechanical", "ip_rating"), wetOrExposed},
	{LevelStandard, "/outdoor_classification/outdoor_distribution_type", "OutdoorDistributionType", "ies", "RP-8", "", str("outdoor_classification", "outdoor_distribution_type"), outdoorSite},
	{LevelStandard, "/outdoor_classification/longitudinal_distribution_range", "LongitudinalDistributionRange", "ies", "RP-8", "", str("outdoor_classification", "longitudinal_distribution_range"), outdoorSite},
	{LevelStandard, "/outdoor_classification/bug_rating", "", "datasheet_pdf", "TM-15", "", hasBugRating, outdoorSite},

	// v0.10.0 exit-sign & emergency STANDARD rows (§4.2). Mode-partitioned per §2.2/§2.10a:
	// luminance gates only for photoluminescent/self-luminous (the modes whose datasheets
	// publish it); face illuminance and contrast for externally illuminated; input power
	// re-gates for internally illuminated (same path as the notExitSign core row, distinct
	// (level,path)); the battery trio gates only when the power-source-core class (dedicated
	// emergency luminaire or internally illuminated sign) carries an integral battery, and an
	// externally illuminated combo sign's battery depth nudges as enrichment instead (§2.9).
	// legend_height uses the DualUnitLength mm leaf (§2.6a/§2.7).
	{LevelStandard, "/exit_sign/legend_height", "", "datasheet_pdf", "NFPA 101 / IBC", "", scalarNum("exit_sign", "legend_height", "mm"), isExitSign},
	{LevelStandard, "/exit_sign/face_count", "ExitSignFaceCount", "datasheet_pdf", "UL 924", "", str("exit_sign", "face_count"), isExitSign},
	{LevelStandard, "/exit_sign/directional_indicator", "ExitSignDirectionalIndicator", "datasheet_pdf", "NFPA 101", "", arr("exit_sign", "directional_indicator"), isExitSign},
	{LevelStandard, "/exit_sign/sign_face_luminance_cd_per_m2", "", "datasheet_pdf", "UL 924", "", num("exit_sign", "sign_face_luminance_cd_per_m2"), both(isExitSign, anyOf(signMode("photoluminescent"), signMode("self_luminous")))},
	{LevelStandard, "/exit_sign/face_illuminance_lx", "", "datasheet_pdf", "NFPA 101 / IBC", "", num("exit_sign", "face_illuminance_lx"), both(isExitSign, signMode("externally_illuminated"))},
	{LevelStandard, "/exit_sign/contrast_ratio", "", "datasheet_pdf", "NFPA 101", "", num("exit_sign", "contrast_ratio"), both(isExitSign, signMode("externally_illuminated"))},
	{LevelStandard, "/electrical/input_power_w", "", "datasheet_pdf", "UL 924", "", num("electrical", "input_power_w"), both(isExitSign, signMode("internally_illuminated"))},
	{LevelStandard, "/exit_sign/tritium_rated_life_years", "", "datasheet_pdf", "NRC 10 CFR 31.5", "", scalarNum("exit_sign", "tritium_rated_life_years"), both(isExitSign, signMode("self_luminous"))},
	{LevelStandard, "/exit_sign/min_charging_illuminance_lx", "", "datasheet_pdf", "UL 924", "", num("exit_sign", "min_charging_illuminance_lx"), both(isExitSign, signMode("photoluminescent"))},
	{LevelStandard, "/emergency/battery_duration_min", "", "datasheet_pdf", "UL 924", "", num("emergency", "battery_duration_min"), both(emergencyPowerCoreClass, hasIntegralBattery)},
	{LevelStandard, "/emergency/battery_chemistry", "BatteryChemistry", "datasheet_pdf", "UL 924", "", str("emergency", "battery_chemistry"), both(emergencyPowerCoreClass, hasIntegralBattery)},
	{LevelStandard, "/emergency/self_test", "EmergencySelfTestCapability", "datasheet_pdf", "UL 924", "", str("emergency", "self_test"), both(emergencyPowerCoreClass, hasIntegralBattery)},

	// --- FULL ---
	{LevelFull, "/photometry/zonal_lumens", "", "ies", "LM-79", "", arr("photometry", "zonal_lumens"), notExitSign},
	{LevelFull, "/operating_point", "", "test_report", "LM-79", "", hasOperatingPoint, notExitSign},
	{LevelFull, "/uncertainty", "", "test_report", "LM-79 / GUM", "", hasUncertainty, notExitSign},
	{LevelFull, "/corrections_applied", "", "test_report", "LM-79", "", hasCorrectionsApplied, notExitSign},
	{LevelFull, "instrumentation depth (goniometer/lab)", "", "test_report", "LM-79 / LM-75", "", hasInstrumentationDepth, notExitSign},
	{LevelFull, "method-backed lumen maintenance (TM-21 hours or TM-28)", "", "test_report", "LM-80 / TM-21 / TM-28", "", hasMethodBackedLumenMaintenance, notExitSign},
	{LevelFull, "/colorimetry/tm_30/rf", "", "test_report", "TM-30", "", num("colorimetry", "tm_30", "rf"), isWhiteLightPrimary},
	{LevelFull, "/colorimetry/tm_30/rf_h_per_bin", "", "test_report", "TM-30", "", arr("colorimetry", "tm_30", "rf_h_per_bin"), isWhiteLightPrimary},

	// v0.10.0 exit-sign FULL tier (§2.6): two provenance-reading rows that PARTITION the
	// sign class by mode, so every sign has exactly one applicable full row and a standard
	// sign cannot auto-promote to full on zero applicable rows. testReportBacked is the
	// package's first provenance-reading gate (a measured value with no provenance object
	// fails full, deliberately). The prose-label paths are shape-guard-exempt. The first
	// row's mode arm is the literal negation !signMode("externally_illuminated"), which
	// also covers absent/junk modes (§2.2).
	{LevelFull, "test-report-backed sign-face luminance", "", "test_report", "UL 924", "", testReportBacked("exit_sign", "sign_face_luminance_cd_per_m2"), selfEmittingSign},
	{LevelFull, "test-report-backed face illuminance", "", "test_report", "UL 924", "", testReportBacked("exit_sign", "face_illuminance_lx"), both(isExitSign, signMode("externally_illuminated"))},

	// --- ENRICHMENT (non-gating; the enrichment roadmap; surfaced at core and above) ---
	// Optional dimensions a record could disclose to deepen the datasheet. Messages
	// are byte-identical to their prior conformance/observation wording; only the
	// finding code changes (see the 0.9.0 CHANGELOG consumer-migration note).
	{LevelEnrichment, "/electrical/power_factor", "", "datasheet_pdf", "LM-79", "full records commonly disclose power factor; absent here", num("electrical", "power_factor"), notExitSign},
	{LevelEnrichment, "/product_family/shared_warranty/term_years", "", "datasheet_pdf", "identity", "warranty term not disclosed", scalarNum("product_family", "shared_warranty", "term_years"), nil},
	{LevelEnrichment, "/photometry/luminous_opening_shape", "LuminousOpeningShape", "ies", "LM-79", "luminous opening shape not disclosed", str("photometry", "luminous_opening_shape"), notExitSign},
	{LevelEnrichment, "/photometry/emission_face", "EmissionFace", "ies", "LM-79", "emission face not disclosed", str("photometry", "emission_face"), notExitSign},
	{LevelEnrichment, "/colorimetry/duv", "", "test_report", "ANSI C78.377", "Duv (distance from the Planckian locus) not disclosed", num("colorimetry", "duv"), hasWhitePoint},
	{LevelEnrichment, "/colorimetry/chromaticity_x", "", "test_report", "ANSI C78.377", "chromaticity x not disclosed", num("colorimetry", "chromaticity_x"), hasWhitePoint},
	{LevelEnrichment, "/colorimetry/chromaticity_y", "", "test_report", "ANSI C78.377", "chromaticity y not disclosed", num("colorimetry", "chromaticity_y"), hasWhitePoint},
	{LevelEnrichment, "/product_family/shared_mechanical/ambient_operating_range", "", "datasheet_pdf", "identity", "ambient operating range not disclosed", hasAmbientOperatingRange, nil},
	{LevelEnrichment, "/compatible_accessories", "AccessoryType", "datasheet_pdf", "identity", "compatible accessories not listed", arr("compatible_accessories"), nil},
	{LevelEnrichment, "/thermal_derating", "", "test_report", "LM-82", "thermal derating not disclosed", hasThermalDerating, notExitSign},
	{LevelEnrichment, "/flicker_measurements", "", "test_report", "LM-90 / IEEE 1789", "flicker measurements not disclosed", hasFlickerMeasurements, notExitSign},
	{LevelEnrichment, "/alpha_opic_metrics", "", "test_report", "CIE S 026", "alpha-opic (circadian) metrics not disclosed", hasAlphaOpicMetrics, notExitSign},
	{LevelEnrichment, "/chromaticity_shift_projection", "", "test_report", "TM-35", "chromaticity-shift projection not disclosed", hasChromaticityShiftProjection, notExitSign},
	{LevelEnrichment, "/photometry/field_angle_deg", "", "ies", "LM-79", "field angle not disclosed", num("photometry", "field_angle_deg"), directional},
	{LevelEnrichment, "/photometry/cutoff_angle_from_horizontal_deg", "", "ies", "LM-79", "cutoff angle not disclosed", num("photometry", "cutoff_angle_from_horizontal_deg"), notExitSign},
	{LevelEnrichment, "/photometry/spacing_criterion", "", "ies", "LM-79", "spacing criterion not disclosed", num("photometry", "spacing_criterion"), notExitSign},
	{LevelEnrichment, "/photometry/ugr_4h_8h", "", "ies", "CIE 117", "UGR not disclosed", num("photometry", "ugr_4h_8h"), notExitSign},
	{LevelEnrichment, "/outdoor_classification/lcs_zonal_lumens", "", "ies", "TM-15", "LCS zonal lumens not disclosed", arr("outdoor_classification", "lcs_zonal_lumens"), outdoorSite},
	{LevelEnrichment, "/product_family/shared_mechanical/ik_rating", "", "compliance_documents", "IEC 62262", "impact (IK) rating not disclosed", str("product_family", "shared_mechanical", "ik_rating"), impactPublic},
	{LevelEnrichment, "/product_family/physical_dimensions/epa", "", "datasheet_pdf", "identity", "EPA (effective projected area) not disclosed", hasEPA, poleMounted},

	// Bucket B: new enrichment rows gated on their parent block being present, so an
	// absent block draws one block-level nudge (or none), never sub-field spam.
	// photometry.
	{LevelEnrichment, "/photometry/beam_family", "BeamFamily", "ies", "LM-79", "beam family not disclosed", str("photometry", "beam_family"), blockPresent("photometry")},
	// test_conditions.
	{LevelEnrichment, "/test_conditions/photometry_method", "PhotometryMethod", "ies", "LM-79", "photometry method not disclosed", str("test_conditions", "photometry_method"), blockPresent("test_conditions")},
	{LevelEnrichment, "/test_conditions/stabilization_method", "StabilizationMethod", "ies", "LM-79", "stabilization method not disclosed", str("test_conditions", "stabilization_method"), blockPresent("test_conditions")},
	{LevelEnrichment, "/test_conditions/negative_intensity_handling", "NegativeIntensityHandling", "ies", "LM-79", "negative intensity handling not disclosed", str("test_conditions", "negative_intensity_handling"), blockPresent("test_conditions")},
	{LevelEnrichment, "/test_conditions/file_generation_type", "FileGenerationType", "ies", "LM-63", "file generation type not disclosed", str("test_conditions", "file_generation_type"), blockPresent("test_conditions")},
	{LevelEnrichment, "/test_conditions/nonstandard_condition_flags", "NonstandardConditionFlag", "ies", "LM-79", "nonstandard condition flags not disclosed", arr("test_conditions", "nonstandard_condition_flags"), blockPresent("test_conditions")},
	// operating_point (its own qualifiers, gated on a genuinely-present operating point).
	{LevelEnrichment, "/operating_point/dut_operating_mode", "DutOperatingMode", "test_report", "LM-79", "device-under-test operating mode not disclosed", str("operating_point", "dut_operating_mode"), hasOperatingPoint},
	{LevelEnrichment, "/operating_point/case_temperature_monitoring_point", "TemperatureMonitoringPoint", "test_report", "LM-79", "case-temperature monitoring point not disclosed", str("operating_point", "case_temperature_monitoring_point"), hasOperatingPoint},
	// lumen_maintenance_package (per-entry; any entry carrying the field satisfies).
	{LevelEnrichment, "/lumen_maintenance_package[]/flux_maintenance_quantity", "FluxMaintenanceQuantity", "test_report", "TM-21", "flux maintenance quantity not disclosed", arrItemHas([]string{"lumen_maintenance_package"}, "flux_maintenance_quantity"), hasLumenMaintenancePackage},
	{LevelEnrichment, "/lumen_maintenance_package[]/tested_product_type", "TestedProductType", "test_report", "LM-80", "tested product type not disclosed", arrItemHas([]string{"lumen_maintenance_package"}, "tested_product_type"), hasLumenMaintenancePackage},
	{LevelEnrichment, "/lumen_maintenance_package[]/tm_21_interpolation_type", "TM21InterpolationType", "test_report", "TM-21", "TM-21 interpolation type not disclosed", arrItemHas([]string{"lumen_maintenance_package"}, "tm_21_interpolation_type"), hasLumenMaintenancePackage},
	// FluxMaintenanceThreshold: two legs sharing one taxonomy token (package entry, luminaire claim).
	{LevelEnrichment, "/lumen_maintenance_package[]/flux_maintenance_threshold", "FluxMaintenanceThreshold", "test_report", "TM-21", "flux maintenance threshold not disclosed", arrItemHas([]string{"lumen_maintenance_package"}, "flux_maintenance_threshold"), hasLumenMaintenancePackage},
	{LevelEnrichment, "/lumen_maintenance_luminaire/manufacturer_rated_claim/claim_type", "FluxMaintenanceThreshold", "datasheet_pdf", "TM-21", "manufacturer-rated maintenance claim type not disclosed", str("lumen_maintenance_luminaire", "manufacturer_rated_claim", "claim_type"), hasLumenMaintenanceLuminaire},
	// ProjectionReliability: two legs sharing one taxonomy token (package entry, tm_28 sub-block).
	{LevelEnrichment, "/lumen_maintenance_package[]/projection_reliability", "ProjectionReliability", "test_report", "TM-21", "projection reliability not disclosed", arrItemHas([]string{"lumen_maintenance_package"}, "projection_reliability"), hasLumenMaintenancePackage},
	{LevelEnrichment, "/lumen_maintenance_luminaire/tm_28/projection_reliability", "ProjectionReliability", "test_report", "TM-28", "TM-28 projection reliability not disclosed", str("lumen_maintenance_luminaire", "tm_28", "projection_reliability"), hasLumenMaintenanceLuminaire},
	// lumen_maintenance_luminaire (tm_28 + framework).
	{LevelEnrichment, "/lumen_maintenance_luminaire/tm_28/projection_basis", "ProjectionBasis", "test_report", "TM-28", "TM-28 projection basis not disclosed", str("lumen_maintenance_luminaire", "tm_28", "projection_basis"), hasLumenMaintenanceLuminaire},
	{LevelEnrichment, "/lumen_maintenance_luminaire/tm_28/projection_method", "LumenMaintenanceProjectionMethod", "test_report", "TM-28", "lumen-maintenance projection method not disclosed", str("lumen_maintenance_luminaire", "tm_28", "projection_method"), hasLumenMaintenanceLuminaire},
	{LevelEnrichment, "/lumen_maintenance_luminaire/declaration_framework", "LumenMaintenanceDeclarationFramework", "datasheet_pdf", "TM-21 / TM-28", "lumen-maintenance declaration framework not disclosed", str("lumen_maintenance_luminaire", "declaration_framework"), hasLumenMaintenanceLuminaire},
	// AmbientCleanliness: block-level (its leaf is required in the CIE 97 table items).
	{LevelEnrichment, "/lumen_maintenance_luminaire/cie_97_lmf_table", "AmbientCleanliness", "datasheet_pdf", "CIE 97", "CIE 97 luminaire maintenance factor table not disclosed", hasCie97LmfTable, hasLumenMaintenanceLuminaire},
	// chromaticity_shift_projection sub-fields.
	{LevelEnrichment, "/chromaticity_shift_projection/shift_metric", "ChromaticityShiftMetric", "test_report", "TM-35", "chromaticity-shift metric not disclosed", str("chromaticity_shift_projection", "shift_metric"), hasChromaticityShiftProjection},
	{LevelEnrichment, "/chromaticity_shift_projection/shift_threshold", "ChromaticityShiftThreshold", "test_report", "TM-35", "chromaticity-shift threshold not disclosed", str("chromaticity_shift_projection", "shift_threshold"), hasChromaticityShiftProjection},
	{LevelEnrichment, "/chromaticity_shift_projection/shift_mode", "ChromaticityShiftMode", "test_report", "TM-35", "chromaticity-shift mode not disclosed", str("chromaticity_shift_projection", "shift_mode"), hasChromaticityShiftProjection},
	{LevelEnrichment, "/chromaticity_shift_projection/tm_35_edition", "TM35Edition", "test_report", "TM-35", "TM-35 edition not disclosed", str("chromaticity_shift_projection", "tm_35_edition"), hasChromaticityShiftProjection},
	// flicker_measurements qualifiers.
	{LevelEnrichment, "/flicker_measurements/risk_level", "FlickerRiskLevel", "test_report", "LM-90 / IEEE 1789", "flicker risk level not disclosed", str("flicker_measurements", "risk_level"), hasFlickerMeasurements},
	{LevelEnrichment, "/flicker_measurements/test_chamber_type", "FlickerTestChamberType", "test_report", "LM-90 / IEEE 1789", "flicker test chamber type not disclosed", str("flicker_measurements", "test_chamber_type"), hasFlickerMeasurements},
	{LevelEnrichment, "/flicker_measurements/dimming_type_at_test", "FlickerDimmingType", "test_report", "LM-90 / IEEE 1789", "flicker dimming type at test not disclosed", str("flicker_measurements", "dimming_type_at_test"), hasFlickerMeasurements},
	{LevelEnrichment, "/flicker_measurements/photodetector_correction", "FlickerPhotodetectorSpectralCorrection", "test_report", "LM-90 / IEEE 1789", "flicker photodetector spectral correction not disclosed", str("flicker_measurements", "photodetector_correction"), hasFlickerMeasurements},
	{LevelEnrichment, "/flicker_measurements/sampling_class", "FlickerSamplingClass", "test_report", "LM-90 / IEEE 1789", "flicker sampling class not disclosed", str("flicker_measurements", "sampling_class"), hasFlickerMeasurements},
	{LevelEnrichment, "/flicker_measurements/waveform_file_format", "FlickerWaveformFileFormat", "test_report", "LM-90 / IEEE 1789", "flicker waveform file format not disclosed", str("flicker_measurements", "waveform_file_format"), hasFlickerMeasurements},
	// thermal_derating.
	{LevelEnrichment, "/thermal_derating/thermal_control_method", "ThermalControlMethod", "test_report", "LM-82", "thermal control method not disclosed", str("thermal_derating", "thermal_control_method"), hasThermalDerating},
	{LevelEnrichment, "/thermal_derating/curves[]/temperature_axis", "TemperatureAxis", "test_report", "LM-82", "thermal derating temperature axis not disclosed", arrItemHas([]string{"thermal_derating", "curves"}, "temperature_axis"), hasThermalDerating},
	// electrical.
	{LevelEnrichment, "/electrical/dimming_curve", "DimmingCurve", "datasheet_pdf", "identity", "dimming curve not disclosed", str("electrical", "dimming_curve"), blockPresent("electrical")},

	// Bucket C: the 5 additive optional fields wired in v0.9.0 Phase D.
	{LevelEnrichment, "/product_family/orientation", "Orientation", "datasheet_pdf", "identity", "installed orientation not disclosed", str("product_family", "orientation"), nil},
	{LevelEnrichment, "/photometry/optical_radiation_band", "OpticalRadiationBand", "datasheet_pdf", "LM-80", "optical radiation band not disclosed", str("photometry", "optical_radiation_band"), blockPresent("photometry")},
	{LevelEnrichment, "/electrical/adaptive_lighting_modes", "AdaptiveLightingMode", "datasheet_pdf", "RP-8", "adaptive lighting modes not disclosed", arr("electrical", "adaptive_lighting_modes"), controllableDriver},
	{LevelEnrichment, "/source_files[]/photometry_format", "PhotometryFormat", "ies", "LM-63 / TM-33", "photometry file format not disclosed", arrItemHas([]string{"source_files"}, "photometry_format"), hasPhotometrySourceFile},
	// pvf_code surfaces the TM-30 design-intent ground (TM30DesignIntent/TM30Level stay
	// staged vocabulary; pvf_code carries the highest-priority achieved designation).
	{LevelEnrichment, "/colorimetry/tm_30/pvf_code", "", "test_report", "TM-30", "TM-30 PVF designation not disclosed", str("colorimetry", "tm_30", "pvf_code"), both(isWhiteLightPrimary, blockPresent("colorimetry", "tm_30"))},

	// v0.10.0 exit-sign & emergency ENRICHMENT rows (§4.5; all non-gating). Powered signs
	// also disclose input voltage; every sign can disclose the UL 924-marked viewing
	// distance; an internally illuminated sign can disclose measured face luminance (UL 924
	// governs internal-sign legibility, so this deepens rather than gates, §2.10a);
	// externally illuminated signs disclose the legend-geometry detail trio; every
	// self-emitting sign discloses the technology within its mode. The three emergency-mode
	// nudges fire only for the two dual-mode roles (§2.9); the battery trio nudges any record
	// whose class does not core-gate power_source (a category-d fixture or an externally
	// illuminated combo sign), while the power-source-core classes gate those at standard.
	{LevelEnrichment, "/electrical/input_voltage_v (or input_voltage_class)", "", "datasheet_pdf", "UL 924", "input voltage not disclosed", anyOf(num("electrical", "input_voltage_v"), str("electrical", "input_voltage_class")), both(isExitSign, signMode("internally_illuminated"))},
	{LevelEnrichment, "/exit_sign/rated_viewing_distance_ft", "RatedViewingDistance", "datasheet_pdf", "UL 924", "UL 924-marked rated viewing distance not disclosed", str("exit_sign", "rated_viewing_distance_ft"), isExitSign},
	{LevelEnrichment, "/exit_sign/sign_face_luminance_cd_per_m2", "", "datasheet_pdf", "UL 924", "measured sign-face luminance not disclosed", num("exit_sign", "sign_face_luminance_cd_per_m2"), both(isExitSign, signMode("internally_illuminated"))},
	{LevelEnrichment, "/exit_sign/stroke_width", "", "datasheet_pdf", "IBC 1013.6.1", "legend stroke width not disclosed", scalarNum("exit_sign", "stroke_width", "mm"), both(isExitSign, signMode("externally_illuminated"))},
	{LevelEnrichment, "/exit_sign/letter_width", "", "datasheet_pdf", "IBC 1013.6.1", "legend letter width not disclosed", scalarNum("exit_sign", "letter_width", "mm"), both(isExitSign, signMode("externally_illuminated"))},
	{LevelEnrichment, "/exit_sign/letter_spacing", "", "datasheet_pdf", "IBC 1013.6.1", "legend letter spacing not disclosed", scalarNum("exit_sign", "letter_spacing", "mm"), both(isExitSign, signMode("externally_illuminated"))},
	{LevelEnrichment, "/exit_sign/illumination_technology", "ExitSignIlluminationTechnology", "datasheet_pdf", "UL 924", "illumination technology not disclosed", str("exit_sign", "illumination_technology"), selfEmittingSign},
	{LevelEnrichment, "/emergency/emergency_lumen_output_lm", "", "datasheet_pdf", "UL 924", "distinct emergency-mode lumen output not disclosed", num("emergency", "emergency_lumen_output_lm"), hasEmergencyModeRole},
	{LevelEnrichment, "/emergency/photometry_reference", "", "ies", "UL 924", "emergency-mode photometry file not attached", hasEmergencyPhotometryReference, hasEmergencyModeRole},
	{LevelEnrichment, "/emergency/emergency_input_power_w", "", "datasheet_pdf", "UL 924", "distinct emergency-mode input power not disclosed", num("emergency", "emergency_input_power_w"), hasEmergencyModeRole},
	{LevelEnrichment, "/emergency/battery_duration_min", "", "datasheet_pdf", "UL 924", "battery run time not disclosed", num("emergency", "battery_duration_min"), batteryNudgeClass},
	{LevelEnrichment, "/emergency/battery_chemistry", "BatteryChemistry", "datasheet_pdf", "UL 924", "battery chemistry not disclosed", str("emergency", "battery_chemistry"), batteryNudgeClass},
	{LevelEnrichment, "/emergency/self_test", "EmergencySelfTestCapability", "datasheet_pdf", "UL 924", "self-test capability not disclosed", str("emergency", "self_test"), batteryNudgeClass},

	// --- OBSERVATIONS (non-gating; residual notes; surfaced at core and above) ---
	// A tracked declaration whose presence is noted, and a deprecated classification.
	// Neither is a "disclose Y to deepen the datasheet" nudge, so neither joins the
	// enrichment roadmap. The two note emitters (emitHeadlineProvenance,
	// emitAttestationCoverage) are the other two residual observation sources.
	{LevelObservation, "/sustainability_declaration", "", "datasheet_pdf", "EPD / HPD", "sustainability declaration not disclosed", hasSustainabilityDeclaration, nil},
	{LevelObservation, "/outdoor_classification/legacy_cutoff", "LegacyCutoffClassification", "ies", "RP-8", "legacy cutoff classification not disclosed", str("outdoor_classification", "legacy_cutoff"), outdoorSite},
}

// --- the ladder ---

// AchievedLevel returns the honest grade the record reaches: the highest grade all
// of whose hard requirements (conditional predicates applied) are met, walking
// core -> standard -> full and stopping at the first unmet grade. The floor of the
// walk is LevelIncomplete (returned when core is not yet met), the zero value: the
// grader never refuses and never returns a below-floor sentinel. A record missing
// identity (manufacturer_slug, catalog_model) is schema-invalid and is caught
// upstream; builder.go also reports it via MissingRequiredKeys.
// index.conformance_level == AchievedLevel().String().
func AchievedLevel(record map[string]any) Level {
	if !allRulesMet(record, LevelCore) {
		return LevelIncomplete
	}
	if !allRulesMet(record, LevelStandard) {
		return LevelCore
	}
	if !allRulesMet(record, LevelFull) {
		return LevelStandard
	}
	return LevelFull
}

// allRulesMet reports whether every applicable hard rule at lvl is present.
func allRulesMet(record map[string]any, lvl Level) bool {
	for _, ru := range rubric {
		if ru.level != lvl {
			continue
		}
		if ru.applicable != nil && !ru.applicable(record) {
			continue
		}
		if !ru.present(record) {
			return false
		}
	}
	return true
}

// missingAt returns the applicable-but-absent hard rules at lvl, in path order,
// for deterministic roadmap output.
func missingAt(record map[string]any, lvl Level) []rule {
	out := []rule{}
	for _, ru := range rubric {
		if ru.level != lvl {
			continue
		}
		if ru.applicable != nil && !ru.applicable(record) {
			continue
		}
		if !ru.present(record) {
			out = append(out, ru)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out
}

// --- the structured result ---

// Item is one row of a Result: a tier-roadmap gap, an enrichment suggestion, or an
// observation note. Document is a SourceFileType token; Standard is the governing
// standard; Message is the human-readable line. NextLevel is meaningful ONLY on
// TierRoadmap items (the tier the gap unlocks) and is the zero value elsewhere.
type Item struct {
	Path      string
	Document  string
	Standard  string
	Message   string
	NextLevel Level
}

// Result is the structured completeness picture Compute returns: the achieved
// level plus the two-part roadmap (tier gaps and enrichment) and the observation
// notes. A future achievements package models on this shape by copying, not
// importing.
type Result struct {
	Level       Level
	TierRoadmap []Item // per-tier hard-rule gaps (render emits as conformance/gap)
	Enrichment  []Item // LevelEnrichment applicable-and-absent (render emits as conformance/enrichment)
	// Observations carries the LevelObservation rubric rows ONLY (post-reclassification:
	// the sustainability and legacy-cutoff notes). The two direct note emitters
	// (non-measured headline, attestation coverage) are NOT rubric rows and are NOT
	// included here; render appends them separately inside the same core gate. Do not
	// treat Observations as the full observation set a validate run emits.
	Observations []Item
}

// Compute walks the rubric once and returns the full completeness picture. It is
// pure and unconditional (no core gate): downstream consumers get every slice even
// for an incomplete record. Level is set from AchievedLevel, the independent, cheap
// level ladder the index builder calls, so Compute does not duplicate the ladder.
// render applies the core gate for the human-facing emission.
func Compute(record map[string]any) Result {
	res := Result{Level: AchievedLevel(record)}
	// Tier roadmap: for each outstanding tier, its applicable-but-absent hard rules.
	for _, tier := range []Level{LevelCore, LevelStandard, LevelFull} {
		if tier <= res.Level {
			continue
		}
		for _, ru := range missingAt(record, tier) {
			res.TierRoadmap = append(res.TierRoadmap, Item{
				Path: ru.path, Document: ru.document, Standard: ru.standard,
				Message:   roadmapMessage(tier, ru),
				NextLevel: tier,
			})
		}
	}
	res.Enrichment = collectSentinel(record, LevelEnrichment)
	res.Observations = collectSentinel(record, LevelObservation)
	return res
}

// collectSentinel returns the applicable-but-absent rows at a non-gating sentinel
// level (LevelEnrichment or LevelObservation), in rubric order, as Items carrying
// each row's message / document / standard.
func collectSentinel(record map[string]any, level Level) []Item {
	out := []Item{}
	for _, ru := range rubric {
		if ru.level != level {
			continue
		}
		if ru.applicable != nil && !ru.applicable(record) {
			continue
		}
		if ru.present(record) {
			continue
		}
		out = append(out, Item{Path: ru.path, Document: ru.document, Standard: ru.standard, Message: ru.message})
	}
	return out
}

// --- reporting ---

// Report appends the conformance INFO findings to report and returns the achieved
// grade. It is the human-facing half of grading: it computes the structured Result
// and renders it. The stored index.conformance_level is already verified by the
// builder-parity step, so this explains what the record achieved and how to climb.
//
// Report emits no WARNINGs and no ERRORs: there is no declaration to violate.
func Report(record map[string]any, report *findings.Report) Level {
	res := Compute(record)
	render(res, record, report)
	return res.Level
}

// noBlockerSeen is the "no outstanding gap seen yet" sentinel for render's blocker
// tracking. Its String() is never emitted, because the first outstanding tier always
// carries a real delta and reassigns blocker before any gated grade is rendered. It is
// LevelIncomplete (the floor) so that if a future refactor ever broke that invariant the
// fallback would render as the least-surprising "incomplete" rather than leaking a
// non-gating band token into a grade-unlock message.
const noBlockerSeen = LevelIncomplete

// render reproduces the human-facing INFO emission from a computed Result: the
// achieved-grade summary; then, for each of the three grades, one of three per-grade
// states; and, once the record is a real core record, the enrichment roadmap, the
// observation notes, and the two direct note emitters.
func render(res Result, record map[string]any, report *findings.Report) {
	achieved := res.Level

	report.AddInfo(findings.CodeConformanceLevel, "/index/conformance_level",
		fmt.Sprintf("this record achieves conformance level %q", achieved.String()))

	// Per-grade emission, walking core -> standard -> full. missingAt is exact-tier,
	// so each grade lists only its own delta (no repeats). Three states per grade:
	//   1. satisfied        the grade is at or below the achieved grade.
	//   2. roadmap delta    the grade is outstanding and has missing rows.
	//   3. gated            the grade is outstanding but its OWN rows are all met; it
	//                       is gated only by a lower grade. This is the cascade signal
	//                       (close the lower grade and this one unlocks immediately).
	// blocker tracks the HIGHEST outstanding grade that still has a real delta, so a
	// gated grade can name what to reach to unlock it. Reaching that grade implies
	// every lower grade is met too, whereas reaching a lower grade alone is not enough
	// when an intermediate grade still has a gap. The ascending walk updates blocker on
	// every roadmap grade, so it holds the last delta grade below any gated grade.
	blocker := noBlockerSeen // no outstanding gap seen yet
	for _, tier := range []Level{LevelCore, LevelStandard, LevelFull} {
		if tier <= achieved {
			report.AddInfo(findings.CodeConformanceGradeSatisfied, "/index/conformance_level",
				fmt.Sprintf("conformance grade %q satisfied", tier.String()))
			continue
		}
		delta := missingAt(record, tier)
		if len(delta) == 0 {
			// blocker is the highest outstanding grade below this one that still has a
			// delta; reaching it (and thus every lower grade) unlocks this grade, whose
			// own rows are already met. It is always set: the grade just above `achieved`
			// has a non-empty delta and is walked before any gated grade above it.
			report.AddInfo(findings.CodeConformanceGradeGated, "/index/conformance_level",
				fmt.Sprintf("conformance grade %q requirements are met; unlocked once %q is reached", tier.String(), blocker.String()))
			continue
		}
		// Update on every roadmap grade so blocker holds the highest delta grade.
		blocker = tier
		emitRoadmap(tier, delta, report)
	}

	// The enrichment roadmap, the observation notes, and the two direct note emitters,
	// all behind a SINGLE core gate (an incomplete record's priority is reaching core).
	// The note emitters MUST sit inside this same gate: split them out and an incomplete
	// record would leak conformance/observation findings.
	if achieved >= LevelCore {
		emitEnrichment(res.Enrichment, report)
		emitObservations(res.Observations, report)
		emitHeadlineProvenance(record, report)
		emitAttestationCoverage(record, report)
	}
}

// roadmapMessage is the single source of truth for a tier-roadmap gap's human-readable
// message, used by BOTH Compute (building Result.TierRoadmap) and emitRoadmap (emitting
// the conformance/gap finding), so the structured Result and the emitted finding can
// never drift on wording.
func roadmapMessage(next Level, ru rule) string {
	return fmt.Sprintf("to reach %q, add %s (from %s, per %s)", next.String(), ru.path, ru.document, ru.standard)
}

// emitRoadmap records one CodeConformanceGap INFO per missing rule, each carrying
// the structured next-level / source-document / standard detail.
func emitRoadmap(next Level, missing []rule, report *findings.Report) {
	for _, ru := range missing {
		report.AddRoadmap(findings.CodeConformanceGap, ru.path, next.String(), ru.document, ru.standard,
			roadmapMessage(next, ru))
	}
}

// emitEnrichment emits one CodeConformanceEnrichment INFO per enrichment Item: the
// non-gating "also disclose Y to deepen the datasheet" roadmap.
func emitEnrichment(items []Item, report *findings.Report) {
	for _, it := range items {
		report.AddEnrichment(findings.CodeConformanceEnrichment, it.Path, it.Document, it.Standard, it.Message)
	}
}

// emitObservations emits one CodeConformanceObservation INFO per observation Item:
// the residual data-depth notes the rubric tracks but does not put on the enrichment
// roadmap (the sustainability declaration, the deprecated legacy-cutoff classification).
func emitObservations(items []Item, report *findings.Report) {
	for _, it := range items {
		report.Add(findings.Finding{
			Level:          findings.LevelInfo,
			Code:           findings.CodeConformanceObservation,
			Path:           it.Path,
			Message:        it.Message,
			SourceDocument: it.Document,
			Standard:       it.Standard,
		})
	}
}

// emitHeadlineProvenance surfaces the non-measured-headline note: a record whose
// directly-measured headline photometry (total_luminous_flux_lm / input_power_w /
// maximum_intensity_cd) carries a value_type other than "measured" (for example
// "rated" or an optical simulation). luminaire_efficacy_lm_per_w is excluded: it
// is a derived quantity (flux / power), commonly published as a rounded rated
// figure even on fully measured fixtures, so flagging it would be noise.
func emitHeadlineProvenance(record map[string]any, report *findings.Report) {
	type spec struct {
		field, parent, key string
	}
	for _, s := range []spec{
		{"photometry.total_luminous_flux_lm", "photometry", "total_luminous_flux_lm"},
		{"electrical.input_power_w", "electrical", "input_power_w"},
		{"photometry.maximum_intensity_cd", "photometry", "maximum_intensity_cd"},
	} {
		pn, ok := getMap(record, s.parent, s.key)
		if !ok {
			continue
		}
		vt, _ := pn["value_type"].(string)
		if vt != "" && vt != "measured" {
			report.Add(findings.Finding{
				Level:   findings.LevelInfo,
				Code:    findings.CodeConformanceObservation,
				Path:    "/" + s.parent + "/" + s.key + "/value_type",
				Message: fmt.Sprintf("headline photometric value %s carries value_type %q rather than \"measured\"", s.field, vt),
			})
		}
	}
}

// emitAttestationCoverage emits one observation listing the compliance programs
// tracked on the record (sorted), or noting none are present.
func emitAttestationCoverage(record map[string]any, report *findings.Report) {
	progs := attestationPrograms(record)
	if len(progs) == 0 {
		// Defensive: render calls this only inside the core gate, and reaching core
		// implies a market-safety listing (so progs is non-empty at that call site).
		// Kept so the helper stays correct if it is ever called outside that gate.
		report.AddInfo(findings.CodeConformanceObservation, "/attestations",
			"no compliance or certification programs are listed on this record")
		return
	}
	sorted := append([]string(nil), progs...)
	sort.Strings(sorted)
	report.AddInfo(findings.CodeConformanceObservation, "/attestations",
		fmt.Sprintf("compliance programs tracked on this record: %s", strings.Join(sorted, ", ")))
}

// --- reused accessors and domain helpers (all comma-ok guarded) ---

// hasLM79Attestation reports whether any attestation entry has a program in the
// LM-79 family (program token starts with "lm_79"). Checks both record-level
// attestations and product_family.shared_attestations, matching where the index
// builder collects programs.
func hasLM79Attestation(record map[string]any) bool {
	for _, p := range attestationPrograms(record) {
		if strings.HasPrefix(p, "lm_79") {
			return true
		}
	}
	return false
}

// attestationPrograms collects program tokens from both the top-level
// attestations[] and product_family.shared_attestations[], so a listing authored
// under either is read.
func attestationPrograms(record map[string]any) []string {
	out := []string{}
	collect := func(arr []any) {
		for _, a := range arr {
			if m, ok := a.(map[string]any); ok {
				if p, ok := m["program"].(string); ok && p != "" {
					out = append(out, p)
				}
			}
		}
	}
	if arr, ok := record["attestations"].([]any); ok {
		collect(arr)
	}
	if pf, ok := record["product_family"].(map[string]any); ok {
		if arr, ok := pf["shared_attestations"].([]any); ok {
			collect(arr)
		}
	}
	return out
}

// hasLumenMaintenance reports whether either lumen-maintenance framework is present
// with recognized content of the correct leaf type. A luminaire framework qualifies
// when one of its sub-blocks (tm_28, cie_97_lmf_table, manufacturer_rated_claim)
// carries a recognized populated field; a package qualifies when at least one entry
// carries a recognized field. A bare manufacturer_rated_claim still satisfies the gate
// (a claim_type enum or a numeric claimed_hours is enough, not a published-method
// requirement), but an empty object, one holding only unrecognized keys, or one whose
// recognized keys carry the wrong leaf type does not.
func hasLumenMaintenance(record map[string]any) bool {
	return hasLumenMaintenanceLuminaire(record) || hasLumenMaintenancePackage(record)
}

// hasLumenMaintenanceLuminaire reports whether lumen_maintenance_luminaire carries at
// least one recognized framework sub-block populated with real content of the correct
// leaf type. manufacturer_rated_claim qualifies on a non-empty claim_type enum or a
// numeric claimed_hours (ProvenancedNumber); tm_28 on a numeric tm_28_projection_hours;
// cie_97_lmf_table on a non-empty lmf_by_cleanliness_and_interval or llmf_by_hours
// array. A sub-block holding only unrecognized keys, or recognized keys of the wrong
// type, does not satisfy the gate.
func hasLumenMaintenanceLuminaire(record map[string]any) bool {
	lml, ok := getMap(record, "lumen_maintenance_luminaire")
	if !ok {
		return false
	}
	if claim, ok := getMap(lml, "manufacturer_rated_claim"); ok {
		if getString(claim, "claim_type") != "" || hasNumberValue(claim, "claimed_hours") {
			return true
		}
	}
	if hasNumberValue(lml, "tm_28", "tm_28_projection_hours") {
		return true
	}
	if cie, ok := getMap(lml, "cie_97_lmf_table"); ok {
		for _, k := range []string{"lmf_by_cleanliness_and_interval", "llmf_by_hours"} {
			if a, ok := cie[k].([]any); ok && len(a) > 0 {
				return true
			}
		}
	}
	return false
}

// hasLumenMaintenancePackage reports whether a lumen_maintenance_package array has at
// least one entry carrying a recognized LumenMaintenancePackageEntry field populated
// with real content of the correct leaf type. The entry schema has no required fields,
// so an empty {} entry, or one carrying recognized keys of the wrong type (a key with
// an empty string where an enum is meant, or a value-less object where a
// ProvenancedNumber is meant), must not satisfy the gate. The four ProvenancedNumber
// fields (tm_21_projection_hours, test_hours, test_temperature_c, drive_current_ma)
// qualify via hasNumberValue; the six string/enum fields qualify via a non-empty string.
func hasLumenMaintenancePackage(record map[string]any) bool {
	arr, ok := record["lumen_maintenance_package"].([]any)
	if !ok {
		return false
	}
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		for _, k := range []string{
			"tm_21_projection_hours", "test_hours", "test_temperature_c", "drive_current_ma",
		} {
			if hasNumberValue(m, k) {
				return true
			}
		}
		for _, k := range []string{
			"package_identifier", "tested_product_type", "flux_maintenance_quantity",
			"flux_maintenance_threshold", "projection_reliability", "tm_21_interpolation_type",
		} {
			if getString(m, k) != "" {
				return true
			}
		}
	}
	return false
}

// hasMethodBackedLumenMaintenance reports whether the record carries a
// method-backed lumen-maintenance projection: a package entry with a
// tm_21_projection_hours value, or a luminaire framework with a tm_28 projection.
// A bare manufacturer_rated_claim does not count.
func hasMethodBackedLumenMaintenance(record map[string]any) bool {
	if arr, ok := record["lumen_maintenance_package"].([]any); ok {
		for _, e := range arr {
			if m, ok := e.(map[string]any); ok {
				if hasNumberValue(m, "tm_21_projection_hours") {
					return true
				}
			}
		}
	}
	if lml, ok := record["lumen_maintenance_luminaire"].(map[string]any); ok {
		if hasNumberValue(lml, "tm_28", "tm_28_projection_hours") {
			return true
		}
	}
	return false
}

// --- observation-row present-closures (non-gating; same real-content discipline) ---
//
// These back the LevelObservation rows that read whole sub-objects. Like the gating
// helpers above they route every access through getMap / comma-ok and require at
// least one recognized field of the correct leaf type, so an empty object, a map
// holding only unrecognized keys, or recognized keys of the wrong type reads as
// absent and the "... not disclosed" nudge still fires.

// hasAmbientOperatingRange reports whether shared_mechanical.ambient_operating_range
// carries a real bound: a min or max DualUnitTemperature populated with a numeric c or
// f leaf (DualUnitTemperature requires both, but either present as a number signals
// real content). A bound holding only unrecognized keys reads as absent.
func hasAmbientOperatingRange(record map[string]any) bool {
	rng, ok := getMap(record, "product_family", "shared_mechanical", "ambient_operating_range")
	if !ok {
		return false
	}
	for _, b := range []string{"min", "max"} {
		if t, ok := getMap(rng, b); ok && (isNumber(t["c"]) || isNumber(t["f"])) {
			return true
		}
	}
	return false
}

// hasThermalDerating reports whether thermal_derating carries a recognized field of the
// correct type: a non-empty thermal_control_method enum, or a non-empty curves array.
func hasThermalDerating(record map[string]any) bool {
	td, ok := getMap(record, "thermal_derating")
	if !ok {
		return false
	}
	if getString(td, "thermal_control_method") != "" {
		return true
	}
	a, ok := td["curves"].([]any)
	return ok && len(a) > 0
}

// hasFlickerMeasurements reports whether flicker_measurements carries a recognized field
// of the correct type: a non-empty metrics array, or any of the six qualifier enums
// (risk_level, test_chamber_type, dimming_type_at_test, photodetector_correction,
// sampling_class, waveform_file_format) populated with a non-empty string.
func hasFlickerMeasurements(record map[string]any) bool {
	fm, ok := getMap(record, "flicker_measurements")
	if !ok {
		return false
	}
	if a, ok := fm["metrics"].([]any); ok && len(a) > 0 {
		return true
	}
	for _, k := range []string{
		"risk_level", "test_chamber_type", "dimming_type_at_test",
		"photodetector_correction", "sampling_class", "waveform_file_format",
	} {
		if getString(fm, k) != "" {
			return true
		}
	}
	return false
}

// hasAlphaOpicMetrics reports whether alpha_opic_metrics carries a recognized field of
// the correct type: a numeric melanopic_der ProvenancedNumber, a non-empty per_channel
// array, or a populated reference_illuminant / standard_observer const string.
func hasAlphaOpicMetrics(record map[string]any) bool {
	ao, ok := getMap(record, "alpha_opic_metrics")
	if !ok {
		return false
	}
	if hasNumberValue(ao, "melanopic_der") {
		return true
	}
	if a, ok := ao["per_channel"].([]any); ok && len(a) > 0 {
		return true
	}
	for _, k := range []string{"reference_illuminant", "standard_observer"} {
		if getString(ao, k) != "" {
			return true
		}
	}
	return false
}

// hasChromaticityShiftProjection reports whether chromaticity_shift_projection carries a
// recognized field of the correct type: a numeric projected_hours ProvenancedNumber, or
// any of the four enums (shift_metric, shift_threshold, shift_mode, tm_35_edition)
// populated with a non-empty string.
func hasChromaticityShiftProjection(record map[string]any) bool {
	cs, ok := getMap(record, "chromaticity_shift_projection")
	if !ok {
		return false
	}
	if hasNumberValue(cs, "projected_hours") {
		return true
	}
	for _, k := range []string{"shift_metric", "shift_threshold", "shift_mode", "tm_35_edition"} {
		if getString(cs, k) != "" {
			return true
		}
	}
	return false
}

// hasCie97LmfTable reports whether lumen_maintenance_luminaire.cie_97_lmf_table
// carries a populated grid (a non-empty lmf_by_cleanliness_and_interval or
// llmf_by_hours array). It backs the AmbientCleanliness enrichment row as a block-level
// nudge: AmbientCleanliness is a required member of the lmf_by_cleanliness_and_interval
// items, so a field-level row could never be applicable-and-absent on a valid record;
// the block row instead says "you have a luminaire framework; also add the CIE 97 LMF
// table". A hollow table (empty grids, or only unrecognized keys) reads as absent.
func hasCie97LmfTable(record map[string]any) bool {
	cie, ok := getMap(record, "lumen_maintenance_luminaire", "cie_97_lmf_table")
	if !ok {
		return false
	}
	for _, k := range []string{"lmf_by_cleanliness_and_interval", "llmf_by_hours"} {
		if a, ok := cie[k].([]any); ok && len(a) > 0 {
			return true
		}
	}
	return false
}

// hasSustainabilityDeclaration reports whether sustainability_declaration carries a
// recognized field of the correct type. The string / enum / date fields qualify via a
// non-empty string; life_expectancy_years and recyclable_percent are bare JSON numbers;
// end_of_life_options and ingredient_list are arrays; lbc_criteria_compliance is a bool.
// A map holding only unrecognized keys, or recognized keys of the wrong type, reads as
// absent.
func hasSustainabilityDeclaration(record map[string]any) bool {
	sd, ok := getMap(record, "sustainability_declaration")
	if !ok {
		return false
	}
	for _, k := range []string{
		"declaration_type", "document_id", "original_issue_date", "expiration_date",
		"final_assembly_location", "voc_content", "interior_performance", "responsible_sourcing",
	} {
		if getString(sd, k) != "" {
			return true
		}
	}
	for _, k := range []string{"life_expectancy_years", "recyclable_percent"} {
		if isNumber(sd[k]) {
			return true
		}
	}
	for _, k := range []string{"end_of_life_options", "ingredient_list"} {
		if a, ok := sd[k].([]any); ok && len(a) > 0 {
			return true
		}
	}
	if _, isBool := sd["lbc_criteria_compliance"].(bool); isBool {
		return true
	}
	return false
}

// hasEPA reports whether physical_dimensions.epa carries a real area: a numeric m2 or
// ft2 leaf of the DualUnitArea (the schema requires both, but either present as a
// number signals real content). value_type / provenance alone read as absent.
func hasEPA(record map[string]any) bool {
	epa, ok := getMap(record, "product_family", "physical_dimensions", "epa")
	if !ok {
		return false
	}
	return isNumber(epa["m2"]) || isNumber(epa["ft2"])
}

// hasOperatingPoint reports whether operating_point is present with at least one
// recognized qualifier carrying real content of the correct leaf type: a numeric
// ProvenancedNumber for input_voltage_v / input_frequency_hz / drive_current_ma, a
// numeric c or f leaf for the ambient_temperature / case_temperature DualUnit blocks,
// or a non-empty dut_operating_mode string. A map holding only unrecognized keys, or
// a bare scalar where a ProvenancedNumber is required, reads as absent.
func hasOperatingPoint(record map[string]any) bool {
	op, ok := record["operating_point"].(map[string]any)
	if !ok {
		return false
	}
	for _, q := range []string{"input_voltage_v", "input_frequency_hz", "drive_current_ma"} {
		if hasNumberValue(op, q) {
			return true
		}
	}
	for _, q := range []string{"ambient_temperature", "case_temperature"} {
		if t, ok := getMap(op, q); ok && (isNumber(t["c"]) || isNumber(t["f"])) {
			return true
		}
	}
	if s, _ := op["dut_operating_mode"].(string); s != "" {
		return true
	}
	return false
}

// hasInstrumentationDepth reports whether instrumentation carries any field beyond
// measurement_regime that signals lab depth. Each of the five recognized fields is a
// string (enum or free text) in the schema, so a non-empty string is required: a
// non-string value at one of these keys reads as absent.
func hasInstrumentationDepth(record map[string]any) bool {
	instr, ok := record["instrumentation"].(map[string]any)
	if !ok {
		return false
	}
	for _, k := range []string{
		"laboratory_certification", "laboratory_accreditation_scheme",
		"laboratory_name", "laboratory_report_id", "goniometer_type",
	} {
		if s, isStr := instr[k].(string); isStr && s != "" {
			return true
		}
	}
	return false
}

// --- map / value accessors (local, so grade has no dependency beyond findings) ---

// hasNumberValue reports whether the object at the given path is a
// ProvenancedNumber-shaped object carrying a numeric value field.
func hasNumberValue(record map[string]any, keys ...string) bool {
	m, ok := getMap(record, keys...)
	if !ok {
		return false
	}
	v, present := m["value"]
	if !present {
		return false
	}
	switch v.(type) {
	case float64, int, int64:
		return true
	default:
		return false
	}
}

// getMap returns the object at the path and whether it was found as an object.
// With zero keys it returns the record itself.
func getMap(record map[string]any, keys ...string) (map[string]any, bool) {
	node := any(record)
	for _, k := range keys {
		m, ok := node.(map[string]any)
		if !ok {
			return nil, false
		}
		v, present := m[k]
		if !present {
			return nil, false
		}
		node = v
	}
	m, ok := node.(map[string]any)
	return m, ok
}

// getString returns the string at the path, or "" if absent or not a string.
func getString(record map[string]any, keys ...string) string {
	node := any(record)
	for i, k := range keys {
		m, ok := node.(map[string]any)
		if !ok {
			return ""
		}
		v, present := m[k]
		if !present {
			return ""
		}
		if i == len(keys)-1 {
			s, _ := v.(string)
			return s
		}
		node = v
	}
	return ""
}
