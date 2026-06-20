## Compliance and attestation program glossary

This glossary is the reference for every `AttestationProgram` token a ULC record may claim. It is a companion to [methodology.md](methodology.md), which explains why compliance badges are modeled as an orthogonal axis (the attestations block) rather than as a conformance-level gate. Each program below is an external listing, certification, mark, declaration, or test-method standard. ULC references every one of them by identifier only: it does not, and by the metadata-only principle cannot, redistribute the text of any paid or restricted standard. A token on a record is a pointer to evidence the manufacturer holds, not a copy of that evidence.

The 146 tokens are split across two subtables. Subtable A covers the compliance and listing badges (safety listings, market-access marks, energy and environmental programs, origin and trade-body claims). Subtable B covers the test-method attestations (the IES/ANSI/IEEE/NEMA/NTCIP/CSA measurement and method standards). Together the two subtables enumerate the complete `AttestationProgram` enum with no omissions.

Two columns carry the load. **Category** sorts each badge into one governance family. **Conformance role** states whether the reference grader reads the token. It takes exactly one of two values: `core-eligible safety listing` (the grader's core safety gate is satisfied when this token is present) or `tracked` (the token is recorded for catalog and search and never affects the conformance level). The verification standing of any claim, genuine or not, lives separately in the attestation's own `AttestationStatus` field (`claimed`, `verified`, `listed`, `audited_member`, `provisional_member`, `expired`, `withdrawn`, `not_applicable`).

**Conformance grading checks for the presence of a self-asserted attestation claim, not third-party verification of that claim. A `core-eligible safety listing` row means: if the record lists this program token, the core safety gate is satisfied. ULC does not, and cannot, verify the listing is genuine; verification state lives in the attestation's own `AttestationStatus` (`claimed` / `verified` / `requires_manufacturer_confirmation`). A ULC conformance level is a data-completeness grade, never a safety certification.**

### Subtable A: compliance and listing badges

| token | Full name | What it certifies | Governing program / standard | Category | Conformance role |
| --- | --- | --- | --- | --- | --- |
| `ul_listed` | UL Listed | Product safety listing by UL Solutions; the product was evaluated and is listed in UL Product iQ. | UL Solutions (NRTL) against the applicable UL safety standard | safety-listing | `core-eligible safety listing` |
| `c_ul_listed` | cULus / C-UL Listed | UL listing carrying the Canadian (c) mark, indicating evaluation to Canadian safety requirements as well as US. | UL Solutions against CSA/Canadian-adopted standards | safety-listing | `core-eligible safety listing` |
| `etl` | ETL Listed (Intertek) | Product safety listing by Intertek's ETL mark, an OSHA-recognized NRTL alternative to UL. | Intertek (NRTL) against the applicable UL/CSA standard | safety-listing | `core-eligible safety listing` |
| `csa_listed` | CSA Listed / Certified | Product safety certification by CSA Group, recognized in Canada and the US. | CSA Group (NRTL / SCC) | safety-listing | `core-eligible safety listing` |
| `met_listed` | MET Listed (MET Labs) | Product safety listing by MET Laboratories, the first OSHA-recognized NRTL. | MET Labs / Eurofins (NRTL) | safety-listing | `core-eligible safety listing (North America)` |
| `nrtl_osha_recognized` | NRTL (OSHA-recognized) | Generic claim that the product is listed by a Nationally Recognized Testing Laboratory recognized under OSHA 29 CFR 1910.7. | OSHA NRTL program | safety-listing | `core-eligible safety listing` |
| `tuv` | TUV mark | Product safety certification by a TUV body (TUV Rheinland, TUV SUD, TUV Nord). | TUV against IEC/EN luminaire safety standards | safety-listing | `core-eligible safety listing` |
| `cb_scheme` | IECEE CB Scheme | CB Test Certificate and report enabling mutual recognition of product-safety test results across IECEE member bodies. | IECEE CB Scheme (IEC standards) | safety-listing | `core-eligible safety listing` |
| `ul_1598` | UL 1598 | Safety standard for luminaires (the core US luminaire safety standard). | UL 1598 (Luminaires) | safety-listing | `core-eligible safety listing` |
| `ce` | CE marking | Manufacturer's declaration of conformity with applicable EU directives (Low Voltage, EMC, RoHS) for sale in the EEA. | EU CE marking framework | safety-listing | `core-eligible safety listing` |
| `ukca` | UKCA marking | UK Conformity Assessed mark; the Great Britain equivalent of CE marking post-Brexit. | UK product-safety regulations | safety-listing | `core-eligible safety listing` |
| `enec` | ENEC mark | European Norms Electrical Certification, a pan-European third-party safety mark to EN standards. | ENEC scheme (EN 60598 family) | safety-listing | `core-eligible safety listing` |
| `iec_60598` | IEC 60598 | International safety standard for luminaires (general and particular requirements). | IEC 60598 series | safety-listing | `core-eligible safety listing` |
| `nom` | NOM mark | Mexican official-standard safety certification (Norma Oficial Mexicana) required for sale in Mexico. | NOM (Mexico) | safety-listing / market-access | `core-eligible safety listing` |
| `ccc` | CCC mark | China Compulsory Certification required for many products sold in mainland China. | CCC (China) | safety-listing / market-access | `core-eligible safety listing` |
| `rcm_australia` | RCM | Regulatory Compliance Mark for electrical safety and EMC in Australia and New Zealand. | RCM / ACMA + electrical regulators | safety-listing / market-access | `core-eligible safety listing` |
| `saa_australia` | SAA approval | Standards Australia approval mark for electrical equipment safety. | SAA / Australian electrical safety scheme | safety-listing / market-access | `core-eligible safety listing` |
| `kc_korea` | KC mark | Korea Certification mark for electrical and electronic equipment safety. | KC (Korea) | safety-listing / market-access | `core-eligible safety listing` |
| `pse_japan` | PSE mark | Product Safety Electrical Appliance and Material mark required for electrical products sold in Japan. | PSE / DENAN (Japan) | safety-listing / market-access | `core-eligible safety listing` |
| `ul_1574` | UL 1574 | Safety standard for track lighting systems. | UL 1574 (Track Lighting) | safety-listing | `tracked` |
| `ul_8750` | UL 8750 | Safety standard for LED equipment used in lighting products (the LED-component safety standard). | UL 8750 | safety-listing | `tracked` |
| `ul_924` | UL 924 | Safety standard for emergency lighting and power equipment. | UL 924 | safety-listing | `tracked` |
| `ul_2108` | UL 2108 | Safety standard for low-voltage lighting systems. | UL 2108 | safety-listing | `tracked` |
| `ul_1994` | UL 1994 | Safety standard for luminous egress-path marking systems. | UL 1994 | safety-listing | `tracked` |
| `iec_62031` | IEC 62031 | Safety standard for LED modules for general lighting. | IEC 62031 | safety-listing | `tracked` |
| `iec_62471` | IEC 62471 | Photobiological safety of lamps and lamp systems (blue-light and optical-radiation hazard groups). | IEC 62471 | safety-listing | `tracked` |
| `iec_61347` | IEC 61347 | Safety standard for lamp control gear (drivers and ballasts). | IEC 61347 | safety-listing | `tracked` |
| `atex` | ATEX | EU directive conformity for equipment in explosive (hazardous) atmospheres. | ATEX Directive 2014/34/EU | safety-listing | `tracked` |
| `iecex` | IECEx | International certification scheme for equipment used in explosive atmospheres. | IECEx scheme | safety-listing | `tracked` |
| `wet_location_ul` | UL Wet Location | Listing that the luminaire is rated for wet-location installation. | UL wet-location evaluation | safety-listing | `tracked` |
| `damp_location_ul` | UL Damp Location | Listing that the luminaire is rated for damp-location installation. | UL damp-location evaluation | safety-listing | `tracked` |
| `nsf_ansi_2` | NSF/ANSI 2 | Food-equipment sanitation standard; relevant to luminaires installed in food-handling zones. | NSF/ANSI 2 | safety-listing / food-zone | `tracked` |
| `fcc` | FCC | US conformity for unintentional/intentional radio-frequency emissions (EMC). | FCC 47 CFR Part 15/18 | EMC | `tracked` |
| `ices_canada` | ICES (Canada) | Innovation, Science and Economic Development Canada EMC requirements (the Canadian Part-15 analog). | ICES-003 / ICES-005 | EMC | `tracked` |
| `cispr_15` | CISPR 15 | International EMC standard for the radio-disturbance characteristics of lighting equipment. | CISPR 15 | EMC | `tracked` |
| `vcci_japan` | VCCI (Japan) | Voluntary Control Council for Interference EMC compliance for IT and electronic equipment in Japan. | VCCI (Japan) | EMC / market-access | `tracked` |
| `bis_india` | BIS (India) | Bureau of Indian Standards registration/certification for sale in India. | BIS (India) | market-access | `tracked` |
| `eac_eaeu` | EAC mark | Eurasian Conformity mark for the Eurasian Economic Union (Russia, Belarus, Kazakhstan, and others). | EAEU TR CU technical regulations | market-access | `tracked` |
| `inmetro_brazil` | INMETRO (Brazil) | Brazilian conformity certification administered by INMETRO. | INMETRO (Brazil) | market-access | `tracked` |
| `saso_saudi` | SASO (Saudi Arabia) | Saudi Standards, Metrology and Quality Organization conformity (SABER/Saleem) for the Saudi market. | SASO (Saudi Arabia) | market-access | `tracked` |
| `darksky_approved` | DarkSky Approved | Certification that a luminaire meets DarkSky International criteria for minimizing light pollution. | DarkSky International (formerly IDA) | environmental / energy | `tracked` |
| `dlc_standard` | DLC Standard (SSL) | DesignLights Consortium Solid-State Lighting qualification at the Standard performance tier. | DLC SSL Technical Requirements | energy | `tracked` |
| `dlc_premium` | DLC Premium (SSL) | DLC SSL qualification at the higher Premium performance tier. | DLC SSL Technical Requirements | energy | `tracked` |
| `dlc_qpl` | DLC QPL listing | Presence on the DLC Qualified Products List (generic qualification claim). | DLC Qualified Products List | energy | `tracked` |
| `dlc_nlc` | DLC Networked Lighting Controls | DLC qualification of networked lighting control systems. | DLC NLC Technical Requirements | energy | `tracked` |
| `dlc_luna` | DLC LUNA | DLC Light Usage for Night Applications qualification for outdoor luminaires that limit light pollution (sky glow and trespass) while meeting SSL efficiency. | DLC LUNA Technical Requirements | energy / environmental | `tracked` |
| `dlc_horticultural` | DLC Horticultural | DLC qualification for horticultural (plant-growth) lighting on photon efficacy and related metrics. | DLC Horticultural Technical Requirements | energy | `tracked` |
| `energy_star` | ENERGY STAR | EPA energy-efficiency program mark. Note: the EPA sunset the general ENERGY STAR Lamps and Luminaires specifications effective December 31, 2024 (the residential luminaires specification was retired); recessed downlights were carved out and continue under the separate Downlights specification (see `energy_star_downlights_v1`). A legacy `energy_star` claim on older datasheets references the retired luminaires program. | ENERGY STAR (US EPA) | energy | `tracked` |
| `energy_star_downlights_v1` | ENERGY STAR Downlights v1.0 | The downlights-specific ENERGY STAR specification (recessed downlights and retrofit kits) that replaced the retired luminaires specification; finalized 2023, fully effective January 1, 2025. | ENERGY STAR Downlights Specification v1.0 | energy | `tracked` |
| `ja8_title_24` | California Title 24 JA8 | California Building Energy Efficiency Standards, Joint Appendix JA8 high-efficacy light-source requirements. | California Title 24, Part 6, JA8 | energy / building-code | `tracked` |
| `ca_title_20` | California Title 20 | California Appliance Efficiency Regulations (Title 20) for lamps and related products. | California Title 20 (CEC) | energy | `tracked` |
| `eu_ecodesign_2019_2020` | EU Ecodesign (2019/2020) | EU Ecodesign requirements for light sources and separate control gear. | Commission Regulation (EU) 2019/2020 | energy | `tracked` |
| `eu_energy_label_2019_2015` | EU Energy Label (2019/2015) | EU energy-labelling requirements for light sources (the rescaled A-G label and EPREL registration). | Commission Regulation (EU) 2019/2015 | energy | `tracked` |
| `ftc_lighting_facts` | FTC Lighting Facts | US Federal Trade Commission Lighting Facts label disclosure for consumer lamps. | FTC Lighting Facts (16 CFR Part 305) | energy | `tracked` |
| `doe_led_lighting_facts` | DOE LED Lighting Facts | US Department of Energy LED Lighting Facts program disclosure (distinct from the FTC consumer label). | DOE LED Lighting Facts | energy | `tracked` |
| `nrcan_ee_regulations` | NRCan Energy Efficiency Regulations | Natural Resources Canada energy-efficiency regulations for regulated lighting products. | Canada Energy Efficiency Regulations (NRCan) | energy | `tracked` |
| `reach` | REACH (general) | EU Registration, Evaluation, Authorisation and Restriction of Chemicals compliance (general claim). | EU Regulation (EC) 1907/2006 | environmental | `tracked` |
| `reach_svhc` | REACH SVHC | Specific declaration regarding Substances of Very High Concern on the REACH candidate list (the SCIP/SVHC disclosure). | REACH SVHC candidate list | environmental | `tracked` |
| `rohs` | RoHS | Restriction of Hazardous Substances in electrical and electronic equipment. | EU Directive 2011/65/EU (RoHS) | environmental | `tracked` |
| `weee` | WEEE | Waste Electrical and Electronic Equipment take-back and recycling compliance. | EU Directive 2012/19/EU (WEEE) | environmental | `tracked` |
| `prop_65` | California Prop 65 | California Proposition 65 warning compliance for listed chemicals (Safe Drinking Water and Toxic Enforcement Act of 1986). | California Prop 65 (OEHHA) | environmental | `tracked` |
| `conflict_minerals_3tg` | Conflict Minerals (3TG) | Disclosure regarding conflict minerals (tin, tantalum, tungsten, gold) in the supply chain. | Dodd-Frank Section 1502 / OECD due diligence | environmental / origin | `tracked` |
| `pops` | POPs | Persistent Organic Pollutants restriction compliance. | EU Regulation (EU) 2019/1021 (POPs) | environmental | `tracked` |
| `tsca` | TSCA | US Toxic Substances Control Act chemical-inventory and restriction compliance. | TSCA (US EPA) | environmental | `tracked` |
| `epd_iso_14025` | Environmental Product Declaration (ISO 14025) | A Type III environmental product declaration based on a life-cycle assessment, per ISO 14025. | ISO 14025 / ISO 21930 | environmental / sustainability | `tracked` |
| `hpd` | Health Product Declaration | An HPD disclosing product material contents and associated health hazards. | HPD Open Standard | sustainability | `tracked` |
| `declare` | Declare label | An ILFI Declare "nutrition label" disclosing ingredients, sourcing, and end-of-life. | ILFI Declare program | sustainability | `tracked` |
| `lbc_red_list_free` | LBC Red List Free | Declare tier: the product contains no Living Building Challenge Red List chemicals. | ILFI Declare / LBC Red List | sustainability | `tracked` |
| `lbc_red_list_approved` | LBC Red List Approved | Declare tier: the product complies with the Red List via approved exceptions. | ILFI Declare / LBC Red List | sustainability | `tracked` |
| `lbc_red_list_declared` | LBC Red List Declared | Declare tier: full ingredient disclosure, but the product contains one or more Red List chemicals. | ILFI Declare / LBC Red List | sustainability | `tracked` |
| `living_building_challenge` | Living Building Challenge | Building-level certification under the ILFI Living Building Challenge (referenced as a contributing product claim). | ILFI Living Building Challenge | sustainability / building-code | `tracked` |
| `living_product_challenge` | Living Product Challenge | ILFI product-level certification for net-positive, handprint-positive products. | ILFI Living Product Challenge | sustainability | `tracked` |
| `living_community_challenge` | Living Community Challenge | ILFI community-scale certification (referenced as a contributing product claim). | ILFI Living Community Challenge | sustainability / building-code | `tracked` |
| `just_label` | JUST label | An ILFI JUST social-justice and equity transparency label for the manufacturing organization. | ILFI JUST program | sustainability / trade-body | `tracked` |
| `well_building_standard` | WELL Building Standard | The WELL standard for building features that affect occupant health and well-being (referenced as a contributing product claim). | IWBI WELL Building Standard | sustainability / building-code | `tracked` |
| `leed_v4` | LEED v4 | USGBC Leadership in Energy and Environmental Design v4 (product contributes to credits). | USGBC LEED v4 | sustainability / building-code | `tracked` |
| `leed_v4_1` | LEED v4.1 | USGBC LEED v4.1 (product contributes to credits). | USGBC LEED v4.1 | sustainability / building-code | `tracked` |
| `leed_v5` | LEED v5 | USGBC LEED v5 (product contributes to credits). | USGBC LEED v5 | sustainability / building-code | `tracked` |
| `cradle_to_cradle` | Cradle to Cradle Certified | Multi-attribute product certification covering material health, product circularity, clean air and climate, water, and social fairness. | Cradle to Cradle Products Innovation Institute | sustainability | `tracked` |
| `greencircle_certified` | GreenCircle Certified | Third-party verification of sustainability claims (recycled content, life-cycle attributes, and similar). | GreenCircle Certified, LLC | sustainability | `tracked` |
| `epeat` | EPEAT | Registry of products meeting environmental-sustainability criteria managed by the Global Electronics Council. | EPEAT / Global Electronics Council | sustainability / environmental | `tracked` |
| `ul_ecologo` | UL ECOLOGO | UL multi-attribute environmental certification (formerly EcoLogo / Environmental Choice). | UL ECOLOGO (UL 2700-series criteria) | sustainability / environmental | `tracked` |
| `taa` | TAA compliant | Trade Agreements Act compliance: the product is made or substantially transformed in a TAA-designated country for US federal procurement. | Trade Agreements Act (19 U.S.C. 2501) | origin | `tracked` |
| `baa` | Buy American Act | Buy American Act compliance for US federal procurement (domestic-content thresholds). | Buy American Act (41 U.S.C. 8301) | origin | `tracked` |
| `baba` | Build America, Buy America | BABA Act domestic-sourcing requirements for federally funded infrastructure projects. | BABA Act (IIJA, 2021) | origin | `tracked` |
| `american_iron_and_steel` | American Iron and Steel | AIS requirement that iron and steel components be produced domestically (used in certain federally funded projects). | American Iron and Steel provisions | origin | `tracked` |
| `country_of_origin` | Country of Origin | A declared country-of-origin marking claim (generic origin statement). | Country-of-origin marking requirements | origin | `tracked` |
| `chicago_plenum` | Chicago Plenum | Compliance with the City of Chicago plenum requirements for products installed in air-handling (plenum) spaces. | Chicago Electrical Code (plenum provisions) | building-code | `tracked` |
| `lia_member` | LIA Member | Membership of the Lighting Industry Association (trade-body membership claim). | Lighting Industry Association | trade-body | `tracked` |
| `lia_audited` | LIA Audited | LIA-audited membership (a verified-membership tier above plain membership). | Lighting Industry Association | trade-body | `tracked` |
| `liaqa` | LIAQA | LIA Quality Assurance scheme participation. | LIA Quality Assurance | trade-body | `tracked` |
| `liasc_plus` | LIASC+ | LIA Supply Chain (LIASC+) verification of supply-chain practices. | LIA Supply Chain scheme | trade-body | `tracked` |
| `performance_verified` | Performance Verified | A LIA Performance Verified claim (independently checked headline performance). | LIA Performance Verified | trade-body | `tracked` |
| `tm66_assured` | TM66 Assured | Conformity with TM-66 circular-economy assessment for luminaires (the CIBSE/SLL/LIA TM66 framework), an LIA-administered assurance. | CIBSE TM-66 / LIA TM66 Assured | sustainability / trade-body | `tracked` |
| `tm65_2` | TM65.2 | Embodied-carbon assessment of building services equipment per CIBSE TM65 (the 2.x lighting-relevant edition). | CIBSE TM65 | sustainability / trade-body | `tracked` |
| `icel` | ICEL | Industry Committee for Emergency Lighting registration (UK emergency-lighting trade scheme). | ICEL (UK) | trade-body / safety-listing | `tracked` |
| `iso_9001` | ISO 9001 | Quality management system certification of the manufacturing organization. | ISO 9001 | trade-body | `tracked` |

### Subtable B: test-method attestations

These tokens reference measurement and method standards rather than safety listings or market badges. Per the schema, both a family label (for example `lm_79`, `tm_30`) and version-specific values (for example `lm_79_24`, `tm_30_24`) exist; claims should use the version-specific value whenever the revision is known, because numeric results may not be comparable across revisions. The reference grader reads only one family here: the `lm_79*` family (any token starting with `lm_79`) is the LM-79 attestation gate evaluated at the STANDARD tier. Every other token in this subtable is `tracked`.

| token | Full name | What it certifies | Governing program / standard | Category | Conformance role |
| --- | --- | --- | --- | --- | --- |
| `lm_79` | IES LM-79 (family) | Approved method for the electrical and photometric measurement of solid-state lighting products (revision unspecified). | IES LM-79 | test-method (photometry) | `tracked` (LM-79 family read at standard) |
| `lm_79_08` | IES LM-79-08 | LM-79, 2008 edition. | IES LM-79-08 | test-method (photometry) | `tracked` (LM-79 family read at standard) |
| `lm_79_19` | IES LM-79-19 | LM-79, 2019 edition. | ANSI/IES LM-79-19 | test-method (photometry) | `tracked` (LM-79 family read at standard) |
| `lm_79_24` | IES LM-79-24 | LM-79, 2024 edition (current). | ANSI/IES LM-79-24 | test-method (photometry) | `tracked` (LM-79 family read at standard) |
| `lm_75_19` | IES LM-75-19 | Goniophotometer types and photometric coordinate systems guidance. | ANSI/IES LM-75-19 | test-method (photometry) | `tracked` |
| `lm_78_20` | IES LM-78-20 | Total luminous flux measurement using an integrating sphere. | ANSI/IES LM-78-20 | test-method (photometry) | `tracked` |
| `lm_82_20` | IES LM-82-20 | Characterization of LED light engines and lamps for electrical and photometric properties as a function of temperature (thermal derating). | ANSI/IES LM-82-20 | test-method (thermal) | `tracked` |
| `lm_85_20` | IES LM-85-20 | Approved method for electrical and photometric measurement of high-power LEDs. | ANSI/IES LM-85-20 | test-method (photometry) | `tracked` |
| `lm_31_20` | IES LM-31-20 | Photometric testing of roadway and area luminaires using incandescent filament and HID lamps (method reference). | IES LM-31-20 | test-method (photometry) | `tracked` |
| `lm_35_20` | IES LM-35-20 | Photometric testing of floodlights using HID or incandescent filament lamps (method reference). | IES LM-35-20 | test-method (photometry) | `tracked` |
| `lm_80` | IES LM-80 (family) | Measuring luminous flux and color maintenance of LED packages, arrays, and modules (revision unspecified). | IES LM-80 | test-method (maintenance) | `tracked` |
| `lm_80_08` | IES LM-80-08 | LM-80, 2008 edition. | IES LM-80-08 | test-method (maintenance) | `tracked` |
| `lm_80_15` | IES LM-80-15 | LM-80, 2015 edition. | IES LM-80-15 | test-method (maintenance) | `tracked` |
| `lm_80_20` | IES LM-80-20 | LM-80, 2020 edition. | ANSI/IES LM-80-20 | test-method (maintenance) | `tracked` |
| `lm_80_21` | IES LM-80-21 | LM-80, 2021 edition (current). | ANSI/IES LM-80-21 | test-method (maintenance) | `tracked` |
| `lm_84` | IES LM-84 (family) | Measuring luminous flux and color maintenance of LED lamps, light engines, and luminaires (revision unspecified). | IES LM-84 | test-method (maintenance) | `tracked` |
| `lm_84_14` | IES LM-84-14 | LM-84, 2014 edition. | IES LM-84-14 | test-method (maintenance) | `tracked` |
| `lm_84_20` | IES LM-84-20 | LM-84, 2020 edition. | ANSI/IES LM-84-20 | test-method (maintenance) | `tracked` |
| `lm_84_20_e1` | IES LM-84-20 Errata 1 | LM-84-20 with Errata 1 (normative-content correction). | ANSI/IES LM-84-20 (Errata 1) | test-method (maintenance) | `tracked` |
| `tm_21` | IES TM-21 (family) | Projecting long-term lumen maintenance of LED light sources from LM-80 data (revision unspecified). | IES TM-21 | test-method (maintenance) | `tracked` |
| `tm_21_11` | IES TM-21-11 | TM-21, 2011 edition. | IES TM-21-11 | test-method (maintenance) | `tracked` |
| `tm_21_21` | IES TM-21-21 | TM-21, 2021 edition (current). | ANSI/IES TM-21-21 | test-method (maintenance) | `tracked` |
| `tm_28` | IES TM-28 (family) | Projecting long-term luminous flux maintenance of LED lamps and luminaires from LM-84 data (revision unspecified). | IES TM-28 | test-method (maintenance) | `tracked` |
| `tm_28_20` | IES TM-28-20 | TM-28, 2020 edition. | ANSI/IES TM-28-20 | test-method (maintenance) | `tracked` |
| `tm_35` | IES TM-35 (family) | Projecting long-term chromaticity shift of LED packages (revision unspecified). | IES TM-35 | test-method (chromaticity) | `tracked` |
| `tm_35_19` | IES TM-35-19 | TM-35, 2019 edition. | ANSI/IES TM-35-19 | test-method (chromaticity) | `tracked` |
| `tm_35_19_e1` | IES TM-35-19 Errata 1 | TM-35-19 with Errata 1 (normative-content correction). | ANSI/IES TM-35-19 (Errata 1) | test-method (chromaticity) | `tracked` |
| `tm_27_20` | IES TM-27-20 | Standard format for the electronic transfer of spectral data (the IES SPDX-style spectral file). | ANSI/IES TM-27-20 | test-method (color) | `tracked` |
| `tm_30` | IES TM-30 (family) | Evaluating light-source color rendition (Rf fidelity and Rg gamut) (revision unspecified; note the fidelity scaling changed between editions). | IES TM-30 | test-method (color) | `tracked` |
| `tm_30_15` | IES TM-30-15 | TM-30, 2015 edition. | IES TM-30-15 | test-method (color) | `tracked` |
| `tm_30_18` | IES TM-30-18 | TM-30, 2018 edition. | ANSI/IES TM-30-18 | test-method (color) | `tracked` |
| `tm_30_20` | IES TM-30-20 | TM-30, 2020 edition. | ANSI/IES TM-30-20 | test-method (color) | `tracked` |
| `tm_30_24` | IES TM-30-24 | TM-30, 2024 edition (current; adds the design-intent PVF framework). | ANSI/IES TM-30-24 | test-method (color) | `tracked` |
| `rp_46` | IES RP-46 (family) | Recommended practice for reporting the photobiological / circadian (alpha-opic) potency of light sources (revision unspecified). | IES RP-46 | test-method (melanopic) | `tracked` |
| `rp_46_23` | IES RP-46-23 | RP-46, 2023 edition, used with CIE S 026 alpha-opic action spectra. | ANSI/IES RP-46-23 | test-method (melanopic) | `tracked` |
| `tm_15` | IES TM-15 (family) | Luminaire Classification System (LCS) and the Backlight-Uplight-Glare (BUG) rating method (revision unspecified). | IES TM-15 | test-method (photometry) | `tracked` |
| `tm_15_11` | IES TM-15-11 | TM-15, 2011 edition. | IES TM-15-11 | test-method (photometry) | `tracked` |
| `tm_15_20` | IES TM-15-20 | TM-15, 2020 edition (current). | ANSI/IES TM-15-20 | test-method (photometry) | `tracked` |
| `lm_90_20` | IES LM-90-20 | Optical measurement of the temporal light modulation (flicker) waveform of lighting products. | ANSI/IES LM-90-20 | test-method (flicker) | `tracked` |
| `ieee_1789_2015` | IEEE 1789-2015 | Recommended practice for modulating current in high-brightness LEDs to mitigate health risks (the flicker risk-zone framework). | IEEE 1789-2015 | test-method (flicker) | `tracked` |
| `nema_77_2017` | NEMA 77-2017 | Temporal light artifacts (flicker and stroboscopic effects) test and reporting methods for SSL. | NEMA 77-2017 | test-method (flicker) | `tracked` |
| `cie_13` | CIE 13.3 | Method of measuring and specifying color-rendering properties of light sources (the CRI Ra method). | CIE 13.3 | test-method (color) | `tracked` |
| `ansi_c78_377_2024` | ANSI C78.377-2024 | Chromaticity specification (nominal CCT quadrangles) for solid-state lighting products. | ANSI C78.377-2024 | test-method (color) | `tracked` |
| `ansi_c78_377_2017` | ANSI C78.377-2017 | ANSI C78.377, 2017 edition (legacy nominal-CCT quadrangles). | ANSI C78.377-2017 | test-method (color) | `tracked` |
| `ansi_c136_25` | ANSI C136.25 | Roadway and area lighting equipment: ingress protection (resistance to dust, solid objects, and moisture) for luminaire enclosures and devices. | ANSI C136.25 | test-method (roadway) | `tracked` |
| `ansi_c136_31` | ANSI C136.31 | Roadway and area lighting equipment: luminaire vibration withstand capability and test methods. | ANSI C136.31 | test-method (roadway) | `tracked` |
| `csa_c653` | CSA C653 | Photometric performance (unit power density / energy performance) of roadway and street lighting luminaires. | CSA C653 | test-method (roadway) | `tracked` |
| `csa_c811` | CSA C811 | Performance (unit power density) of highmast luminaires for roadway lighting. | CSA C811 | test-method (roadway) | `tracked` |
| `ntcip_1213` | NTCIP 1213 | Object definitions for Electrical and Lighting Management Systems (ELMS): NTCIP data elements for monitoring and controlling roadway electrical and lighting systems. | NTCIP 1213 | test-method (roadway / controls) | `tracked` |
