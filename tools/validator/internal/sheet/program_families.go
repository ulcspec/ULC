package sheet

import "sort"

type attestationFamily string

const (
	attestationFamilyPhotometric attestationFamily = "photometric"
	attestationFamilyMaintenance attestationFamily = "maintenance"
	attestationFamilyFlicker     attestationFamily = "flicker"
	attestationFamilyMelanopic   attestationFamily = "melanopic"
)

// programFamilies is the authored routing table for programs that can anchor
// converter provenance. It uses every spelling the taxonomy publishes for each
// anchoring family. All other declared programs are listed explicitly in
// nonAnchoringPrograms below.
var programFamilies = map[string]attestationFamily{
	"lm_79":          attestationFamilyPhotometric,
	"lm_79_08":       attestationFamilyPhotometric,
	"lm_79_19":       attestationFamilyPhotometric,
	"lm_79_24":       attestationFamilyPhotometric,
	"lm_80":          attestationFamilyMaintenance,
	"lm_80_08":       attestationFamilyMaintenance,
	"lm_80_15":       attestationFamilyMaintenance,
	"lm_80_20":       attestationFamilyMaintenance,
	"lm_80_21":       attestationFamilyMaintenance,
	"tm_21":          attestationFamilyMaintenance,
	"tm_21_11":       attestationFamilyMaintenance,
	"tm_21_21":       attestationFamilyMaintenance,
	"lm_90_20":       attestationFamilyFlicker,
	"ieee_1789_2015": attestationFamilyFlicker,
	"nema_77_2017":   attestationFamilyFlicker,
	"rp_46":          attestationFamilyMelanopic,
	"rp_46_23":       attestationFamilyMelanopic,
}

// nonAnchoringPrograms is the declared residue. These programs are real
// AttestationProgram tokens, but none attests one of the converter value
// families above, so none can be selected as an automatic provenance anchor.
var nonAnchoringPrograms = map[string]bool{
	"lia_member":                 true,
	"lia_audited":                true,
	"liaqa":                      true,
	"liasc_plus":                 true,
	"performance_verified":       true,
	"tm66_assured":               true,
	"tm65_2":                     true,
	"icel":                       true,
	"iso_9001":                   true,
	"ul_listed":                  true,
	"c_ul_listed":                true,
	"etl":                        true,
	"tuv":                        true,
	"nom":                        true,
	"cb_scheme":                  true,
	"ul_1598":                    true,
	"ul_1574":                    true,
	"ul_8750":                    true,
	"nrtl_osha_recognized":       true,
	"iec_60598":                  true,
	"iec_62031":                  true,
	"iec_62471":                  true,
	"dlc_standard":               true,
	"dlc_premium":                true,
	"dlc_qpl":                    true,
	"energy_star":                true,
	"ja8_title_24":               true,
	"baa":                        true,
	"baba":                       true,
	"chicago_plenum":             true,
	"ce":                         true,
	"ukca":                       true,
	"ccc":                        true,
	"fcc":                        true,
	"atex":                       true,
	"iecex":                      true,
	"darksky_approved":           true,
	"enec":                       true,
	"rohs":                       true,
	"reach":                      true,
	"wet_location_ul":            true,
	"damp_location_ul":           true,
	"declare":                    true,
	"lbc_red_list_free":          true,
	"lbc_red_list_approved":      true,
	"lbc_red_list_declared":      true,
	"living_building_challenge":  true,
	"living_product_challenge":   true,
	"living_community_challenge": true,
	"just_label":                 true,
	"well_building_standard":     true,
	"leed_v4":                    true,
	"leed_v4_1":                  true,
	"leed_v5":                    true,
	"cie_13":                     true,
	"lm_75_19":                   true,
	"lm_78_20":                   true,
	"lm_82_20":                   true,
	"tm_30":                      true,
	"tm_30_15":                   true,
	"tm_30_18":                   true,
	"tm_30_20":                   true,
	"tm_30_24":                   true,
	"lm_84":                      true,
	"lm_84_14":                   true,
	"lm_84_20":                   true,
	"lm_84_20_e1":                true,
	"lm_85_20":                   true,
	"tm_27_20":                   true,
	"tm_28":                      true,
	"tm_28_20":                   true,
	"tm_15":                      true,
	"tm_15_11":                   true,
	"tm_15_20":                   true,
	"lm_31_20":                   true,
	"lm_35_20":                   true,
	"ntcip_1213":                 true,
	"ansi_c136_25":               true,
	"ansi_c136_31":               true,
	"csa_c653":                   true,
	"csa_c811":                   true,
	"ansi_c78_377_2024":          true,
	"ansi_c78_377_2017":          true,
	"tm_35":                      true,
	"tm_35_19":                   true,
	"tm_35_19_e1":                true,
	"csa_listed":                 true,
	"met_listed":                 true,
	"ul_924":                     true,
	"ul_2108":                    true,
	"ul_1994":                    true,
	"iec_61347":                  true,
	"ices_canada":                true,
	"cispr_15":                   true,
	"rcm_australia":              true,
	"saa_australia":              true,
	"kc_korea":                   true,
	"pse_japan":                  true,
	"vcci_japan":                 true,
	"bis_india":                  true,
	"eac_eaeu":                   true,
	"inmetro_brazil":             true,
	"saso_saudi":                 true,
	"nsf_ansi_2":                 true,
	"energy_star_downlights_v1":  true,
	"dlc_nlc":                    true,
	"dlc_luna":                   true,
	"dlc_horticultural":          true,
	"ca_title_20":                true,
	"eu_ecodesign_2019_2020":     true,
	"eu_energy_label_2019_2015":  true,
	"ftc_lighting_facts":         true,
	"doe_led_lighting_facts":     true,
	"nrcan_ee_regulations":       true,
	"reach_svhc":                 true,
	"weee":                       true,
	"prop_65":                    true,
	"conflict_minerals_3tg":      true,
	"pops":                       true,
	"tsca":                       true,
	"epd_iso_14025":              true,
	"hpd":                        true,
	"cradle_to_cradle":           true,
	"greencircle_certified":      true,
	"epeat":                      true,
	"ul_ecologo":                 true,
	"taa":                        true,
	"american_iron_and_steel":    true,
	"country_of_origin":          true,
}

type attestationAnchor struct {
	ids   []string
	count int
}

type attestationReference struct {
	family                      attestationFamily
	requiresManufacturerConfirm bool
}

func newProvenanceContext(attestations []any) provenanceContext {
	return provenanceContext{
		anchors:    familyAnchors(attestations),
		references: attestationReferences(attestations),
	}
}

func requiresManufacturerConfirmation(attestation map[string]any) bool {
	verification, _ := attestation["verification"].(map[string]any)
	typeName, _ := verification["type"].(string)
	return typeName == "requires_manufacturer_confirmation"
}

// familyAnchors groups attestations through the authored program table and
// sorts each family's ids so resolution stays deterministic.
func familyAnchors(attestations []any) map[attestationFamily]attestationAnchor {
	anchors := map[attestationFamily]attestationAnchor{}
	for _, value := range attestations {
		attestation, ok := value.(map[string]any)
		if !ok {
			continue
		}
		program, _ := attestation["program"].(string)
		family, anchorsValues := programFamilies[program]
		if !anchorsValues || requiresManufacturerConfirmation(attestation) {
			continue
		}
		anchor := anchors[family]
		anchor.count++
		if id, _ := attestation["attestation_id"].(string); id != "" {
			anchor.ids = append(anchor.ids, id)
		}
		anchors[family] = anchor
	}
	for family, anchor := range anchors {
		sort.Strings(anchor.ids)
		anchors[family] = anchor
	}
	return anchors
}

func attestationReferences(attestations []any) map[string][]attestationReference {
	references := map[string][]attestationReference{}
	for _, value := range attestations {
		attestation, ok := value.(map[string]any)
		if !ok {
			continue
		}
		id, _ := attestation["attestation_id"].(string)
		if id == "" {
			continue
		}
		program, _ := attestation["program"].(string)
		references[id] = append(references[id], attestationReference{
			family:                      programFamilies[program],
			requiresManufacturerConfirm: requiresManufacturerConfirmation(attestation),
		})
	}
	return references
}
