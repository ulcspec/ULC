package sheet

// Kind classifies how a records-sheet cell maps onto a ULC field. The assembler
// dispatches on Kind to coerce the cell and write the right shape at the column's
// dotted path.
type Kind int

const (
	// KindString writes the trimmed cell verbatim as a JSON string.
	KindString Kind = iota
	// KindEnum is a string whose domain the schema validates. The converter
	// writes it verbatim; the schema (loaded by the validator) rejects unknown
	// tokens with the offending instance location, so there is no second copy of
	// the enum domain here.
	KindEnum
	// KindNumber writes the cell as a JSON number (int when integral).
	KindNumber
	// KindBool writes the cell as a JSON boolean (true/false, yes/no, 1/0).
	KindBool
	// KindDate writes a date string verbatim (schema enforces the date format).
	KindDate
	// KindList splits the cell on ";" into a JSON array of trimmed strings.
	KindList
	// KindProvNumber writes a ProvenancedNumber object: {value, value_type,
	// provenance{source, method, ...}} with per-column provenance defaults.
	KindProvNumber
	// KindDualUnitSI writes a DualUnit object from the authored SI leaf, computing
	// the Imperial/Fahrenheit companion. The Unit field selects the family.
	KindDualUnitSI
)

// Column is one declarative mapping from a records-sheet header to a ULC field.
// The full set of Columns is the implementation contract for the records sheet:
// it is read as data by the assembler, so adding a field is a one-line edit here
// rather than new code.
type Column struct {
	// Header is the records.csv column name the manufacturer authors.
	Header string
	// Path is the dotted ULC path the value is written to (for example
	// "photometry.total_luminous_flux_lm.value").
	Path string
	// Kind selects the coercion and output shape.
	Kind Kind
	// Unit selects the dual-unit family for KindDualUnitSI columns; ignored
	// otherwise. For ProvenancedNumber columns it is the optional "unit" string
	// (lm, W, cd, ...).
	Unit string
	// DualKind is the dual-unit family for KindDualUnitSI columns.
	DualKind dualUnitKind
	// ProvSource / ProvMethod / ProvValueType are the per-column provenance
	// defaults applied to KindProvNumber and KindDualUnitSI columns when the
	// manufacturer leaves the companion override columns blank. See DESIGN.md
	// section 3.3.
	ProvSource    string
	ProvMethod    string
	ProvValueType string
}

// Provenanced reports whether this column carries provenance/value_type (and so
// participates in the measured -> attestation_ref auto-link).
func (c Column) Provenanced() bool {
	return c.Kind == KindProvNumber || c.Kind == KindDualUnitSI
}

// recordColumns is the column spec for the records sheet, covering all CORE and
// STANDARD fields for a downlight-class record: identity, cutsheet, taxonomy,
// shared_mechanical, the common physical_dimensions, configuration (tested_axes
// and tested_conditions), electrical, photometry, and colorimetry. Optional and
// full-only columns are deferred to increment 3; the data-driven shape means
// they are added here without touching the assembler.
//
// Provenance defaults follow DESIGN.md section 3.3: datasheet-sourced mechanical
// and rated electrical values default to {datasheet_pdf, extracted, rated}; the
// photometric anchors default to {ies, extracted, measured}, which triggers the
// attestation_ref auto-link.
var recordColumns = []Column{
	// --- top-level identity ---
	{Header: "ulc_version", Path: "ulc_version", Kind: KindString},
	{Header: "record_status", Path: "record_status", Kind: KindEnum},
	{Header: "record_status_as_of", Path: "record_status_as_of", Kind: KindDate},

	// --- product_family identity ---
	{Header: "family_id", Path: "product_family.family_id", Kind: KindString},
	{Header: "family_display_name", Path: "product_family.family_display_name", Kind: KindString},
	{Header: "family_description", Path: "product_family.family_description", Kind: KindString},
	{Header: "manufacturer_slug", Path: "product_family.manufacturer.slug", Kind: KindString},
	{Header: "manufacturer_display_name", Path: "product_family.manufacturer.display_name", Kind: KindString},
	{Header: "catalog_line", Path: "product_family.catalog_line", Kind: KindString},
	{Header: "catalog_model", Path: "product_family.catalog_model", Kind: KindString},

	// --- product_family taxonomy ---
	{Header: "primary_category", Path: "product_family.primary_category", Kind: KindEnum},
	{Header: "secondary_function", Path: "product_family.secondary_function", Kind: KindList},
	{Header: "indoor_outdoor", Path: "product_family.indoor_outdoor", Kind: KindEnum},
	{Header: "technical_region", Path: "product_family.technical_region", Kind: KindEnum},
	{Header: "mounting_types", Path: "product_family.mounting_types", Kind: KindList},
	{Header: "environment_rating", Path: "product_family.environment_rating", Kind: KindEnum},
	{Header: "shape", Path: "product_family.shape", Kind: KindEnum},

	// --- shared_mechanical ---
	{Header: "housing_material", Path: "product_family.shared_mechanical.housing_material", Kind: KindEnum},
	{Header: "lens_material", Path: "product_family.shared_mechanical.lens_material", Kind: KindEnum},
	{Header: "reflector_material", Path: "product_family.shared_mechanical.reflector_material", Kind: KindString},
	{Header: "finish_color_options", Path: "product_family.shared_mechanical.finish_color_options", Kind: KindList},
	{Header: "ip_rating", Path: "product_family.shared_mechanical.ip_rating", Kind: KindString},
	{Header: "ik_rating", Path: "product_family.shared_mechanical.ik_rating", Kind: KindString},

	// --- physical_dimensions (common downlight subset) ---
	{Header: "overall_diameter_mm", Path: "product_family.physical_dimensions.overall_diameter", Kind: KindDualUnitSI, DualKind: dualLength, ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},
	{Header: "ceiling_aperture_mm", Path: "product_family.physical_dimensions.ceiling_aperture", Kind: KindDualUnitSI, DualKind: dualLength, ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},
	{Header: "recess_depth_mm", Path: "product_family.physical_dimensions.recess_depth", Kind: KindDualUnitSI, DualKind: dualLength, ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},
	{Header: "connection_cable_length_mm", Path: "product_family.physical_dimensions.connection_cable_length", Kind: KindDualUnitSI, DualKind: dualLength, ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},
	{Header: "overall_length_mm", Path: "product_family.physical_dimensions.overall_length", Kind: KindDualUnitSI, DualKind: dualLength, ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},
	{Header: "overall_width_mm", Path: "product_family.physical_dimensions.overall_width", Kind: KindDualUnitSI, DualKind: dualLength, ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},
	{Header: "overall_height_mm", Path: "product_family.physical_dimensions.overall_height", Kind: KindDualUnitSI, DualKind: dualLength, ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},
	{Header: "luminaire_mass_kg", Path: "product_family.physical_dimensions.luminaire_mass", Kind: KindDualUnitSI, DualKind: dualMass, ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},
	{Header: "linear_mass_per_foot_kg_per_m", Path: "product_family.physical_dimensions.linear_mass_per_foot", Kind: KindDualUnitSI, DualKind: dualMassPerLength, ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},

	// --- configuration ---
	{Header: "photometric_scenario_id", Path: "configuration.photometric_scenario_id", Kind: KindString},
	{Header: "catalog_number", Path: "configuration.catalog_number", Kind: KindString},
	{Header: "scenario_label", Path: "configuration.scenario_label", Kind: KindString},
	{Header: "source_ies_ref", Path: "configuration.source_ies_ref", Kind: KindString},

	// configuration.tested_axes
	{Header: "distribution_manufacturer_label", Path: "configuration.tested_axes.distribution_code.manufacturer_label", Kind: KindString},
	{Header: "distribution_type", Path: "configuration.tested_axes.distribution_code.distribution_type", Kind: KindEnum},
	{Header: "outdoor_distribution_type_axis", Path: "configuration.tested_axes.distribution_code.outdoor_distribution_type", Kind: KindEnum},
	{Header: "light_engine_variant", Path: "configuration.tested_axes.light_engine_variant", Kind: KindString},
	{Header: "output_tier_manufacturer_label", Path: "configuration.tested_axes.output_tier.manufacturer_label", Kind: KindString},
	{Header: "output_tier_meaning", Path: "configuration.tested_axes.output_tier.tier_meaning", Kind: KindString},
	{Header: "cri_tier", Path: "configuration.tested_axes.cri_tier", Kind: KindEnum},
	{Header: "color_tunability", Path: "configuration.tested_axes.color_tunability", Kind: KindEnum},

	// configuration.tested_conditions
	{Header: "nominal_cct_at_test", Path: "configuration.tested_conditions.nominal_cct_at_test", Kind: KindEnum},
	{Header: "input_voltage_at_test", Path: "configuration.tested_conditions.input_voltage_at_test", Kind: KindString},
	{Header: "mounting_at_test", Path: "configuration.tested_conditions.mounting_at_test", Kind: KindEnum},

	// --- electrical ---
	{Header: "input_power_w", Path: "electrical.input_power_w", Kind: KindProvNumber, Unit: "W", ProvSource: "ies", ProvMethod: "extracted", ProvValueType: "measured"},
	{Header: "input_voltage_v", Path: "electrical.input_voltage_v", Kind: KindProvNumber, Unit: "V", ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},
	{Header: "input_voltage_class", Path: "electrical.input_voltage_class", Kind: KindString},
	{Header: "driver_protocol", Path: "electrical.driver_protocol", Kind: KindEnum},
	{Header: "dimming_method", Path: "electrical.dimming_method", Kind: KindEnum},
	{Header: "control_gear_type", Path: "electrical.control_gear_type", Kind: KindEnum},
	{Header: "led_module_power_w", Path: "electrical.led_module_power_w", Kind: KindProvNumber, Unit: "W", ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},

	// --- photometry ---
	{Header: "total_luminous_flux_lm", Path: "photometry.total_luminous_flux_lm", Kind: KindProvNumber, Unit: "lm", ProvSource: "ies", ProvMethod: "extracted", ProvValueType: "measured"},
	{Header: "luminaire_efficacy_lm_per_w", Path: "photometry.luminaire_efficacy_lm_per_w", Kind: KindProvNumber, Unit: "lm/W", ProvSource: "ies", ProvMethod: "computed", ProvValueType: "measured"},
	{Header: "maximum_intensity_cd", Path: "photometry.maximum_intensity_cd", Kind: KindProvNumber, Unit: "cd", ProvSource: "ies", ProvMethod: "extracted", ProvValueType: "measured"},
	{Header: "beam_angle_deg", Path: "photometry.beam_angle_deg", Kind: KindProvNumber, Unit: "deg", ProvSource: "ies", ProvMethod: "extracted", ProvValueType: "measured"},
	{Header: "field_angle_deg", Path: "photometry.field_angle_deg", Kind: KindProvNumber, Unit: "deg", ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "nominal"},
	{Header: "ugr_4h_8h", Path: "photometry.ugr_4h_8h", Kind: KindProvNumber, Unit: "ratio", ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},
	{Header: "ugr_4h_8h_bound_operator", Path: "photometry.ugr_4h_8h_bound_operator", Kind: KindEnum},
	{Header: "beam_family", Path: "photometry.beam_family", Kind: KindEnum},
	{Header: "distribution_type_photometry", Path: "photometry.distribution_type", Kind: KindEnum},
	{Header: "symmetry_type", Path: "photometry.symmetry_type", Kind: KindEnum},
	{Header: "photometric_coordinate_system", Path: "photometry.photometric_coordinate_system", Kind: KindEnum},
	{Header: "luminous_opening_shape", Path: "photometry.luminous_opening_shape", Kind: KindEnum},
	{Header: "emission_face", Path: "photometry.emission_face", Kind: KindEnum},

	// --- photometry.per_length_normalized (Pattern D per-foot rates) ---
	// These rates feed the declared_by_length generator (see linear.go) and are
	// also written directly onto the record so a Pattern D record carries the
	// authoritative per-unit-length values it scales from.
	{Header: "lumens_per_foot", Path: "photometry.per_length_normalized.lumens_per_foot", Kind: KindProvNumber, Unit: "lm/ft", ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},
	{Header: "watts_per_foot", Path: "photometry.per_length_normalized.watts_per_foot", Kind: KindProvNumber, Unit: "W/ft", ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},
	{Header: "reference_length_mm", Path: "photometry.per_length_normalized.reference_length", Kind: KindDualUnitSI, DualKind: dualLength, ProvSource: "datasheet_pdf", ProvMethod: "normalized", ProvValueType: "nominal"},

	// --- operating_point (full-level gate: any one qualifier present satisfies) ---
	{Header: "operating_input_voltage_v", Path: "operating_point.input_voltage_v", Kind: KindProvNumber, Unit: "V", ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},
	{Header: "operating_input_frequency_hz", Path: "operating_point.input_frequency_hz", Kind: KindProvNumber, Unit: "Hz", ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},

	// --- colorimetry ---
	{Header: "nominal_cct_k", Path: "colorimetry.nominal_cct_k", Kind: KindEnum},
	{Header: "cri_ra", Path: "colorimetry.cri_ra", Kind: KindProvNumber, Unit: "ratio", ProvSource: "ies", ProvMethod: "extracted", ProvValueType: "measured"},
	{Header: "duv", Path: "colorimetry.duv", Kind: KindProvNumber, Unit: "ratio", ProvSource: "ies", ProvMethod: "extracted", ProvValueType: "measured"},
	{Header: "sdcm_step", Path: "colorimetry.sdcm_step", Kind: KindProvNumber, Unit: "ratio", ProvSource: "datasheet_pdf", ProvMethod: "extracted", ProvValueType: "rated"},

	// --- outdoor_classification (full-level gate for outdoor products) ---
	{Header: "bug_b", Path: "outdoor_classification.bug_rating.b", Kind: KindNumber},
	{Header: "bug_u", Path: "outdoor_classification.bug_rating.u", Kind: KindNumber},
	{Header: "bug_g", Path: "outdoor_classification.bug_rating.g", Kind: KindNumber},
	{Header: "outdoor_distribution_type", Path: "outdoor_classification.outdoor_distribution_type", Kind: KindEnum},
	{Header: "longitudinal_distribution_range", Path: "outdoor_classification.longitudinal_distribution_range", Kind: KindEnum},

	// --- test_conditions / instrumentation (standard-level hard gates) ---
	{Header: "photometry_basis", Path: "test_conditions.photometry_basis", Kind: KindEnum},
	{Header: "measurement_regime", Path: "instrumentation.measurement_regime", Kind: KindEnum},

	// --- lumen_maintenance_luminaire (bare manufacturer claim; standard gate) ---
	// The standard rubric requires any lumen-maintenance framework. The simplest
	// inline form is a bare manufacturer_rated_claim authored on the records sheet.
	// Richer LM-80 / TM-21 data rides on the lumen_maintenance_package long sheet,
	// and the CIE-97 LMF table on the cie97_lmf / cie97_llmf long sheets; both
	// coexist with these scalars under the same parent (see fulllevel.go).
	{Header: "lm_declaration_framework", Path: "lumen_maintenance_luminaire.declaration_framework", Kind: KindEnum},
	{Header: "lm_claim_type", Path: "lumen_maintenance_luminaire.manufacturer_rated_claim.claim_type", Kind: KindEnum},
	{Header: "lm_claimed_hours", Path: "lumen_maintenance_luminaire.manufacturer_rated_claim.claimed_hours", Kind: KindProvNumber, Unit: "h", ProvSource: "manufacturer_direct", ProvMethod: "transcribed", ProvValueType: "rated"},

	// --- sustainability_declaration (full-level enrichment; block-level scalars) ---
	// The Declare / Living Building Challenge roster (ingredient_list) rides on its
	// own long sheet; these one-per-record scalars frame it. declaration_type is an
	// enum the schema validates; the *_content / *_performance / *_sourcing fields
	// are free strings ("not_applicable" is common on a Declare label).
	{Header: "sustainability_declaration_type", Path: "sustainability_declaration.declaration_type", Kind: KindEnum},
	{Header: "sustainability_document_id", Path: "sustainability_declaration.document_id", Kind: KindString},
	{Header: "sustainability_issue_date", Path: "sustainability_declaration.original_issue_date", Kind: KindDate},
	{Header: "sustainability_expiration_date", Path: "sustainability_declaration.expiration_date", Kind: KindDate},
	{Header: "final_assembly_location", Path: "sustainability_declaration.final_assembly_location", Kind: KindString},
	{Header: "life_expectancy_years", Path: "sustainability_declaration.life_expectancy_years", Kind: KindNumber},
	{Header: "recyclable_percent", Path: "sustainability_declaration.recyclable_percent", Kind: KindNumber},
	{Header: "end_of_life_options", Path: "sustainability_declaration.end_of_life_options", Kind: KindList},
	{Header: "lbc_criteria_compliance", Path: "sustainability_declaration.lbc_criteria_compliance", Kind: KindBool},
	{Header: "voc_content", Path: "sustainability_declaration.voc_content", Kind: KindString},
	{Header: "interior_performance", Path: "sustainability_declaration.interior_performance", Kind: KindString},
	{Header: "responsible_sourcing", Path: "sustainability_declaration.responsible_sourcing", Kind: KindString},
}
