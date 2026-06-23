## Methodology

This document explains why ULC is shaped the way it is. It sits above [authoring-patterns.md](authoring-patterns.md), which describes the concrete patterns and primitives, and complements [how-it-works.md](how-it-works.md), which walks through ULC end to end for a consumer. Here the concern is the reasoning: the core unit of the standard, the principles that shape the schema, and how the standard evolves.

## The core unit: one attested photometric scenario

A ULC record represents one attested photometric scenario for a luminaire. That definition is deliberate, and it does three things at once.

First, it pins each record to measurement evidence: an LM-79 test, or a declared derivative of one. A record is not a marketing summary; it is a normalized view of measured (or explicitly rated) performance with the provenance to prove it.

Second, it decouples records from orderable SKUs. A manufacturer may produce thousands of SKUs from one cutsheet through a configurator. Tying records to scenarios rather than SKUs means a single record can cover a wide range of orderable configurations through its applicability block, so a cutsheet that spans tens of thousands of theoretical SKUs does not force tens of thousands of ULC files.

Third, it decouples records from IES files. A manufacturer that ships many IES files derived from a smaller base-test set does not need a separate record per file unless the manufacturer chooses to materialize that 1:1 mapping. The scenario, not the file, is the unit.

Two links carry the weight of this decoupling. A structured applicability block connects a record to the orderable SKUs it describes. Provenance metadata on each value connects that value to the measurement evidence behind it. The four authoring patterns (A, B, C, D) show how different manufacturer data models map onto this single unit; see [authoring-patterns.md](authoring-patterns.md) for each pattern in detail rather than a summary here.

## Design principles that shape the schema

Each principle below is a deliberate constraint, with a short rationale for why it earns its place.

**Provenance-first.** Every value traces to its source, and carries a `value_type` of measured, rated, or nominal. This is what lets a consumer distinguish a physically tested figure from a manufacturer-rated projection from a nominal claim awaiting verification. Without it, cross-manufacturer comparison silently mixes evidence of different strengths.

**SI-authoritative dual-unit representation.** Physical quantities are carried in both SI and Imperial units, with SI authoritative. A single fixed policy for which unit governs avoids per-record ambiguity and makes derived scalars deterministic, so two readers extracting the same field always get the same number.

**Closed taxonomy with extensions for the long tail.** Vocabularies live in a closed enum set so that search, classification, and comparison are interoperable across manufacturers. The long tail of manufacturer-specific or experimental data has a defined home in the extensions mechanism, so the core stays comparable without forcing every edge case into a shared enum.

**Generated, not hand-authored, derived data.** The `index` block is a flat, denormalized summary of the most commonly queried values, and it is a deterministic projection of the deep blocks beneath it, produced by the reference builder rather than typed by a manufacturer. The computed conformance level is written into that same index. Because the index is a pure function of the deep blocks, drift is prevented by construction, and a hand-edited index value fails the builder-parity check. This mirrors precedent across the industry, where scan surfaces are generated from tooling rather than hand-authored.

**Metadata-only.** ULC references external standards and source files by identifier and content hash. It does not embed or redistribute the text of paid or restricted standards, and it does not bundle the source files themselves. The record points to evidence; it does not republish it.

**Explicit completeness.** Missing fields are explicit rather than hidden, and the computed conformance grade (core, standard, full, above an incomplete floor) makes a record's completeness legible at a glance. When every manufacturer used their own notion of what belongs on a datasheet, gaps were invisible. A common structure where absence is stated, and where the level is computed from the data actually present, surfaces data-quality issues that no single proprietary format ever exposed.

Taken together, these principles serve one goal: records that are comparable across manufacturers, with evidence a consumer can verify and completeness a consumer can read.

## Source documents, standards corpus, and compliance

A ULC record is the machine-readable union of data a manufacturer publishes across several separate documents. This section is the stable reference for which document each kind of data comes from, and for the standards and compliance programs that govern it. The conformance-level thresholds and the applicability predicates themselves are defined separately, in [The conformance rubric](#the-conformance-rubric) below.

### The source documents

ULC draws from the full document set a manufacturer produces, not the cutsheet alone. The two documents most often conflated are the IES/LDT photometric file and the accredited test report, and they are distinct. The IES (LM-63) file encodes a candela distribution at one operating point plus header keyword metadata. It does not encode measurement uncertainty, the list of correction factors applied, thermal-derating curves, TM-30 per-hue-bin detail, or lumen-maintenance projections. Those live only in the separate accredited test reports. A manufacturer can publish an IES file and still lack test-report depth.

| Document | SourceFileType token | What it carries |
| --- | --- | --- |
| Marketing cutsheet | `datasheet_pdf` | Identity, classification, mechanical, physical, and environmental data, catalog and order codes, and all rated or nominal headline specs. |
| Driver / LED-driver cutsheet | `driver_datasheet_pdf` | LED-driver electrical specifications (input voltage range, drive current, power factor, THD, dimming protocol and method, standby power, wiring) when the manufacturer publishes the driver on a separate sheet rather than folding it into the marketing cutsheet. It is an alternate source for the electrical fields and is needed only when those values are not already carried on the `datasheet_pdf`. |
| IES photometric file | `ies` | The candela distribution (maximum intensity, zonal lumens, coordinate system, symmetry, beam angle) plus LM-63-2019 header keywords (catalog, lamp, dimensions, absolute input watts, and lab name, report id, accreditation scheme). |
| EULUMDAT file | `ldt` | The European photometric distribution interchange. Distribution only for ULC: it corroborates flux, symmetry, coordinate system, and luminous-area dimensions, and carries no colorimetry, electrical, maintenance, or thermal depth. |
| Installation instructions | `installation_instructions_pdf` | The authoritative mounting, wiring, ceiling cutout, recess geometry, and weight detail that the datasheet summarizes only coarsely. |
| Compliance documents | `compliance_documents` | Safety and QPL listings, sustainability declarations (Declare, EPD, HPD), origin letters (BAA, TAA, BABA), RoHS and REACH declarations, and verification of IP, IK, and hazardous-location ratings. |
| Accredited test reports | `test_report` | The lab reports behind the cutsheet: LM-79 (full photometric and electrical, uncertainty, corrections), LM-80 with TM-21, LM-84 with TM-28, LM-82 (thermal), TM-30 (color rendition), TM-35 (chromaticity shift), and LM-90 (flicker). |
| Supplementary bulletins | `supplementary_pdf`, `article_text` | Manufacturer technical bulletins and computed photometric road reports (for example outdoor LCS and BUG tabulations). |

### Standards corpus reviewed

The taxonomy is built from a corpus of measurement standards: LM-79-24, LM-75-19, TM-30-24, LM-80-21, LM-84-20, TM-21-21, TM-28-20, RP-8-22, TM-15-20, LM-90-20, LM-82-20, RP-46-23 (with CIE S 026), and TM-35-19-e1, plus the non-IES references ANSI C78.377-2024, IEEE 1789-2015, CIE TN 006, NEMA 77, and GUM / ISO-IEC 17025. Commercial product-classification schemes such as ETIM and GLDF are not taxonomy sources; they are interoperability crosswalks tracked separately.

| Standard | Source document(s) |
| --- | --- |
| LM-79-24 photometric distribution | `ies`, `ldt`, `datasheet_pdf` |
| LM-79-24 electrical measurement | `ies`, `datasheet_pdf`, `test_report` |
| LM-79-24 uncertainty and corrections | `test_report` only |
| LM-79-24 lab attestation and instrumentation | `ies` (LM-63-2019 header), `test_report`, `compliance_documents` |
| LM-75-19 goniophotometer and coordinate systems | `ies` (coordinate token), `test_report` (goniometer type, stray light) |
| TM-30-24 color rendition | `datasheet_pdf` (headline Rf and Rg), `test_report` (hue bins, SPD) |
| LM-80-21 package lumen maintenance | `test_report` only |
| TM-21-21 package projection | `test_report`, `datasheet_pdf` (headline L70) |
| TM-28-20 luminaire projection | `test_report`, `datasheet_pdf` |
| LM-84-20 luminaire maintenance test | `test_report` only |
| LM-82-20 thermal characterization | `test_report` only |
| TM-15-20 and RP-8-22 BUG and LCS zonal | `datasheet_pdf`, `ies`, `test_report`, `supplementary_pdf` |
| TM-35-19-e1 chromaticity shift | `test_report` only |
| LM-90-20 flicker | `test_report` only |
| ANSI C78.377-2024 nominal CCT | `datasheet_pdf`, `ies`, `compliance_documents` |
| RP-46-23 and CIE S 026 circadian | `test_report`, `datasheet_pdf`, `supplementary_pdf` |
| Listings, certifications, origin, sustainability | `compliance_documents`, `datasheet_pdf` |
| Installation, mechanical, mounting | `installation_instructions_pdf`, `datasheet_pdf` |

### Taxonomy data to source document

Most taxonomy data is datasheet-sourced. The photometric distribution and a band of LM-63-2019 header metadata come from the IES/LDT file. A distinct, deeper band comes only from accredited test reports.

- From the datasheet: all of `product_family.*` (identity, mechanical, physical, environmental), all of `applicability.*` and `configuration.tested_axes.*` (catalog and order codes), and the rated or nominal specs in `colorimetry` (nominal CCT, SDCM step, rated CRI), `electrical` (voltage class, driver protocol, dimming, control gear type, rated power factor and THD), and the lumen-maintenance floor (declaration framework, manufacturer rated claim, CIE 97 LMF tables).
- From the IES file: the photometric distribution (total luminous flux, maximum intensity, efficacy, beam and field angles, zonal lumens) and the photometric enums (distribution type, symmetry type, photometric coordinate system, luminous opening shape); absolute input power; `test_conditions.photometry_basis` and file generation type; `instrumentation.measurement_regime` and the LM-63-2019 lab keywords (laboratory name, report id, certification, accreditation scheme); and the derived outdoor distribution type and longitudinal distribution range.
- From installation instructions: the deep mechanical detail the datasheet only summarizes (exact mounting, wiring, recess geometry, cable and driver dimensions).
- From compliance documents: the attestations and listings block and the sustainability declaration, plus verification of IP, IK, and hazardous-location ratings.
- From accredited test reports only: measured colorimetry (measured CCT, Duv, chromaticity, per-sample CRI, the full TM-30 block including per-hue-bin fidelity, the five-channel alpha-opic vector); measured electrical (input current, measured power factor and THD); the entire flicker block; package and luminaire lumen-maintenance projections; thermal derating; chromaticity-shift projection; corrections applied; measurement uncertainty; the deep operating-point qualifiers (drive current, ambient and case temperature, monitoring point, operating mode); and goniometer type.

### Compliance programs

Lighting compliance is a patchwork of independent, jurisdiction-by-jurisdiction programs. No single standard or document lists them. They fall into four governance families: NRTL safety listings (bodies recognized under OSHA 29 CFR 1910.7 and Canada's Standards Council), product-safety and EMC standards (UL, IEC, CISPR, and their regional adoptions), energy and performance programs (EPA ENERGY STAR, the separate DesignLights Consortium QPLs, the California Energy Commission, the FTC, the EU Commission, and NRCan), and environmental, material-health, and origin regimes (ECHA, US EPA and California OEHHA, ILFI, Cradle to Cradle, ISO 14025 program operators, and US federal procurement statutes). The closest partial unifiers are the IECEE CB Scheme on the safety side and LEED Materials and Resources credits on the materials side, but neither is a normative master list.

Compliance evidence is carried by `compliance_documents` (listing certificates, CB Test Certificates, Declarations of Conformity, Declare, EPD, and HPD labels, conflict-minerals reporting templates, and QPL printouts). In practice the artifact is often a logo or footnote on the `datasheet_pdf`, which is a claim, or a searchable database entry such as UL Product iQ, the DLC QPL, or EPREL, which is a listing. ULC therefore records an attestation status: a datasheet logo is a claim, a database entry is a listing, and a certificate in `compliance_documents` is verified. Compliance badges are an orthogonal axis: they say nothing about whether a fixture is photometrically characterized. Apart from the core safety listing, which the core tier gates on, they are modeled in the attestations block and tracked, never used as a conformance-level gate.

## The conformance rubric

Every ULC record carries a conformance level, and that level is computed, never declared. The reference builder grades the record from the fields actually populated and stamps the result into `index.conformance_level`; the builder-parity check then guards that value like any other index field. There is no place to assert a level by hand and no way to inflate one: a record simply is the highest tier all of whose hard requirements it meets. Grading walks the tiers from low to high and stops at the first tier with an unmet requirement.

There are three ordered grades, `core` < `standard` < `full`, sitting above an `incomplete` floor.

- **`incomplete`.** The floor: a record that has not yet met a core requirement. It grades incomplete, indexes, and carries a roadmap to core; a record missing identity (the schema-required `product_family.manufacturer.slug`, `product_family.family_id`, and `product_family.catalog_model`) is malformed rather than incomplete: it fails JSON Schema validation, and the builder cannot derive its required index keys (`manufacturer_slug`, `catalog_model`), reported through `MissingRequiredKeys`.
- **`core`.** A complete, identifiable, legally-sellable luminaire with headline numbers: the identity, classification, headline photometric and electrical values, one-line colorimetry, and a market safety listing that a buyer needs before specifying the fixture.
- **`standard`.** Core plus the fuller specification an LM-79 report produces: the maximum intensity, symmetry and coordinate system, materials, the test conditions and measurement regime, an LM-79 attestation, a lumen-maintenance framework, and the conditional rows that apply to the fixture's form.
- **`full`.** Standard plus exhaustive accredited characterization: zonal lumens, an operating point, measurement uncertainty, corrections, instrumentation depth, a method-backed lumen-maintenance projection, and (for white-light fixtures) TM-30 detail. These are mostly fed by accredited test reports.

A requirement is only ever counted against a record when it applies. A conditional requirement whose predicate is false for a given record (CRI for a pure color-mixing fixture, a BUG rating for an indoor downlight) is dropped from grading entirely, never reported as missing.

### Why the tiers fall where they do

The cut points between the tiers are not arbitrary. They mirror the way a Division 26 construction specification escalates the evidence it asks of a manufacturer, which broadens across three classes as the specification tightens:

- **Mandatory product data, plus a safety listing.** The identity, electrical, and headline photometric facts, and a recognized listing marked for the installation, that every fixture must provide to be specified, ordered, and approved at all. This is the `core` floor.
- **Selection-grade performance specifications.** The values a designer compares to choose among already-listed products (fuller photometry, lumen maintenance, color quality, distribution, dimming, ingress rating), demanded conditionally by product type and application. This is the `standard` tier.
- **Independently-certified test reports and engineered proof.** Accredited-laboratory photometry, measurement uncertainty, applied corrections, and method-backed projections, required only for the most rigorous or specifically scheduled fixtures. This is the `full` tier.

Two axes order the whole structure. The first is **evidence depth**: each tier asks for a deeper class of characterization, from the headline facts every fixture must publish, to the selection-grade performance a datasheet carries, to the accredited-laboratory detail (measurement uncertainty, applied corrections, method-backed projections) that in practice only a test report supplies, which is why `full` sits above the datasheet-grade `standard` band. Provenance runs alongside this axis but does not set the level: ULC records the pedigree of every value through `value_type`, `provenance.source`, and `attestation_ref` so a consumer can weigh a measured figure against a rated or simulated one, and a non-`measured` headline value is surfaced as an observation rather than used to gate the tier. The second is **conditional applicability**: a specification asks for a metric only when the fixture's form calls for it (a backlight-uplight-glare rating only outdoors, a MacAdam step only for white light), which is precisely what the applicability-predicate layer below encodes. A record therefore earns the tier whose evidence a specifier would actually demand of that fixture, which is what makes the computed level meaningful rather than nominal.

### Core requirements (all universal)

| Requirement | Field | Source |
| --- | --- | --- |
| Manufacturer slug and display name | `product_family.manufacturer.slug`, `.display_name` | `datasheet_pdf` |
| Catalog model | `product_family.catalog_model` | `datasheet_pdf` |
| Cutsheet | `product_family.cutsheet` | `datasheet_pdf` |
| Primary category | `product_family.primary_category` | `datasheet_pdf` |
| Indoor / outdoor | `product_family.indoor_outdoor` | `datasheet_pdf` |
| Secondary function | `product_family.secondary_function` | `datasheet_pdf` |
| Mounting types | `product_family.mounting_types` | `datasheet_pdf` |
| Environment rating | `product_family.environment_rating` | `datasheet_pdf` |
| Shape | `product_family.shape` | `datasheet_pdf` |
| Technical region | `product_family.technical_region` | `datasheet_pdf` |
| Distribution type | `photometry.distribution_type` | `ies` |
| Color tunability | `configuration.tested_axes.color_tunability` | `datasheet_pdf` |
| Driver protocol | `electrical.driver_protocol` | `datasheet_pdf` |
| Total luminous flux | `photometry.total_luminous_flux_lm` | `ies` |
| Input power | `electrical.input_power_w` | `ies` |
| Luminaire efficacy | `photometry.luminaire_efficacy_lm_per_w` | `ies` |
| Input voltage (value or class) | `electrical.input_voltage_v` or `.input_voltage_class` | `datasheet_pdf` |
| Nominal CCT (conditional: has a white point) | `colorimetry.nominal_cct_k` | `datasheet_pdf` |
| CRI Ra (conditional: primarily white light) | `colorimetry.cri_ra` | `datasheet_pdf` |
| Market safety listing (region-conditional) | a safety-listing attestation | `compliance_documents` |

The market safety listing is region-aware: a North American record (technical region `120v_60hz_north_america`) must claim a North-American-recognized listing (UL, cUL, ETL, CSA, MET, a generic NRTL claim, or UL 1598); every other technical region, including the cross-market `universal`, accepts any recognized listing, CE, ENEC, or IEC 60598 among them, to satisfy the gate.

### Standard requirements

Universal rows:

| Requirement | Field | Source |
| --- | --- | --- |
| Maximum intensity | `photometry.maximum_intensity_cd` | `ies` |
| Symmetry type | `photometry.symmetry_type` | `ies` |
| Photometric coordinate system | `photometry.photometric_coordinate_system` | `ies` |
| Control gear type | `electrical.control_gear_type` | `datasheet_pdf` |
| Housing material | `product_family.shared_mechanical.housing_material` | `datasheet_pdf` |
| Lens material | `product_family.shared_mechanical.lens_material` | `datasheet_pdf` |
| Photometry basis | `test_conditions.photometry_basis` | `ies` |
| Measurement regime | `instrumentation.measurement_regime` | `ies` |
| LM-79 attestation | an `lm_79*` attestation | `test_report` |
| Lumen-maintenance framework | `lumen_maintenance_luminaire` or `lumen_maintenance_package` | `datasheet_pdf` |

Conditional rows (each names the predicate that switches it on):

| Requirement | Field | Applies when (predicate) |
| --- | --- | --- |
| Beam angle | `photometry.beam_angle_deg` | directional fixture |
| Per-length normalized | `photometry.per_length_normalized` | linear fixture |
| Declared-by-length | `photometry.declared_by_length` | linear fixture |
| CRI tier | `configuration.tested_axes.cri_tier` | primarily white light |
| SDCM step (MacAdam) | `colorimetry.sdcm_step` | primarily white light |
| Dimming method | `electrical.dimming_method` | analog or phase-cut driver |
| Dimming range | `electrical.dimming_range_percent` | analog or phase-cut driver |
| IP rating | `product_family.shared_mechanical.ip_rating` | wet or outdoor-exposed |
| Outdoor distribution type | `outdoor_classification.outdoor_distribution_type` | outdoor-site fixture |
| Longitudinal range | `outdoor_classification.longitudinal_distribution_range` | outdoor-site fixture |
| BUG rating | `outdoor_classification.bug_rating` | outdoor-site fixture |

The dimming method and dimming range gates apply only to analog and phase-cut drivers (0-10V, 1-10V, phase, and PWM-input), whose dim floor and electrical method are published driver specifications a designer selects on. Digital control protocols (the DALI family, DMX, DSI), wireless protocols, and non-dimming drivers are exempt: their dim behavior is commanded externally or is not conventionally printed on the cutsheet, so requiring it would penalize a complete datasheet for a value it does not carry.

### Full requirements

| Requirement | Field | Source |
| --- | --- | --- |
| Zonal lumens | `photometry.zonal_lumens` | `ies` |
| Operating point | `operating_point` | `test_report` |
| Measurement uncertainty | `uncertainty` | `test_report` |
| Corrections applied | `corrections_applied` | `test_report` |
| Instrumentation depth | goniometer / lab fields on `instrumentation` | `test_report` |
| Method-backed lumen maintenance | TM-21 projection hours or a TM-28 projection | `test_report` |
| TM-30 fidelity Rf (conditional: primarily white light) | `colorimetry.tm_30.rf` | `test_report` |
| TM-30 per-hue-bin detail (conditional: primarily white light) | `colorimetry.tm_30.rf_h_per_bin` | `test_report` |

`zonal_lumens` sits at full rather than standard because it is IES-derived: it is computed from the candela grid, not a value a manufacturer publishes on a datasheet, so it belongs with the exhaustive accredited characterization rather than the LM-79 specification band.

### Observations (non-gating)

A separate band of fields is surfaced as observations once a record reaches core, but never gates the level. The reference validator hides them from text output unless `--verbose` is passed. They are: power factor, warranty term, luminous opening shape, emission face, Duv, chromaticity x and y, ambient operating range, compatible accessories, thermal derating, flicker, alpha-opic and circadian metrics, chromaticity-shift projection, field angle, cutoff angle, spacing criterion, UGR, LCS zonal lumens, legacy cutoff, IK rating, and EPA, plus a note when a headline photometric value (flux, power, or maximum intensity) carries a `value_type` other than `measured` (a rated figure or an optical simulation).

### Applicability predicates

Not every requirement applies to every fixture. A pure color-mixing fixture has no CRI, an indoor downlight has no BUG rating, a DALI or DMX fixture publishes no analog dim range, and an indoor fixture needs no IP rating. The predicate layer encodes this conditional applicability: each conditional rule carries a predicate, and when the predicate is false for a record the rule is dropped from grading entirely, never reported as a missing field. Without it the grader would demand metrics that are meaningless for a fixture's form and under-grade honest records.

Two invariants keep the layer safe. First, every predicate reads only core fields (primary category, color tunability, indoor or outdoor, environment rating, technical region, driver protocol, mounting types; the market-safety predicate additionally reads the attestations, which are themselves the core safety rule). Grading walks the tiers low to high, so a predicate that decides a standard- or full-tier rule must not read a standard- or full-tier field, or adding one field could change which higher-tier rules apply and make the level order-dependent. The reference tests assert this behaviorally: stripping every non-core field from a record must leave each predicate's value unchanged. Second, each predicate is pure and total, so it never panics on a malformed record, because grading runs before full schema validation on the spreadsheet authoring path.

This layer replaced an earlier handful of inline conditionals, and three corrections made it right: the single white-light test was split into `hasWhitePoint` (which still gates CCT, because an RGBW white channel has a CCT) and `isWhiteLightPrimary` (which gates CRI, SDCM, and TM-30 and excludes every color-mixing mode); the BUG gate was narrowed from any outdoor fixture to a specific outdoor-site category set, so beam-characterized architectural uplights are not asked for an area distribution they do not have; and `wetOrExposed` stopped reading `ip_rating` (a standard field it was gating) so the core-fields-only invariant holds.

| Predicate | Reads (core fields only) | True when | Gates |
| --- | --- | --- | --- |
| `hasWhitePoint` | `configuration.tested_axes.color_tunability` | in {static_white, tunable_white, dim_to_warm, rgbw, rgbww} | nominal CCT (core); Duv and chromaticity x/y (observations) |
| `isWhiteLightPrimary` | `color_tunability` | in {static_white, tunable_white, dim_to_warm} only | CRI Ra (core); CRI tier and SDCM step (standard); TM-30 Rf and hue bins (full) |
| `directional` | `product_family.primary_category` | downlight, tracklight, cylinder, wall_washer, grazer, facade_projector, in_ground_uplight, sports_flood | beam angle (standard); field angle (observation) |
| `outdoorSite` | `primary_category` | flood_area_site, roadway_street, walkway, bulkhead_wall_pack, sports_flood | outdoor distribution type, longitudinal range, BUG (standard); LCS zonal and legacy cutoff (observations) |
| `linear` | `primary_category` | linear, cove | per-length normalized and declared-by-length (standard) |
| `requiresDimmingDetail` | `electrical.driver_protocol` | in {0-10v, 1-10v, phase_forward, phase_reverse, pwm} (analog and phase-cut) | dimming method and dimming range (standard) |
| `wetOrExposed` | `environment_rating`, `indoor_outdoor` | env in {wet, marine_coastal, outdoor_rated}, or indoor_outdoor in {outdoor, both}; excludes `damp` | IP rating (standard) |
| `impactPublic` | `environment_rating` | == `vandal_resistant` | IK rating (observation only) |
| `poleMounted` | `mounting_types` | contains a pole or mast token | EPA (observation only) |
| market safety (`hasMarketSafetyListing`) | `technical_region` + attestation programs | a region-appropriate safety listing is present | the core safety-listing gate (region-conditional accept sets) |

The first seven predicates plus the market-safety check gate hard requirements; `impactPublic` and `poleMounted` gate observations only.

Compliance beyond the core safety listing is tracked, never gated: every other listing, certification, energy program, and declaration lives in the attestations array and `index.attestation_programs[]`, and the grader ignores it.

A note on what the safety gate proves. Conformance grading checks for the presence of a self-asserted safety-listing claim, not third-party verification of it; a conformance level is a data-completeness grade, never a safety certification. The verification standing of any claim lives separately in the attestation's own `AttestationStatus`. See the [compliance and attestation program glossary](compliance-attestation.md) for the per-program conformance role and status model.

Two categorization choices in the predicates are worth spelling out, because they decide which fixtures owe outdoor-site data and which owe white-light data.

The first is the outdoor-site boundary. An `in_ground_uplight` and a `facade_projector` are graded as directional (beam-characterized), not as outdoor-site, even though both are installed outdoors. They have no Type I-V area distribution, and a BUG rating is undefined for a fixture whose whole architecture is intended uplight, so requiring outdoor distribution type, longitudinal range, and a BUG rating of them would demand metrics that do not exist for the form. Area, roadway, walkway, wall-pack, and sports-flood fixtures do carry a Type I-V distribution and a meaningful BUG rating, so those are the outdoor-site categories that owe the standard-tier outdoor rows.

The second is the white-point versus white-light-primary split. A CCT is meaningful for any fixture with a white point, including the white channel of an RGBW or RGBWW fixture, so `hasWhitePoint` gates nominal CCT broadly. CRI, SDCM, and TM-30 are white-light-quality metrics that a color-mixing fixture does not characterize: an RGBW fixture is specified by its mixing gamut, not a single CRI figure measured against a reference illuminant. So `isWhiteLightPrimary` excludes every color-mixing mode (RGBW included), and those quality metrics are waived for color-mixing fixtures rather than counted against them.

## How the standard evolves

ULC changes through the Schema Change Proposal (SCP) process described in `CONTRIBUTING.md` and `GOVERNANCE.md`. A proposal is opened as an issue that states what the change is, why it is needed, which records and tools it affects, and whether it is backward-compatible. After discussion and maintainer feedback, an accepted proposal lands as a pull request that updates the schema, at least one example, the relevant documentation, and the changelog together.

Two practices keep that evolution honest. Changes are grounded in real manufacturer cutsheets: example records are sourced from real spec sheets and real IES files, not invented data, so every field that enters the schema is motivated by a real product or a real workflow gap. And the schema is additive by default. New fields and enums extend the standard without breaking existing records; when a constraint genuinely needs tightening, the change is documented explicitly and chosen so that existing valid records remain valid wherever possible.

The result is a standard that grows from the evidence rather than ahead of it. For the consumer-facing walkthrough, see [how-it-works.md](how-it-works.md); for the patterns and primitives, see [authoring-patterns.md](authoring-patterns.md); for governance and the guiding principles, see `GOVERNANCE.md`.
