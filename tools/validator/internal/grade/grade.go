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
func obj(keys ...string) predicate { return func(r map[string]any) bool { return hasMap(r, keys...) } }

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

// hasUncertainty: coverage_factor_k present plus at least one expanded_uncertainty_*,
// on the guarded accessor.
func hasUncertainty(r map[string]any) bool {
	m, ok := getMap(r, "uncertainty")
	if !ok {
		return false
	}
	if _, ok := m["coverage_factor_k"]; !ok {
		return false
	}
	for _, k := range []string{
		"expanded_uncertainty_total_flux_percent", "expanded_uncertainty_input_power_percent",
		"expanded_uncertainty_cct_k", "expanded_uncertainty_efficacy_percent",
	} {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
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
		if s, ok := m.(string); ok {
			if strings.Contains(s, "pole") || strings.Contains(s, "mast") {
				return true
			}
		}
	}
	return false
}

// Safety-listing acceptance. Only the TechnicalRegion token
// 120v_60hz_north_america contains "north_america"; the other three
// (230v_50hz_europe, 100v_50_60hz_japan, universal) route to anySafetyListings.
var naSafetyListings = map[string]bool{
	"ul_listed": true, "c_ul_listed": true, "etl": true, "csa_listed": true,
	"met_listed": true, "nrtl_osha_recognized": true, "ul_1598": true,
}
var anySafetyListings = map[string]bool{
	"ul_listed": true, "c_ul_listed": true, "etl": true, "csa_listed": true,
	"nrtl_osha_recognized": true, "ul_1598": true, "tuv": true, "cb_scheme": true,
	"ce": true, "ukca": true, "enec": true, "iec_60598": true, "nom": true, "ccc": true,
	"rcm_australia": true, "saa_australia": true, "kc_korea": true, "pse_japan": true,
}

// hasMarketSafetyListing checks for the PRESENCE OF A CLAIM, not third-party
// verification. A self-asserted listing token satisfies the core gate; see
// docs/compliance-attestation.md. North American records require an NA-recognized
// listing; everywhere else any recognized listing (including IEC 60598 / CE)
// satisfies the gate.
func hasMarketSafetyListing(r map[string]any) bool {
	accept := anySafetyListings
	if strings.Contains(getString(r, "product_family", "technical_region"), "north_america") {
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
	{LevelStandard, "/photometry/per_length_normalized", "", "datasheet_pdf", "LM-79", "", obj("photometry", "per_length_normalized"), linear},
	{LevelStandard, "/photometry/declared_by_length", "", "datasheet_pdf", "LM-79", "", arr("photometry", "declared_by_length"), linear},
	{LevelStandard, "/configuration/tested_axes/cri_tier", "CriTier", "datasheet_pdf", "CIE 13.3", "", str("configuration", "tested_axes", "cri_tier"), isWhiteLightPrimary},
	{LevelStandard, "/colorimetry/sdcm_step", "", "datasheet_pdf", "ANSI C78.377", "", num("colorimetry", "sdcm_step"), isWhiteLightPrimary},
	{LevelStandard, "/electrical/dimming_method", "DimmingMethod", "datasheet_pdf", "identity", "", str("electrical", "dimming_method"), requiresDimmingDetail},
	{LevelStandard, "/electrical/dimming_range_percent", "", "datasheet_pdf", "identity", "", obj("electrical", "dimming_range_percent"), requiresDimmingDetail},
	{LevelStandard, "/product_family/shared_mechanical/ip_rating", "", "compliance_documents", "IEC 60529", "", str("product_family", "shared_mechanical", "ip_rating"), wetOrExposed},
	{LevelStandard, "/outdoor_classification/outdoor_distribution_type", "OutdoorDistributionType", "ies", "RP-8", "", str("outdoor_classification", "outdoor_distribution_type"), outdoorSite},
	{LevelStandard, "/outdoor_classification/longitudinal_distribution_range", "LongitudinalDistributionRange", "ies", "RP-8", "", str("outdoor_classification", "longitudinal_distribution_range"), outdoorSite},
	{LevelStandard, "/outdoor_classification/bug_rating", "", "datasheet_pdf", "TM-15", "", obj("outdoor_classification", "bug_rating"), outdoorSite},

	// --- FULL ---
	{LevelFull, "/photometry/zonal_lumens", "", "ies", "LM-79", "", arr("photometry", "zonal_lumens"), nil},
	{LevelFull, "/operating_point", "", "test_report", "LM-79", "", hasOperatingPoint, nil},
	{LevelFull, "/uncertainty", "", "test_report", "LM-79 / GUM", "", hasUncertainty, nil},
	{LevelFull, "/corrections_applied", "", "test_report", "LM-79", "", obj("corrections_applied"), nil},
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
	{LevelObservation, "/product_family/shared_mechanical/ambient_operating_range", "", "datasheet_pdf", "identity", "ambient operating range not disclosed", obj("product_family", "shared_mechanical", "ambient_operating_range"), nil},
	{LevelObservation, "/compatible_accessories", "AccessoryType", "datasheet_pdf", "identity", "compatible accessories not listed", arr("compatible_accessories"), nil},
	{LevelObservation, "/thermal_derating", "", "test_report", "LM-82", "thermal derating not disclosed", obj("thermal_derating"), nil},
	{LevelObservation, "/flicker_measurements", "", "test_report", "LM-90 / IEEE 1789", "flicker measurements not disclosed", obj("flicker_measurements"), nil},
	{LevelObservation, "/alpha_opic_metrics", "", "test_report", "CIE S 026", "alpha-opic (circadian) metrics not disclosed", obj("alpha_opic_metrics"), nil},
	{LevelObservation, "/chromaticity_shift_projection", "", "test_report", "TM-35", "chromaticity-shift projection not disclosed", obj("chromaticity_shift_projection"), nil},
	{LevelObservation, "/sustainability_declaration", "", "datasheet_pdf", "EPD / HPD", "sustainability declaration not disclosed", obj("sustainability_declaration"), nil},
	{LevelObservation, "/photometry/field_angle_deg", "", "ies", "LM-79", "field angle not disclosed", num("photometry", "field_angle_deg"), directional},
	{LevelObservation, "/photometry/cutoff_angle_from_horizontal_deg", "", "ies", "LM-79", "cutoff angle not disclosed", num("photometry", "cutoff_angle_from_horizontal_deg"), nil},
	{LevelObservation, "/photometry/spacing_criterion", "", "ies", "LM-79", "spacing criterion not disclosed", num("photometry", "spacing_criterion"), nil},
	{LevelObservation, "/photometry/ugr_4h_8h", "", "ies", "CIE 117", "UGR not disclosed", num("photometry", "ugr_4h_8h"), nil},
	{LevelObservation, "/outdoor_classification/lcs_zonal_lumens", "", "ies", "TM-15", "LCS zonal lumens not disclosed", arr("outdoor_classification", "lcs_zonal_lumens"), outdoorSite},
	{LevelObservation, "/outdoor_classification/legacy_cutoff", "LegacyCutoffClassification", "ies", "RP-8", "legacy cutoff classification not disclosed", str("outdoor_classification", "legacy_cutoff"), outdoorSite},
	{LevelObservation, "/product_family/shared_mechanical/ik_rating", "", "compliance_documents", "IEC 62262", "impact (IK) rating not disclosed", str("product_family", "shared_mechanical", "ik_rating"), impactPublic},
	{LevelObservation, "/product_family/physical_dimensions/epa", "", "datasheet_pdf", "identity", "EPA (effective projected area) not disclosed", obj("product_family", "physical_dimensions", "epa"), poleMounted},
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

// hasPhotometricAnchors: the minimum that makes a record an indexable photometric
// record. The same three fields gate index.conformance_level emission in builder.go.
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

	report.AddInfo(findings.CodeConformanceLevel, "/index/conformance_level",
		fmt.Sprintf("this record achieves conformance level %q", achieved.String()))

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

// hasLumenMaintenance reports whether either lumen-maintenance framework is
// present. Any framework satisfies the standard hard gate, including a bare
// lumen_maintenance_luminaire that carries only a manufacturer_rated_claim (block
// presence, not a published-hours requirement).
func hasLumenMaintenance(record map[string]any) bool {
	return hasMap(record, "lumen_maintenance_luminaire") || hasLumenMaintenancePackage(record)
}

// hasLumenMaintenancePackage reports whether a non-empty lumen_maintenance_package
// array is present.
func hasLumenMaintenancePackage(record map[string]any) bool {
	arr, ok := record["lumen_maintenance_package"].([]any)
	return ok && len(arr) > 0
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

// hasOperatingPoint reports whether operating_point is present with at least one
// of the recognized qualifiers carrying real content.
func hasOperatingPoint(record map[string]any) bool {
	op, ok := record["operating_point"].(map[string]any)
	if !ok {
		return false
	}
	for _, q := range []string{
		"input_voltage_v", "input_frequency_hz", "drive_current_ma",
		"ambient_temperature", "case_temperature", "dut_operating_mode",
	} {
		v, present := op[q]
		if !present {
			continue
		}
		switch vv := v.(type) {
		case string:
			if vv != "" {
				return true
			}
		case map[string]any:
			if len(vv) > 0 {
				return true
			}
		}
	}
	return false
}

// hasInstrumentationDepth reports whether instrumentation carries any field beyond
// measurement_regime that signals lab depth.
func hasInstrumentationDepth(record map[string]any) bool {
	instr, ok := record["instrumentation"].(map[string]any)
	if !ok {
		return false
	}
	for _, k := range []string{
		"laboratory_certification", "laboratory_accreditation_scheme",
		"laboratory_name", "laboratory_report_id", "goniometer_type",
	} {
		if v, present := instr[k]; present {
			if s, isStr := v.(string); !isStr || s != "" {
				return true
			}
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

// hasMap reports whether the value at the path is a non-empty object.
func hasMap(record map[string]any, keys ...string) bool {
	m, ok := getMap(record, keys...)
	return ok && m != nil && len(m) > 0
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
