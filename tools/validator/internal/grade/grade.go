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
// Severity model:
//
//	INFO    everything grading emits: the achieved-level summary, the gap
//	        guidance toward the next level up (when below full), and the
//	        full-level observations for comprehensive items that are absent.
//
// Levels are cumulative: standard includes core, full includes standard. The
// achieved level is the highest level whose hard requirements (conditional
// predicates applied) are all met, walking core then standard then full and
// stopping at the first level with an unmet hard requirement.
package grade

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ulcspec/ULC/tools/validator/internal/findings"
)

// Level is a conformance tier, ordered core < standard < full.
type Level int

const (
	// LevelNone is the zero value, below core. Used internally when even the
	// core anchors are absent (a record that is not a photometric record at all,
	// normally already rejected by schema validation).
	LevelNone Level = iota
	LevelCore
	LevelStandard
	LevelFull
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
	default:
		return "none"
	}
}

// directionalCategories is the set of PrimaryCategory enum values for which a
// beam angle is meaningful (the product throws a defined beam: downlights,
// track/spot/accent heads, cylinders, wall grazers and washers, facade
// projectors, aimed in-ground uplights, aimed sports floods). Area, roadway,
// linear and cove categories are omitted on purpose, as are the broadly-
// distributing in-ground and landscape fixtures (bollard, step_marker,
// landscape_path_marker): they distribute broadly and carry no single beam
// angle. The in_ground_uplight category, by contrast, is an aimed architectural
// uplight (column, tree, and facade uplighting) specified by beam angle, so it
// is directional. Confirmed against taxonomy.schema.json PrimaryCategory.
var directionalCategories = map[string]bool{
	"downlight":         true,
	"tracklight":        true,
	"cylinder":          true,
	"wall_washer":       true,
	"grazer":            true,
	"facade_projector":  true,
	"in_ground_uplight": true,
	"sports_flood":      true,
}

// AchievedLevel returns the highest cumulative level whose hard requirements
// (with conditional predicates applied) are all met, reading only fields present
// in the record. This is the single computation the builder calls to populate
// index.conformance_level, and the validator calls to report it.
//
// Walk core -> standard -> full and stop at the first level with an unmet hard
// requirement. A record missing the core photometric anchors returns LevelNone;
// such a record is not a photometric record at all and is normally already
// rejected by schema validation upstream.
func AchievedLevel(record map[string]any) Level {
	if !checkCore(record) {
		return LevelNone
	}
	if !allMet(standardRequirements(record)) {
		return LevelCore
	}
	if !allMet(fullRequirements(record)) {
		return LevelStandard
	}
	return LevelFull
}

// Report appends the conformance INFO findings to report. It is the human-facing
// half of grading: the stored index.conformance_level is already verified by the
// builder-parity step, so this exists to explain to the author what the record
// achieved and how to climb. It emits:
//
//	(a) one summary INFO naming the achieved level;
//	(b) when below full, gap guidance listing the specific HARD fields missing
//	    for the next level up (conditional predicates applied);
//	(c) when at full, the INFO observations for comprehensive full-level items
//	    that are absent (TM-30 hue bins, uncertainty, method-backed projection,
//	    sustainability declaration, instrumentation depth, rated-not-measured
//	    provenance).
//
// Report emits no WARNINGs and no ERRORs: there is no declaration to violate.
func Report(record map[string]any, report *findings.Report) Level {
	achieved := AchievedLevel(record)

	// Summary INFO.
	report.AddInfo(findings.CodeConformanceLevel, "/index/conformance_level",
		fmt.Sprintf("this record achieves conformance level %q", achieved.String()))

	// Gap guidance toward the next level up, or full-level observations once at
	// full. LevelNone is the not-a-photometric-record case; schema validation
	// surfaces that, so grading stays silent on guidance there.
	switch achieved {
	case LevelCore:
		emitGap(LevelStandard, missingHard(standardRequirements(record)), report)
	case LevelStandard:
		emitGap(LevelFull, missingHard(fullRequirements(record)), report)
	case LevelFull:
		emitFullObservations(record, report)
	}

	return achieved
}

// emitGap records the gap-guidance INFO listing the HARD fields a record must
// add to reach the next level. missing is the ordered set of unmet requirement
// paths at that level (conditional predicates already applied).
func emitGap(next Level, missing []string, report *findings.Report) {
	if len(missing) == 0 {
		return
	}
	report.AddInfo(findings.CodeConformanceGap, "/index/conformance_level",
		fmt.Sprintf("to reach %q, add %s", next.String(), strings.Join(missing, ", ")))
}

// --- core ---

// checkCore reports whether the two photometric anchors plus the primary
// category are all present. Returns false when any is absent.
func checkCore(record map[string]any) bool {
	if !hasNumberValue(record, "photometry", "total_luminous_flux_lm") {
		return false
	}
	if !hasNumberValue(record, "electrical", "input_power_w") {
		return false
	}
	if getString(record, "product_family", "primary_category") == "" {
		return false
	}
	return true
}

// --- requirement model ---

// requirement is one hard gate at a given level. met records whether the field
// is present (after the conditional predicate is applied: an inapplicable
// conditional requirement is omitted from the slice entirely, so it is never
// reported as missing).
type requirement struct {
	level Level
	path  string // JSON Pointer, used in gap guidance and the achieved computation
	met   bool
}

func allMet(reqs []requirement) bool {
	for _, r := range reqs {
		if !r.met {
			return false
		}
	}
	return true
}

// missingHard returns the unmet requirement paths in path order, for
// deterministic gap-guidance output.
func missingHard(reqs []requirement) []string {
	out := []string{}
	for _, r := range reqs {
		if !r.met {
			out = append(out, r.path)
		}
	}
	sort.Strings(out)
	return out
}

// --- standard level ---

func standardRequirements(record map[string]any) []requirement {
	reqs := []requirement{
		{LevelStandard, "/photometry/luminaire_efficacy_lm_per_w", hasNumberValue(record, "photometry", "luminaire_efficacy_lm_per_w")},
		{LevelStandard, "/photometry/maximum_intensity_cd", hasNumberValue(record, "photometry", "maximum_intensity_cd")},
		{LevelStandard, "/photometry/distribution_type", getString(record, "photometry", "distribution_type") != ""},
		{LevelStandard, "/photometry/photometric_coordinate_system", getString(record, "photometry", "photometric_coordinate_system") != ""},
		{LevelStandard, "/photometry/symmetry_type", getString(record, "photometry", "symmetry_type") != ""},
		{LevelStandard, "/electrical/control_gear_type", getString(record, "electrical", "control_gear_type") != ""},
		// nominal_cct_k is a hard standard requirement (mandatory at standard).
		{LevelStandard, "/colorimetry/nominal_cct_k", getString(record, "colorimetry", "nominal_cct_k") != ""},
		{LevelStandard, "/test_conditions/photometry_basis", getString(record, "test_conditions", "photometry_basis") != ""},
		{LevelStandard, "/instrumentation/measurement_regime", getString(record, "instrumentation", "measurement_regime") != ""},
		// LM-79 family attestation.
		{LevelStandard, "/attestations", hasLM79Attestation(record)},
		// Either a luminaire or a package lumen-maintenance framework (any
		// framework, including a bare manufacturer_rated_claim, satisfies this).
		// The label names both accepted paths so gap guidance does not imply only
		// the luminaire block works.
		{LevelStandard, "/lumen_maintenance_luminaire (or /lumen_maintenance_package)", hasLumenMaintenance(record)},
	}

	// Input voltage: either input_voltage_v or input_voltage_class satisfies.
	// The label names both accepted paths so gap guidance does not imply only
	// input_voltage_v works.
	voltageMet := hasNumberValue(record, "electrical", "input_voltage_v") || getString(record, "electrical", "input_voltage_class") != ""
	reqs = append(reqs, requirement{LevelStandard, "/electrical/input_voltage_v (or /electrical/input_voltage_class)", voltageMet})

	// CONDITIONAL: beam_angle_deg only when the product is directional.
	if isDirectional(record) {
		reqs = append(reqs, requirement{LevelStandard, "/photometry/beam_angle_deg", hasNumberValue(record, "photometry", "beam_angle_deg")})
	}

	// CONDITIONAL: cri_ra only for white-light products. Pure color-mixing
	// (rgb / rgba, no white channel) skips it.
	if !isPureColorMixing(record) {
		reqs = append(reqs, requirement{LevelStandard, "/colorimetry/cri_ra", hasNumberValue(record, "colorimetry", "cri_ra")})
	}

	return reqs
}

// --- full level ---

func fullRequirements(record map[string]any) []requirement {
	reqs := []requirement{
		{LevelFull, "/operating_point", hasOperatingPoint(record)},
	}

	// CONDITIONAL: bug_rating only when the product is outdoor or both.
	if isOutdoor(record) {
		reqs = append(reqs, requirement{LevelFull, "/outdoor_classification/bug_rating", hasMap(record, "outdoor_classification", "bug_rating")})
	}

	return reqs
}

// emitFullObservations records the INFO-only items for a record at full. Each
// fires only when the corresponding depth is absent; over-delivery is silent.
// The provenance-quality note fires when a headline photometric value carries a
// value_type other than "measured".
func emitFullObservations(record map[string]any, report *findings.Report) {
	// TM-30 hue bins.
	if !hasValue(record, "colorimetry", "tm_30", "rf_h_per_bin") {
		report.AddInfo(findings.CodeConformanceObservation, "/colorimetry/tm_30/rf_h_per_bin",
			"full records commonly carry TM-30 per-hue-bin fidelity (colorimetry.tm_30.rf_h_per_bin); absent here")
	}
	// Measurement uncertainty.
	if !hasMap(record, "uncertainty") {
		report.AddInfo(findings.CodeConformanceObservation, "/uncertainty",
			"full records commonly carry a measurement uncertainty block; absent here")
	}
	// Method-backed lumen-maintenance projection (a bare manufacturer_rated_claim
	// does not count). Point the author at the block they already have: a record
	// carrying a lumen_maintenance_luminaire framework but no package adds the
	// tm_28 projection there, while a record with a package (or neither block)
	// adds a TM-21 projection to the package.
	if !hasMethodBackedLumenMaintenance(record) {
		lmPath := "/lumen_maintenance_package"
		if hasMap(record, "lumen_maintenance_luminaire") && !hasLumenMaintenancePackage(record) {
			lmPath = "/lumen_maintenance_luminaire/tm_28"
		}
		report.AddInfo(findings.CodeConformanceObservation, lmPath,
			"full records commonly carry a method-backed lumen-maintenance projection (TM-21 hours or a TM-28 projection); only a manufacturer-rated claim is present")
	}
	// Sustainability declaration.
	if !hasMap(record, "sustainability_declaration") {
		report.AddInfo(findings.CodeConformanceObservation, "/sustainability_declaration",
			"full records commonly carry a sustainability_declaration block; absent here")
	}
	// Instrumentation depth beyond measurement_regime.
	if !hasInstrumentationDepth(record) {
		report.AddInfo(findings.CodeConformanceObservation, "/instrumentation",
			"full records commonly carry instrumentation depth beyond measurement_regime (laboratory certification, accreditation scheme, name, report id, or goniometer type); none present")
	}
	// Provenance quality: headline photometric values not measured.
	for _, p := range nonMeasuredHeadlineValues(record) {
		report.AddInfo(findings.CodeConformanceObservation, p.path,
			fmt.Sprintf("headline photometric value %s carries value_type %q rather than \"measured\"", p.field, p.valueType))
	}
}

// --- predicates ---

// isDirectional reports whether the product's primary category throws a defined
// beam (so a beam angle is expected). Reads the authoritative deep block
// product_family.primary_category. The index is a denormalized projection of
// this field and is written by the builder AFTER it calls AchievedLevel, so the
// deep block is the only source that is reliably present at grade time.
func isDirectional(record map[string]any) bool {
	cat := getString(record, "product_family", "primary_category")
	return directionalCategories[cat]
}

// isPureColorMixing reports whether the fixture is pure color-mixing with no
// white channel, in which case CRI Ra is not meaningful. Reads the authoritative
// deep block configuration.tested_axes.color_tunability: rgb or rgba signals
// pure color-mixing. Any other value (including absent) is treated as not pure
// color-mixing, so cri_ra is required whenever the predicate cannot prove pure
// RGB (conservative per the rubric). The index.color_tunability projection is
// written by the builder after grading, so the deep block is the source.
func isPureColorMixing(record map[string]any) bool {
	tun := getString(record, "configuration", "tested_axes", "color_tunability")
	return tun == "rgb" || tun == "rgba"
}

// isOutdoor reports whether the product is intended for outdoor use (so a BUG
// rating is expected): indoor_outdoor is "outdoor" or "both".
func isOutdoor(record map[string]any) bool {
	io := getString(record, "product_family", "indoor_outdoor")
	return io == "outdoor" || io == "both"
}

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
// lumen_maintenance_luminaire that carries only a manufacturer_rated_claim.
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
// tm_21_projection_hours value, or a luminaire framework with a tm_28
// projection. A bare manufacturer_rated_claim does not count.
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
// of the recognized qualifiers.
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
		// A qualifier counts only with real content: a non-empty string
		// (dut_operating_mode) or a non-empty object (ProvenancedNumber via
		// .value, DualUnitTemperature via .c/.f). An empty shell does not.
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

// hasInstrumentationDepth reports whether instrumentation carries any field
// beyond measurement_regime that signals lab depth.
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

type headlineValue struct {
	field     string
	path      string
	valueType string
}

// nonMeasuredHeadlineValues returns the headline photometric values whose
// value_type is something other than "measured" (for example "rated" or
// "nominal"), as a provenance-quality observation at full.
func nonMeasuredHeadlineValues(record map[string]any) []headlineValue {
	out := []headlineValue{}
	type spec struct {
		field  string
		parent string
		key    string
	}
	for _, s := range []spec{
		{"photometry.total_luminous_flux_lm", "photometry", "total_luminous_flux_lm"},
		{"photometry.luminaire_efficacy_lm_per_w", "photometry", "luminaire_efficacy_lm_per_w"},
		{"photometry.maximum_intensity_cd", "photometry", "maximum_intensity_cd"},
		{"electrical.input_power_w", "electrical", "input_power_w"},
	} {
		pn, ok := getMap(record, s.parent, s.key)
		if !ok {
			continue
		}
		vt, _ := pn["value_type"].(string)
		if vt != "" && vt != "measured" {
			out = append(out, headlineValue{
				field:     s.field,
				path:      "/" + s.parent + "/" + s.key + "/value_type",
				valueType: vt,
			})
		}
	}
	return out
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

// hasValue reports whether a value is present and non-nil at the path.
func hasValue(record map[string]any, keys ...string) bool {
	node := any(record)
	for i, k := range keys {
		m, ok := node.(map[string]any)
		if !ok {
			return false
		}
		v, present := m[k]
		if !present || v == nil {
			return false
		}
		if i == len(keys)-1 {
			return true
		}
		node = v
	}
	return false
}

// hasMap reports whether the value at the path is a non-empty object.
func hasMap(record map[string]any, keys ...string) bool {
	m, ok := getMap(record, keys...)
	return ok && m != nil && len(m) > 0
}

// getMap returns the object at the path and whether it was found as an object.
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
