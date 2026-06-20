// Package grade implements the ULC conformance-level grading rubric.
//
// Grading is a pure function of an already-parsed record map. It reads only
// fields that are present in the record (no I/O, no external data, no schema
// access) and never mutates the record. The companion JSON Schema validation
// runs first and catches structural defects; grading layers the conformance
// rubric on top.
//
// A record is not assigned a conformance level by hand: it simply IS whatever
// level its populated fields achieve. AchievedLevel is the pure computation the
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
// Four ordered tiers:
//
//	incomplete  a real photometric record (the three anchors flux + power +
//	            primary_category are present, so it is indexable) that has not yet
//	            met every core requirement. It still gets an index and a roadmap.
//	core        a complete, identifiable, legally-sellable luminaire with headline
//	            photometric/electrical numbers, one-line colorimetry, and a market
//	            safety listing.
//	standard    core plus the fuller specification an LM-79 report produces.
//	full        standard plus exhaustive accredited characterization.
//
// LevelNone sits below incomplete for a genuinely non-photometric record (no
// anchors); such a record is normally already rejected by schema validation and
// fails ulc build-index on its required keys regardless. LevelObservation is a
// non-ordered sentinel for the non-gating depth rows in the same table.
//
// Severity model:
//
//	INFO    everything grading emits: the achieved-level summary, the roadmap to
//	        the next tier (each missing item naming its source document and
//	        standard), and, at core and above, the non-gating depth observations.
package grade

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// Level is a conformance tier, ordered none < incomplete < core < standard < full.
type Level int

const (
	// LevelNone: not a photometric record (missing the three anchors). Normally
	// already rejected by schema validation, and fails ulc build-index on the
	// nominal_total_lumens / nominal_input_power_w required keys regardless.
	LevelNone Level = iota
	// LevelIncomplete: a real photometric record (anchors present) that misses a
	// core requirement. It gets an index and a roadmap to core.
	LevelIncomplete
	LevelCore
	LevelStandard
	LevelFull
	// LevelObservation: sentinel for non-gating depth rows. Excluded from
	// AchievedLevel / allRulesMet / missingAt; iterated only by emitObservations.
	LevelObservation
)

// String returns the canonical lowercase conformance_level token.
func (l Level) String() string {
	switch l {
	case LevelIncomplete:
		return "incomplete"
	case LevelCore:
		return "core"
	case LevelStandard:
		return "standard"
	case LevelFull:
		return "full"
	default:
		return "none"
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

// --- applicability predicates (read only core fields; see determinism note) ---

func category(r map[string]any) string { return getString(r, "product_family", "primary_category") }

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
	{LevelCore, "/product_family/primary_category", "PrimaryCategory", "datasheet_pdf", "identity", "", str("product_family", "primary_category"), nil},
	{LevelCore, "/product_family/indoor_outdoor", "IndoorOutdoor", "datasheet_pdf", "identity", "", str("product_family", "indoor_outdoor"), nil},
	{LevelCore, "/product_family/secondary_function", "SecondaryFunction", "datasheet_pdf", "identity", "", arr("product_family", "secondary_function"), nil},
	{LevelCore, "/product_family/mounting_types", "MountingType", "datasheet_pdf", "identity", "", arr("product_family", "mounting_types"), nil},
	{LevelCore, "/product_family/environment_rating", "EnvironmentRating", "datasheet_pdf", "identity", "", str("product_family", "environment_rating"), nil},
	{LevelCore, "/product_family/shape", "Shape", "datasheet_pdf", "identity", "", str("product_family", "shape"), nil},
	{LevelCore, "/product_family/technical_region", "TechnicalRegion", "datasheet_pdf", "identity", "", str("product_family", "technical_region"), nil},
	{LevelCore, "/photometry/distribution_type", "DistributionType", "ies", "LM-79", "", str("photometry", "distribution_type"), nil},
	{LevelCore, "/configuration/tested_axes/color_tunability", "ColorTunabilityCapability", "datasheet_pdf", "identity", "", str("configuration", "tested_axes", "color_tunability"), nil},
	{LevelCore, "/electrical/driver_protocol", "DimmingProtocol", "datasheet_pdf", "identity", "", str("electrical", "driver_protocol"), nil},
	{LevelCore, "/photometry/total_luminous_flux_lm", "", "ies", "LM-79", "", num("photometry", "total_luminous_flux_lm"), nil},
	{LevelCore, "/electrical/input_power_w", "", "ies", "LM-79", "", num("electrical", "input_power_w"), nil},
	{LevelCore, "/photometry/luminaire_efficacy_lm_per_w", "", "ies", "LM-79", "", num("photometry", "luminaire_efficacy_lm_per_w"), nil},
	{LevelCore, "/electrical/input_voltage_v (or input_voltage_class)", "", "datasheet_pdf", "LM-79", "", anyOf(num("electrical", "input_voltage_v"), str("electrical", "input_voltage_class")), nil},
	{LevelCore, "/colorimetry/nominal_cct_k", "NominalCCT", "datasheet_pdf", "ANSI C78.377", "", str("colorimetry", "nominal_cct_k"), hasWhitePoint},
	{LevelCore, "/colorimetry/cri_ra", "", "datasheet_pdf", "CIE 13.3", "", num("colorimetry", "cri_ra"), isWhiteLightPrimary},
	{LevelCore, "safety listing (UL/cUL/ETL/CSA for NA; CE/ENEC/IEC 60598 otherwise)", "AttestationProgram", "compliance_documents", "UL 1598 / IEC 60598", "", hasMarketSafetyListing, nil},

	// --- STANDARD ---
	{LevelStandard, "/photometry/maximum_intensity_cd", "", "ies", "LM-79", "", num("photometry", "maximum_intensity_cd"), nil},
	{LevelStandard, "/photometry/symmetry_type", "SymmetryType", "ies", "LM-75", "", str("photometry", "symmetry_type"), nil},
	{LevelStandard, "/photometry/photometric_coordinate_system", "PhotometricCoordinateSystem", "ies", "LM-75", "", str("photometry", "photometric_coordinate_system"), nil},
	{LevelStandard, "/electrical/control_gear_type", "ControlGearType", "datasheet_pdf", "LM-79", "", str("electrical", "control_gear_type"), nil},
	{LevelStandard, "/product_family/shared_mechanical/housing_material", "HousingMaterial", "datasheet_pdf", "identity", "", str("product_family", "shared_mechanical", "housing_material"), nil},
	{LevelStandard, "/product_family/shared_mechanical/lens_material", "LensMaterial", "datasheet_pdf", "identity", "", str("product_family", "shared_mechanical", "lens_material"), nil},
	{LevelStandard, "/test_conditions/photometry_basis", "PhotometryBasis", "ies", "LM-79", "", str("test_conditions", "photometry_basis"), nil},
	{LevelStandard, "/instrumentation/measurement_regime", "MeasurementRegime", "ies", "LM-79", "", str("instrumentation", "measurement_regime"), nil},
	{LevelStandard, "LM-79 attestation", "AttestationProgram", "test_report", "LM-79", "", hasLM79Attestation, nil},
	{LevelStandard, "/lumen_maintenance_luminaire (or /lumen_maintenance_package)", "", "datasheet_pdf", "TM-21", "", hasLumenMaintenance, nil},
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

	// --- FULL ---
	{LevelFull, "/photometry/zonal_lumens", "", "ies", "LM-79", "", arr("photometry", "zonal_lumens"), nil},
	{LevelFull, "/operating_point", "", "test_report", "LM-79", "", hasOperatingPoint, nil},
	{LevelFull, "/uncertainty", "", "test_report", "LM-79 / GUM", "", hasUncertainty, nil},
	{LevelFull, "/corrections_applied", "", "test_report", "LM-79", "", hasCorrectionsApplied, nil},
	{LevelFull, "instrumentation depth (goniometer/lab)", "", "test_report", "LM-79 / LM-75", "", hasInstrumentationDepth, nil},
	{LevelFull, "method-backed lumen maintenance (TM-21 hours or TM-28)", "", "test_report", "LM-80 / TM-21 / TM-28", "", hasMethodBackedLumenMaintenance, nil},
	{LevelFull, "/colorimetry/tm_30/rf", "", "test_report", "TM-30", "", num("colorimetry", "tm_30", "rf"), isWhiteLightPrimary},
	{LevelFull, "/colorimetry/tm_30/rf_h_per_bin", "", "test_report", "TM-30", "", arr("colorimetry", "tm_30", "rf_h_per_bin"), isWhiteLightPrimary},

	// --- OBSERVATIONS (non-gating; surfaced at core and above) ---
	{LevelObservation, "/electrical/power_factor", "", "datasheet_pdf", "LM-79", "full records commonly disclose power factor; absent here", num("electrical", "power_factor"), nil},
	{LevelObservation, "/product_family/shared_warranty/term_years", "", "datasheet_pdf", "identity", "warranty term not disclosed", scalarNum("product_family", "shared_warranty", "term_years"), nil},
	{LevelObservation, "/photometry/luminous_opening_shape", "LuminousOpeningShape", "ies", "LM-79", "luminous opening shape not disclosed", str("photometry", "luminous_opening_shape"), nil},
	{LevelObservation, "/photometry/emission_face", "EmissionFace", "ies", "LM-79", "emission face not disclosed", str("photometry", "emission_face"), nil},
	{LevelObservation, "/colorimetry/duv", "", "test_report", "ANSI C78.377", "Duv (distance from the Planckian locus) not disclosed", num("colorimetry", "duv"), hasWhitePoint},
	{LevelObservation, "/colorimetry/chromaticity_x", "", "test_report", "ANSI C78.377", "chromaticity x not disclosed", num("colorimetry", "chromaticity_x"), hasWhitePoint},
	{LevelObservation, "/colorimetry/chromaticity_y", "", "test_report", "ANSI C78.377", "chromaticity y not disclosed", num("colorimetry", "chromaticity_y"), hasWhitePoint},
	{LevelObservation, "/product_family/shared_mechanical/ambient_operating_range", "", "datasheet_pdf", "identity", "ambient operating range not disclosed", hasAmbientOperatingRange, nil},
	{LevelObservation, "/compatible_accessories", "AccessoryType", "datasheet_pdf", "identity", "compatible accessories not listed", arr("compatible_accessories"), nil},
	{LevelObservation, "/thermal_derating", "", "test_report", "LM-82", "thermal derating not disclosed", hasThermalDerating, nil},
	{LevelObservation, "/flicker_measurements", "", "test_report", "LM-90 / IEEE 1789", "flicker measurements not disclosed", hasFlickerMeasurements, nil},
	{LevelObservation, "/alpha_opic_metrics", "", "test_report", "CIE S 026", "alpha-opic (circadian) metrics not disclosed", hasAlphaOpicMetrics, nil},
	{LevelObservation, "/chromaticity_shift_projection", "", "test_report", "TM-35", "chromaticity-shift projection not disclosed", hasChromaticityShiftProjection, nil},
	{LevelObservation, "/sustainability_declaration", "", "datasheet_pdf", "EPD / HPD", "sustainability declaration not disclosed", hasSustainabilityDeclaration, nil},
	{LevelObservation, "/photometry/field_angle_deg", "", "ies", "LM-79", "field angle not disclosed", num("photometry", "field_angle_deg"), directional},
	{LevelObservation, "/photometry/cutoff_angle_from_horizontal_deg", "", "ies", "LM-79", "cutoff angle not disclosed", num("photometry", "cutoff_angle_from_horizontal_deg"), nil},
	{LevelObservation, "/photometry/spacing_criterion", "", "ies", "LM-79", "spacing criterion not disclosed", num("photometry", "spacing_criterion"), nil},
	{LevelObservation, "/photometry/ugr_4h_8h", "", "ies", "CIE 117", "UGR not disclosed", num("photometry", "ugr_4h_8h"), nil},
	{LevelObservation, "/outdoor_classification/lcs_zonal_lumens", "", "ies", "TM-15", "LCS zonal lumens not disclosed", arr("outdoor_classification", "lcs_zonal_lumens"), outdoorSite},
	{LevelObservation, "/outdoor_classification/legacy_cutoff", "LegacyCutoffClassification", "ies", "RP-8", "legacy cutoff classification not disclosed", str("outdoor_classification", "legacy_cutoff"), outdoorSite},
	{LevelObservation, "/product_family/shared_mechanical/ik_rating", "", "compliance_documents", "IEC 62262", "impact (IK) rating not disclosed", str("product_family", "shared_mechanical", "ik_rating"), impactPublic},
	{LevelObservation, "/product_family/physical_dimensions/epa", "", "datasheet_pdf", "identity", "EPA (effective projected area) not disclosed", hasEPA, poleMounted},
	// Provenance: the non-measured-headline note fires from emitHeadlineProvenance
	// (a present-but-not-measured check), not a present-closure row.
}

// --- the ladder ---

// AchievedLevel returns the honest tier the record reaches: the highest tier all
// of whose hard requirements (conditional predicates applied) are met, walking
// incomplete -> core -> standard -> full and stopping at the first unmet tier.
// A record without the photometric anchors is LevelNone (not a photometric
// record); a record with the anchors but missing a core requirement is
// LevelIncomplete. index.conformance_level == AchievedLevel().String().
func AchievedLevel(record map[string]any) Level {
	if !hasPhotometricAnchors(record) {
		return LevelNone
	}
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

// hasPhotometricAnchors: the minimum that makes a record a gradeable photometric
// record (it reaches at least LevelIncomplete). The same three fields gate
// index.conformance_level emission in builder.go. They make the record gradeable,
// not fully index-valid: a complete index also needs the identity core-fields
// (manufacturer_slug, catalog_model), which builder.go reports via MissingRequiredKeys.
func hasPhotometricAnchors(record map[string]any) bool {
	return hasNumberValue(record, "photometry", "total_luminous_flux_lm") &&
		hasNumberValue(record, "electrical", "input_power_w") &&
		getString(record, "product_family", "primary_category") != ""
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

// --- reporting ---

// Report appends the conformance INFO findings to report and returns the achieved
// level. It is the human-facing half of grading: the stored index.conformance_level
// is already verified by the builder-parity step, so this explains what the record
// achieved and how to climb. It emits the achieved-level summary; a roadmap to the
// next tier (each missing item naming its source document and standard); and, once
// the record is a real core record, the non-gating depth observations, the
// non-measured-headline provenance note, and the attestation-coverage note.
//
// Report emits no WARNINGs and no ERRORs: there is no declaration to violate.
func Report(record map[string]any, report *findings.Report) Level {
	achieved := AchievedLevel(record)

	// LevelNone is not a ConformanceLevel enum token (its String() is the sentinel
	// "none"), so present it as the absence of an indexable photometric record rather
	// than as a conformance level. Every ordered tier (incomplete and above) names its
	// level token.
	if achieved == LevelNone {
		report.AddInfo(findings.CodeConformanceLevel, "/index/conformance_level",
			"this record lacks the photometric anchors (total_luminous_flux_lm, input_power_w, primary_category), so it is not an indexable photometric record and achieves no conformance level")
	} else {
		report.AddInfo(findings.CodeConformanceLevel, "/index/conformance_level",
			fmt.Sprintf("this record achieves conformance level %q", achieved.String()))
	}

	// Roadmap to the next tier (each missing item names its document + standard).
	switch achieved {
	case LevelIncomplete:
		emitRoadmap(LevelCore, missingAt(record, LevelCore), report)
	case LevelCore:
		emitRoadmap(LevelStandard, missingAt(record, LevelStandard), report)
	case LevelStandard:
		emitRoadmap(LevelFull, missingAt(record, LevelFull), report)
	}

	// Depth observations and attestation coverage, only once the record is a real
	// core record (an incomplete record's priority is reaching core).
	if achieved >= LevelCore {
		emitObservations(record, report)
		emitHeadlineProvenance(record, report)
		emitAttestationCoverage(record, report)
	}

	return achieved
}

// emitRoadmap records one CodeConformanceGap INFO per missing rule, each carrying
// the structured next-level / source-document / standard detail.
func emitRoadmap(next Level, missing []rule, report *findings.Report) {
	for _, ru := range missing {
		report.AddRoadmap(findings.CodeConformanceGap, ru.path, next.String(), ru.document, ru.standard,
			fmt.Sprintf("to reach %q, add %s (from %s, per %s)", next.String(), ru.path, ru.document, ru.standard))
	}
}

// emitObservations iterates the LevelObservation rows: each applicable-but-absent
// row emits a CodeConformanceObservation carrying the row's message, document, and
// standard. These are non-gating "comprehensive data this record could carry" nudges.
func emitObservations(record map[string]any, report *findings.Report) {
	for _, ru := range rubric {
		if ru.level != LevelObservation {
			continue
		}
		if ru.applicable != nil && !ru.applicable(record) {
			continue
		}
		if ru.present(record) {
			continue
		}
		report.Add(findings.Finding{
			Level:          findings.LevelInfo,
			Code:           findings.CodeConformanceObservation,
			Path:           ru.path,
			Message:        ru.message,
			SourceDocument: ru.document,
			Standard:       ru.standard,
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
