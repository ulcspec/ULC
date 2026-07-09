package achievements

// The six achievement theme keys. Always all emitted (deterministic index shape).
const (
	ThemeEmbodiedCarbon = "embodied_carbon"
	ThemeCircularity    = "circularity"
	ThemeMaterialHealth = "material_health"
	ThemeEnergy         = "energy"
	ThemeDarkSky        = "dark_sky"
	ThemeEmergency      = "emergency"
)

// themeOrder is the canonical iteration order for deterministic emission. Findings are
// re-sorted by Finalize, but iterating in a fixed order keeps intermediate output stable.
var themeOrder = []string{
	ThemeEmbodiedCarbon,
	ThemeCircularity,
	ThemeMaterialHealth,
	ThemeEnergy,
	ThemeDarkSky,
	ThemeEmergency,
}

// The program-to-theme map (the published, versioned classification). Each set is a
// named package var so the achievements drift guard can assert the AttestationProgram
// enum partitions exactly into {six themes} + {restricted} + {unthemed residue}: a new
// enum token then fails the guard until it is consciously triaged rather than silently
// badged all-none.

// embodiedCarbonPrograms: a declared kg CO2e figure.
var embodiedCarbonPrograms = map[string]bool{
	"epd_iso_14025": true,
	"tm65_2":        true,
}

// circularityPrograms: a design/lifecycle circularity rating. cradle_to_cradle
// dual-routes (circularity primary, material_health presence).
var circularityPrograms = map[string]bool{
	"tm66_assured":     true,
	"cradle_to_cradle": true,
}

// materialHealthPrograms: ingredient / red-list disclosure. cradle_to_cradle appears
// here too, the one deliberate dual-route.
var materialHealthPrograms = map[string]bool{
	"declare":               true,
	"lbc_red_list_free":     true,
	"lbc_red_list_approved": true,
	"lbc_red_list_declared": true,
	"hpd":                   true,
	"just_label":            true,
	"cradle_to_cradle":      true,
}

// energyPrograms: re-anchored on DLC after ENERGY STAR retired its luminaire specs;
// Title 24 JA8 and Title 20 / EU Ecodesign secondary.
var energyPrograms = map[string]bool{
	"dlc_standard":              true,
	"dlc_premium":               true,
	"dlc_qpl":                   true,
	"dlc_horticultural":         true,
	"energy_star":               true,
	"energy_star_downlights_v1": true,
	"ja8_title_24":              true,
	"ca_title_20":               true,
	"eu_ecodesign_2019_2020":    true,
	"eu_energy_label_2019_2015": true,
	"nrcan_ee_regulations":      true,
}

// darkSkyPrograms: light-pollution qualifications.
var darkSkyPrograms = map[string]bool{
	"darksky_approved": true,
	"dlc_luna":         true,
}

// emergencyPrograms: emergency-lighting product-conformity schemes. icel is the ICEL
// emergency-conformity scheme (a genuine emergency qualification), NOT LIA-style
// trade-body membership, so it is themed here despite the taxonomy prose grouping ICEL
// near the trade bodies.
var emergencyPrograms = map[string]bool{
	"ul_924":  true,
	"ul_1994": true,
	"icel":    true,
}

// restrictedSubstancePrograms feeds index.restricted_substances_declared, the sibling
// legal-floor flag. A restricted-substances declaration is table-stakes legality, never
// a prestige achievement, so these are surfaced beside the themes, never inside one.
var restrictedSubstancePrograms = map[string]bool{
	"rohs":                  true,
	"reach":                 true,
	"reach_svhc":            true,
	"weee":                  true,
	"prop_65":               true,
	"tsca":                  true,
	"pops":                  true,
	"conflict_minerals_3tg": true,
}

// themeSets maps each theme key to its program set, for iteration.
var themeSets = map[string]map[string]bool{
	ThemeEmbodiedCarbon: embodiedCarbonPrograms,
	ThemeCircularity:    circularityPrograms,
	ThemeMaterialHealth: materialHealthPrograms,
	ThemeEnergy:         energyPrograms,
	ThemeDarkSky:        darkSkyPrograms,
	ThemeEmergency:      emergencyPrograms,
}

// declarationTokens maps a sustainability_declaration.declaration_type to the
// AttestationProgram token it implies, mirroring the index builder's
// collectAttestationPrograms mapping EXACTLY. Every mapped token is a material_health
// program, and a declaration-derived contribution caps at claimed (a declaration block
// carries no evidence document). manufacturer_recycle_program is deliberately NOT here:
// it is a SustainabilityDeclarationType with no AttestationProgram token, so it can
// never populate the schema-typed programs array; it is handled as a direct
// declaration-to-circularity(claimed) rule in Compute.
var declarationTokens = map[string]string{
	"ilfi_declare":      "declare",
	"red_list_free":     "lbc_red_list_free",
	"red_list_approved": "lbc_red_list_approved",
	"red_list_declared": "lbc_red_list_declared",
}

// unthemedPrograms is the deliberately-unthemed residue: AttestationProgram tokens that
// are not a single-theme luminaire qualification. Named explicitly (like the completeness
// descriptiveAllowlist) so the exhaustiveness guard forces a conscious triage when a new
// token joins the enum, rather than silently defaulting it to unthemed.
var unthemedPrograms = map[string]bool{
	// Project- and company-level programs (LEED, WELL, Living Building Challenge): whole-building or
	// whole-company, not luminaire qualifications.
	"leed_v4":                    true,
	"leed_v4_1":                  true,
	"leed_v5":                    true,
	"living_building_challenge":  true,
	"living_community_challenge": true,
	"living_product_challenge":   true,
	"well_building_standard":     true,
	// Multi-attribute environmental ecolabels: span energy, materials, packaging, and end-of-life,
	// not single-theme luminaire qualifications.
	"epeat":                 true,
	"greencircle_certified": true,
	"ul_ecologo":            true,
	// Mandatory or discontinued disclosure labels, not qualifications.
	"doe_led_lighting_facts": true,
	"ftc_lighting_facts":     true,
	// Domestic content (v1.1 theme candidate).
	"american_iron_and_steel": true,
	"baa":                     true,
	"baba":                    true,
	"country_of_origin":       true,
	"taa":                     true,
	// Controls (v1.1 theme candidate).
	"dlc_nlc":    true,
	"ntcip_1213": true,
	// Hazardous location (v1.2 theme candidate).
	"atex":  true,
	"iecex": true,
	// Regional market-access marks (v1.2 consolidated-indicator candidate).
	"bis_india":      true,
	"cb_scheme":      true,
	"ccc":            true,
	"ce":             true,
	"csa_c653":       true,
	"csa_c811":       true,
	"eac_eaeu":       true,
	"enec":           true,
	"fcc":            true,
	"ices_canada":    true,
	"inmetro_brazil": true,
	"kc_korea":       true,
	"nom":            true,
	"pse_japan":      true,
	"rcm_australia":  true,
	"saa_australia":  true,
	"saso_saudi":     true,
	"ukca":           true,
	"vcci_japan":     true,
	// Safety, EMC, and component listings.
	"ansi_c136_25":         true,
	"ansi_c136_31":         true,
	"c_ul_listed":          true,
	"chicago_plenum":       true,
	"cispr_15":             true,
	"csa_listed":           true,
	"damp_location_ul":     true,
	"etl":                  true,
	"iec_60598":            true,
	"iec_61347":            true,
	"iec_62031":            true,
	"iec_62471":            true,
	"ieee_1789_2015":       true,
	"met_listed":           true,
	"nema_77_2017":         true,
	"nrtl_osha_recognized": true,
	"nsf_ansi_2":           true,
	"tuv":                  true,
	"ul_1574":              true,
	"ul_1598":              true,
	"ul_2108":              true,
	"ul_8750":              true,
	"ul_listed":            true,
	"wet_location_ul":      true,
	// Lab accreditation, trade-body membership, and quality-management credentials.
	"iso_9001":             true,
	"lia_audited":          true,
	"lia_member":           true,
	"liaqa":                true,
	"liasc_plus":           true,
	"performance_verified": true,
	// Test-method and provenance tokens (measurement standards, not badges).
	"ansi_c78_377_2017": true,
	"ansi_c78_377_2024": true,
	"cie_13":            true,
	"lm_31_20":          true,
	"lm_35_20":          true,
	"lm_75_19":          true,
	"lm_78_20":          true,
	"lm_79":             true,
	"lm_79_08":          true,
	"lm_79_19":          true,
	"lm_79_24":          true,
	"lm_80":             true,
	"lm_80_08":          true,
	"lm_80_15":          true,
	"lm_80_20":          true,
	"lm_80_21":          true,
	"lm_82_20":          true,
	"lm_84":             true,
	"lm_84_14":          true,
	"lm_84_20":          true,
	"lm_84_20_e1":       true,
	"lm_85_20":          true,
	"lm_90_20":          true,
	"rp_46":             true,
	"rp_46_23":          true,
	"tm_15":             true,
	"tm_15_11":          true,
	"tm_15_20":          true,
	"tm_21":             true,
	"tm_21_11":          true,
	"tm_21_21":          true,
	"tm_27_20":          true,
	"tm_28":             true,
	"tm_28_20":          true,
	"tm_30":             true,
	"tm_30_15":          true,
	"tm_30_18":          true,
	"tm_30_20":          true,
	"tm_30_24":          true,
	"tm_35":             true,
	"tm_35_19":          true,
	"tm_35_19_e1":       true,
}
