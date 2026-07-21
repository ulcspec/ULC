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

**Generated, not hand-authored, derived data.** The `index` block is a flat, denormalized summary of the most commonly queried values, and it is a deterministic projection of the deep blocks beneath it, produced by the reference builder rather than typed by a manufacturer. Both computed views are written into that same index: the conformance level (`index.conformance_level`) and the Product Achievements axis (`index.achievements`, with its `index.restricted_substances_declared` sibling flag). Because the index is a pure function of the deep blocks, drift is prevented by construction, and a hand-edited index value fails the builder-parity check.

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

Compliance evidence is carried by `compliance_documents` (listing certificates, CB Test Certificates, Declarations of Conformity, Declare, EPD, and HPD labels, conflict-minerals reporting templates, and QPL printouts). In practice the artifact is often a logo or footnote on the `datasheet_pdf`, which is a claim, or a searchable database entry such as UL Product iQ, the DLC QPL, or EPREL, which is a listing. ULC therefore records an attestation status: a datasheet logo is a claim, a database entry is a listing, and a certificate in `compliance_documents` is verified. Compliance badges are an orthogonal axis: they say nothing about whether a fixture is photometrically characterized. Apart from the core safety listing, which the core tier gates on, they never gate the conformance level; they are modeled in the attestations block, and the third-party program qualifications among them are computed into the Product Achievements view, the second grading axis described in [The two grading axes](#the-two-grading-axes-completeness-and-achievements) below.

## The conformance rubric

Every ULC record carries a conformance level, and that level is computed, never declared. The reference builder grades the record from the fields actually populated and stamps the result into `index.conformance_level`; the builder-parity check then guards that value like any other index field. There is no place to assert a level by hand and no way to inflate one: a record simply is the highest tier all of whose hard requirements it meets. Grading walks the tiers from low to high and stops at the first tier with an unmet requirement.

There are three ordered grades, `core` < `standard` < `full`, sitting above an `incomplete` floor.

- **`incomplete`.** The floor: a record that has not yet met a core requirement. It grades incomplete, indexes, and carries a roadmap to core. A record missing the schema-required identity keys is malformed, a rejection distinct from an incomplete grade; [how-it-works.md](how-it-works.md#validation-and-the-two-computed-views) covers that boundary.
- **`core`.** A complete, identifiable, legally-sellable luminaire with headline numbers: the identity, classification, headline photometric and electrical values, one-line colorimetry, and a market safety listing that a buyer needs before specifying the fixture.
- **`standard`.** Core plus the fuller specification an LM-79 report produces: the maximum intensity, symmetry and coordinate system, materials, the test conditions and measurement regime, an LM-79 attestation, a lumen-maintenance framework, and the conditional rows that apply to the fixture's form.
- **`full`.** Standard plus exhaustive accredited characterization: zonal lumens, an operating point, measurement uncertainty, corrections, instrumentation depth, a method-backed lumen-maintenance projection, and (for white-light fixtures) TM-30 detail. These are mostly fed by accredited test reports.

A requirement is only ever counted against a record when it applies. A conditional requirement whose predicate is false for a given record (CRI for a pure color-mixing fixture, a BUG rating for an indoor downlight) is dropped from grading entirely, never reported as missing.

### Why the tiers fall where they do

The cut points between the tiers are not arbitrary. They mirror the way a Division 26 construction specification escalates the evidence it asks of a manufacturer, which broadens across three classes as the specification tightens:

- **Mandatory product data, plus a safety listing.** The identity, electrical, and headline photometric facts, and a recognized listing marked for the installation, that every fixture must provide to be specified, ordered, and approved at all. This is the `core` floor.
- **Selection-grade performance specifications.** The values a designer compares to choose among already-listed products (fuller photometry, lumen maintenance, color quality, distribution, dimming, ingress rating), demanded conditionally by product type and application. This is the `standard` tier.
- **Independently-certified test reports and engineered proof.** Accredited-laboratory photometry, measurement uncertainty, applied corrections, and method-backed projections, required only for the most rigorous or specifically scheduled fixtures. This is the `full` tier.

Two orderings structure the completeness rubric. The first is **evidence depth**: each tier asks for a deeper class of characterization, from the headline facts every fixture must publish, to the selection-grade performance a datasheet carries, to the accredited-laboratory detail (measurement uncertainty, applied corrections, method-backed projections) that in practice only a test report supplies, which is why `full` sits above the datasheet-grade `standard` band. Provenance runs alongside this ordering but does not set the level: ULC records the pedigree of every value through `value_type`, `provenance.source`, and `attestation_ref` so a consumer can weigh a measured figure against a rated or simulated one, and a non-`measured` headline value is surfaced as an observation rather than used to gate the tier. The second is **conditional applicability**: a specification asks for a metric only when the fixture's form calls for it (a backlight-uplight-glare rating only outdoors, a MacAdam step only for white light), which is precisely what the applicability-predicate layer below encodes. A record therefore earns the tier whose evidence a specifier would actually demand of that fixture, which is what makes the computed level meaningful rather than nominal.

### What gates, and what only enriches

Not every fact a datasheet can carry belongs in the tiers. A taxonomy gates a tier only when it clears three tests. It must be **decision-critical**: a specifier needs it to select, order, or approve the fixture at that tier, not merely to characterize it more fully. It must be **universally available**: every fixture of the applicable form actually publishes it, so requiring it does not penalize an honest datasheet for a value it cannot carry. And its **provenance must fit the tier**: a datasheet-grade fact gates core or standard, while a value that in practice only an accredited test report supplies gates full. This is the same Division 26 escalation the tiers mirror, applied field by field.

Three failure modes follow from getting this wrong. Gating on a value not every fixture publishes under-grades honest records. Gating on characterization depth a specifier does not select on inflates the tier into a raw completeness score. And gating on a value whose provenance a datasheet cannot supply pushes authors toward invented data. A field that is genuinely useful but fails any of the three tests is therefore optional even when it applies, which is a distinct decision from the applicability-predicate layer below: applicability asks whether a metric is meaningful for a fixture's form, while this test asks whether a meaningful metric should gate the level at all. The enrichment roadmap exists for exactly the fields that pass the usefulness bar but not the gating bar.

Every taxonomy the schema defines therefore earns its place in one of three ways: it gates a tier (the tier roadmap), it deepens the datasheet (the enrichment roadmap), or it is structural machinery a graded field is built from (a provenance enum, a value-type, a file format, or the required member of an array item surfaced through its parent block). A taxonomy that fits none of these is dead weight, and the reference tests treat a newly-referenced enum that is neither graded nor consciously set aside as a drift error to be resolved.

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

The market safety listing is region-aware: a North American record (technical region `120v_60hz_north_america`) must claim a North-American-recognized listing (UL, cUL, ETL, CSA, MET, a generic NRTL claim, or UL 1598); every other technical region, including the cross-market `universal`, accepts any recognized listing, CE, ENEC, or IEC 60598 among them, to satisfy the gate. The emergency-lighting listing `ul_924` is recognized in every region's accept set, so an exit sign or emergency luminaire whose only safety listing is UL 924 still clears the core gate.

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

### The two-part roadmap: tier gaps and enrichment

Beyond the achieved level, the grader emits a roadmap in two parts. The first is the **tier roadmap**: for each grade the record has not yet reached, the specific hard fields it must add to get there, emitted as `conformance/gap` findings that name the source document and governing standard. The second is the **enrichment roadmap**: optional dimensions a record could disclose to deepen the datasheet even though they never change the grade, emitted as `conformance/enrichment` findings. Both are surfaced once a record reaches core.

The enrichment roadmap covers the depth a fuller datasheet carries but the tiers deliberately do not gate: power factor, warranty term, luminous opening shape, emission face, Duv, chromaticity x and y, ambient operating range, compatible accessories, thermal derating, flicker, alpha-opic and circadian metrics, chromaticity-shift projection, field angle, cutoff angle, spacing criterion, UGR, LCS zonal lumens, IK rating, EPA, installed orientation, optical radiation band, adaptive lighting modes, photometric file format, the TM-30 design-intent code, and the finer sub-fields of the photometry, test-condition, operating-point, lumen-maintenance, colorimetry, and electrical blocks. Each sub-field suggestion fires only when its parent block is genuinely populated and the field itself absent, so an absent block draws a single block-level suggestion rather than sub-field spam.

A small residual set stays as plain `conformance/observation` notes rather than joining the enrichment roadmap. There are four sources. Two fire when the field is absent, in the same "not disclosed" idiom as the enrichment rows, but are deliberately kept off the roadmap: a sustainability declaration (a tracked program attestation such as an EPD or Declare label, not the datasheet depth a specifier asks a manufacturer to add) and the deprecated legacy-cutoff classification (superseded by the BUG rating and LCS zonal lumens, retained only for lossless ingestion of legacy datasheets and a drop candidate for a future major version). The other two are a note when a headline photometric value (flux, power, or maximum intensity) carries a `value_type` other than `measured` (a rated figure or an optical simulation), and the attestation-coverage summary listing the compliance programs tracked on the record.

### Applicability predicates

Not every requirement applies to every fixture. A pure color-mixing fixture has no CRI, an indoor downlight has no BUG rating, a DALI or DMX fixture publishes no analog dim range, and an indoor fixture needs no IP rating. The predicate layer encodes this conditional applicability: each conditional rule carries a predicate, and when the predicate is false for a record the rule is dropped from grading entirely, never reported as a missing field. Without it the grader would demand metrics that are meaningless for a fixture's form and under-grade honest records.

Two invariants keep the layer safe. First, every **gating-row** predicate reads only core fields (primary category, color tunability, indoor or outdoor, environment rating, technical region, driver protocol, mounting types; the market-safety predicate additionally reads the attestations, which are themselves the core safety rule). The class profiles extend this to class-core fields: a sign's `illumination_mode` and a powered emergency product's `power_source` are core-required for the class whose gating rows read them (through `signMode` and the battery trio), so those reads stay within that class's core tier; the battery trio gates at standard only for the dedicated-emergency and internally-illuminated classes, where `power_source` is core, and surfaces as enrichment for an externally-illuminated combo sign, where it is not. Grading walks the tiers low to high, so a predicate that decides a standard- or full-tier hard rule must not read a standard- or full-tier field, or adding one field could change which higher-tier rules apply and make the level order-dependent. The enrichment and observation rows are non-gating, so their applicability predicates may read the presence of a parent block and non-core fields within it (a flicker sub-field is suggested only when a flicker block is present; an emergency-mode nudge fires only when `emergency_role` names a dual-mode product); because they never move the level, reading a non-core block or field there is harmless. The reference tests assert the core-fields-only invariant behaviorally over the gating rows: stripping every non-core field from a record must leave each gating predicate's value unchanged. Second, each predicate is pure and total, so it never panics on a malformed record, because grading runs before full schema validation on the spreadsheet authoring path.

This layer replaced an earlier handful of inline conditionals, and three corrections made it right: the single white-light test was split into `hasWhitePoint` (which still gates CCT, because an RGBW white channel has a CCT) and `isWhiteLightPrimary` (which gates CRI, SDCM, and TM-30 and excludes every color-mixing mode); the BUG gate was narrowed from any outdoor fixture to a specific outdoor-site category set, so beam-characterized architectural uplights are not asked for an area distribution they do not have; and `wetOrExposed` stopped reading `ip_rating` (a standard field it was gating) so the core-fields-only invariant holds.

| Predicate | Reads (core fields only) | True when | Gates |
| --- | --- | --- | --- |
| `hasWhitePoint` | `configuration.tested_axes.color_tunability` | in {static_white, tunable_white, dim_to_warm, rgbw, rgbww} | nominal CCT (core); Duv and chromaticity x/y (enrichment) |
| `isWhiteLightPrimary` | `color_tunability` | in {static_white, tunable_white, dim_to_warm} only | CRI Ra (core); CRI tier and SDCM step (standard); TM-30 Rf and hue bins (full); the TM-30 PVF code (enrichment) |
| `directional` | `product_family.primary_category` | downlight, tracklight, cylinder, wall_washer, grazer, facade_projector, in_ground_uplight, sports_flood | beam angle (standard); field angle (enrichment) |
| `outdoorSite` | `primary_category` | flood_area_site, roadway_street, walkway, bulkhead_wall_pack, sports_flood | outdoor distribution type, longitudinal range, BUG (standard); LCS zonal (enrichment); legacy cutoff (observation) |
| `linear` | `primary_category` | linear, cove | per-length normalized and declared-by-length (standard) |
| `requiresDimmingDetail` | `electrical.driver_protocol` | in {0-10v, 1-10v, phase_forward, phase_reverse, pwm} (analog and phase-cut) | dimming method and dimming range (standard) |
| `controllableDriver` | `electrical.driver_protocol` | any protocol other than `non_dimming` | adaptive lighting modes (enrichment) |
| `wetOrExposed` | `environment_rating`, `indoor_outdoor` | env in {wet, marine_coastal, outdoor_rated}, or indoor_outdoor in {outdoor, both}; excludes `damp` | IP rating (standard) |
| `impactPublic` | `environment_rating` | == `vandal_resistant` | IK rating (enrichment only) |
| `poleMounted` | `mounting_types` | contains a pole or mast token | EPA (enrichment only) |
| market safety (`hasMarketSafetyListing`) | `technical_region` + attestation programs | a region-appropriate safety listing is present | the core safety-listing gate (region-conditional accept sets) |
| `isExitSign`, `isDedicatedEmergency`, `isDedicatedClass` | `product_family.primary_category` | category is `exit_sign`, `emergency_luminaire`, or either | select the exit-sign and dedicated-emergency profiles; `notExitSign` and `notDedicatedClass` are the literal negations that mark the universal rows those profiles exclude |
| `signMode(mode)` | `exit_sign.illumination_mode` | the sign's illumination mode equals the argument | the mode-conditional sign rows (see Product-class profiles) |
| `hasIntegralBattery` | `emergency.power_source` | == `integral_battery` | the standard battery trio (within `emergencyPowerCoreClass`) and the battery-nudge enrichment rows (outside it) |
| `emergencyPowerCoreClass` | `primary_category` + `exit_sign.illumination_mode` | a dedicated emergency luminaire, or an internally illuminated exit sign | the `power_source` core row, and the scope where the battery trio gates at standard rather than nudging as enrichment |
| `naDedicatedClass` | `primary_category` + `technical_region` | an NA-region dedicated-class product | the UL 924 core listing row (US-first) |

The form/driver and product-class predicates plus the market-safety check gate hard requirements; `controllableDriver`, `impactPublic`, `poleMounted`, and the emergency-mode and battery-nudge predicates gate enrichment rows only. Enrichment sub-field rows carry an additional family of block-presence predicates, which suggest a sub-field only when its parent block (photometry, test conditions, operating point, flicker, thermal derating, and the like) is already populated.

Compliance beyond the core safety listing never gates the conformance level: every other listing, certification, energy program, and declaration lives in the attestations array and `index.attestation_programs[]`, and the completeness grader ignores it. The Product Achievements axis computes a separate view over those same attestations, described in [The two grading axes](#the-two-grading-axes-completeness-and-achievements) below.

A note on what the safety gate proves. Conformance grading checks for the presence of a self-asserted safety-listing claim, not third-party verification of it; a conformance level is a data-completeness grade, never a safety certification. The verification standing of any claim lives separately in the attestation's own `AttestationStatus`. The Product Achievements axis follows the same trust boundary: a `documented` achievement means an attached, unexpired evidence document sits on a qualifying attestation, never that ULC verified the underlying program. See the [compliance and attestation program glossary](compliance-attestation.md) for the per-program conformance role and status model.

Two categorization choices in the predicates are worth spelling out, because they decide which fixtures owe outdoor-site data and which owe white-light data.

The first is the outdoor-site boundary. An `in_ground_uplight` and a `facade_projector` are graded as directional (beam-characterized), not as outdoor-site, even though both are installed outdoors. They have no Type I-V area distribution, and a BUG rating is undefined for a fixture whose whole architecture is intended uplight, so requiring outdoor distribution type, longitudinal range, and a BUG rating of them would demand metrics that do not exist for the form. Area, roadway, walkway, wall-pack, and sports-flood fixtures do carry a Type I-V distribution and a meaningful BUG rating, so those are the outdoor-site categories that owe the standard-tier outdoor rows.

The second is the white-point versus white-light-primary split. A CCT is meaningful for any fixture with a white point, including the white channel of an RGBW or RGBWW fixture, so `hasWhitePoint` gates nominal CCT broadly. CRI, SDCM, and TM-30 are white-light-quality metrics that a color-mixing fixture does not characterize: an RGBW fixture is specified by its mixing gamut, not a single CRI figure measured against a reference illuminant. So `isWhiteLightPrimary` excludes every color-mixing mode (RGBW included), and those quality metrics are waived for color-mixing fixtures rather than counted against them.

### Product-class profiles: exit signs and emergency luminaires

Two product classes grade against their own dataset rather than the architectural-photometry profile above, so a maker of exit-sign-only or emergency-only products is graded on the evidence its cutsheets actually carry instead of being stranded at incomplete for lacking an LM-79 report it never produces. The class is derived from `product_family.primary_category`, an existing core field: `exit_sign` selects the sign profile, `emergency_luminaire` selects the dedicated-emergency profile, and every other category keeps the normal profile. Keying on a core field means every profile-selecting predicate reads a core field, so the core-fields-only invariant holds by construction and a record cannot select two conflicting profiles.

Four product shapes follow:

- **Exit sign** (`exit_sign`): the sign profile. The architectural-photometry rows (distribution, flux, efficacy, input power and voltage, the LM-79 and lumen-maintenance rows, the accredited-lab full rows) are not-applicable, and the sign dataset gates instead. An internally illuminated sign is also a powered product: it authors the `emergency` block and gates on `power_source` at core. A self-luminous or photoluminescent sign carries its whole power story in `illumination_mode` and does not author the block. An externally illuminated sign has an unpowered face, but a combo variant whose egress heads run on a battery authors the block too; because `power_source` is not core for it, its battery depth surfaces as an enrichment nudge, not a standard gate.
- **Dedicated emergency luminaire** (`emergency_luminaire`): the normal profile, minus luminaire efficacy (lumens per AC-charging-watt is not a marketed figure for a battery-operated product), plus the emergency gates. Its own photometry block is its emergency-mode dataset, so its total-flux core row already gates the governing egress figure.
- **Combination exit/emergency unit**: an exit sign that also carries emergency egress heads. An internally illuminated combo gates the battery trio at standard, where its `power_source` is core; an externally illuminated combo is battery-nudged instead, since its face is unpowered.
- **Normal fixture with an emergency-power option**: the normal profile, unchanged. The `emergency` block is optional and non-gating; it adds only enrichment nudges.

As with the conditional predicates, a row a profile marks not-applicable is dropped from grading entirely, never reported as a gap: an exit sign is never asked for a beam angle or an LM-79 attestation. The sign dataset's conditionals all key on `exit_sign.illumination_mode`, a class-core field, never on the descriptive `illumination_technology`.

The sign rubric, beyond the universal identity and safety rows every record carries:

| Tier | Requirement | Field | Applies to |
| --- | --- | --- | --- |
| Core | Illumination mode | `exit_sign.illumination_mode` | every sign |
| Core | Legend color | `exit_sign.legend_color` | every sign |
| Core | Emergency power source | `emergency.power_source` | internally illuminated signs (and dedicated emergency luminaires) |
| Core | UL 924 listing | a `ul_924` attestation | NA-region signs and emergency luminaires |
| Standard | Legend height | `exit_sign.legend_height` | every sign |
| Standard | Face count | `exit_sign.face_count` | every sign |
| Standard | Directional indicator | `exit_sign.directional_indicator` | every sign |
| Standard | Housing and lens material | `product_family.shared_mechanical.*` | every sign (universal rows) |
| Standard | Sign-face luminance | `exit_sign.sign_face_luminance_cd_per_m2` | photoluminescent, self-luminous |
| Standard | Face illuminance | `exit_sign.face_illuminance_lx` | externally illuminated |
| Standard | Contrast ratio | `exit_sign.contrast_ratio` | externally illuminated |
| Standard | Input power (re-gate) | `electrical.input_power_w` | internally illuminated |
| Standard | Tritium rated life | `exit_sign.tritium_rated_life_years` | self-luminous |
| Standard | Minimum charging illuminance | `exit_sign.min_charging_illuminance_lx` | photoluminescent |
| Standard | Battery duration, chemistry, self-test | `emergency.battery_duration_min`, `.battery_chemistry`, `.self_test` | dedicated emergency luminaire or internally illuminated sign, with an integral battery (an externally illuminated combo sign is battery-nudged, not gated) |
| Full | Test-report-backed luminance | `exit_sign.sign_face_luminance_cd_per_m2` with `test_report` provenance | every self-emitting mode (mode other than externally illuminated) |
| Full | Test-report-backed illuminance | `exit_sign.face_illuminance_lx` with `test_report` provenance | externally illuminated |

Legend height gates at standard rather than core: the letter height is selection-grade data a designer compares (a 6-inch versus 8-inch lens choice), but a flagship commodity sign cutsheet does not always publish it, and a core gate that strands such a record is exactly the failure this profile exists to prevent. The rated viewing distance is an enrichment nudge, not a gate, because no surveyed sign cutsheet publishes it (it is a UL 924 product-face marking, not catalog data).

The sign full tier is anchored on UL 924 test-report depth: a lab-measured luminance or illuminance value carrying `test_report` provenance. Because a standard sign would otherwise have zero applicable full rows and vacuously reach full, two provenance-reading rows partition the sign class by mode so every sign has exactly one applicable full row. This is the one place a provenance object, not just a value, is the evidence: a measured luminance with no authored provenance stays at standard.

UL 924 plays a completeness-gate role here. A `ul_924` attestation satisfies the core safety listing (the program joins the region-conditional accept sets) and, for a North American dedicated-class product, a dedicated UL 924 listing row. It is read from the attestation ledger, never a boolean field, so listings have one home. As with every attestation the grader reads, this is a data-completeness signal, not a safety certification.

These cut points follow the same Division 26 escalation the architectural tiers mirror. MasterSpec section 265213 (Emergency and Exit Lighting) asks first for the product and its listing (core), then for the selection-grade sign geometry and battery data a designer compares (standard), then for independently certified test-report depth (full). The gates are US-first, anchored on UL 924 and the NFPA 101 / IBC evidence base; international emergency standards are not yet gated (see ROADMAP.md).

## The two grading axes: completeness and achievements

A ULC record is one authored source of truth, the **Master Ledger**: everything known about the fixture, gathered once. Its attestation component is the `attestations[]` array plus `product_family.shared_attestations[]`. Two computed views read that ledger, and they never cross:

- **Data Completeness** answers "how complete and rigorous is the datasheet?" It is the ordinal `incomplete < core < standard < full` grade the conformance rubric above computes, stamped into `index.conformance_level`.
- **Product Achievements** answers "what is the product documented for?" It is a per-theme picture of the third-party program qualifications the record demonstrates, stamped into `index.achievements`.

The two axes are orthogonal by construction: the achievements view never touches the conformance ladder, and no completeness rule reads the achievements output. A cheap fixture can carry excellent sustainability data, and a lab-characterized premium fixture can carry none, so the qualifications a product has earned do not belong on the completeness ladder. They get their own axis.

### The theme states

Product Achievements computes a state for each of seven themes: `embodied_carbon`, `circularity`, `material_health`, `energy`, `dark_sky`, `emergency`, and `domestic_content`. Each theme is one of three states:

- **`none`**: no qualifying attestation or declaration contributes to the theme.
- **`claimed`**: a qualifying, non-disqualified contribution exists, but no unexpired attached evidence supports `documented` (either no evidence document is attached, or its only attached evidence is past its `valid_until` relative to the record's `record_status_as_of`, which caps the theme at `claimed` even with a document attached). A status-disqualified attestation does not reach `claimed`; it contributes nothing.
- **`documented`**: at least one qualifying, non-disqualified attestation carries an attached evidence document that is unexpired as of the record's `record_status_as_of`.

A theme is routed by the program tokens on the ledger's attestations: a UL 924 listing feeds `emergency`, an EPD feeds `embodied_carbon`, a Declare label feeds `material_health`, and so on. The full program-to-theme map, and the list of programs deliberately left unthemed (project-level programs like LEED, multi-attribute ecolabels, and controls programs held for a future theme), is published as a versioned appendix in the [compliance and attestation program glossary](compliance-attestation.md). There is no weighted or overall achievement grade: the only rollup is `documented_count`, a factual count of themes at `documented`. Weighting embodied carbon against material health has no accepted method, and inventing one would be ULC editorializing.

### Evidence, and what "documented" means

The `claimed`-to-`documented` discriminator is an attached, unexpired evidence document, never a status token. An attestation is evidence-backed when it carries a `source_document_ref`, a file reference the schema guarantees to have a filename and a SHA-256 content hash. The hash is the integrity anchor a consumer can check; the `program` token names the qualification, and the optional `issuing_authority` field names the specific operator that issued it when a program runs under more than one (as EPDs do). `documented` is a record fact ("an evidence document is attached"), never a verification performed by ULC, which is the same trust boundary the safety gate observes. The `restricted_substances_declared` sibling flag records the RoHS, REACH, Prop 65, and similar restricted-substances programs the ledger declares; it sits beside the themes, never inside one, because restricted-substances compliance is a legal floor, not a prestige achievement.

### The reproducible-inputs-only invariant

The achievements compute reads exactly three inputs and nothing else: the merged attestation ledger, `sustainability_declaration.declaration_type`, and the envelope field `record_status_as_of` (read only for the expiry comparison below). It reads no product category, no photometry, no electrical or colorimetry. This is the achievements analog of the completeness read-only-core invariant (see [Applicability predicates](#applicability-predicates)): both views are pure functions of one record. In particular, the wall clock never enters the compute, so `index.achievements` is reproducible: a record built today and rebuilt next year produces byte-identical output, and `build-index --check` never breaks in CI purely because time passed. Separately, the `ulc validate` CLI offers an opt-in, report-only expiry advisory (`--expiry`) evaluated at a caller-chosen date that previews upcoming and already-lapsed evidence; it runs at report time and never enters the compute, the stamped index, or the exit code, so this invariant holds intact.

Two rules qualify a contribution. First, **record-relative expiry**: when an attestation carries a `valid_until` date and the record carries a `record_status_as_of` date, an attestation whose validity ended before the record's as-of date cannot support `documented` (it still supports `claimed`); when either date is absent, no expiry evaluation happens. The comparison is against the record's own as-of date, never the build date, which is what keeps the output reproducible. Second, **status disqualifiers**: an attestation whose `status` is `expired`, `withdrawn`, or `not_applicable` contributes nothing to any theme, as if absent. Every other status contributes normally; the `claimed`-to-`documented` split comes from the evidence (its presence and, per the expiry rule above, its currency), never from the status token.

### The emergency theme is ledger-only

The emergency theme is computed from the ledger tokens (`ul_924`, `ul_1994`, `icel`) with no product-role or category predicate. A dedicated exit sign that carries UL 924 with an attached certificate is `documented`; a normal troffer that carries an emergency-power option and a UL 924 listing earns the same theme from the same token. Gating the theme on product role would show `none` on a dedicated sign that plainly qualifies, which is perversely false, so the theme reads the ledger and nothing else, consistent with the reproducible-inputs-only invariant.

### Where the axis surfaces

The builder stamps `index.achievements` (the seven themes, each with its state, qualifying programs, contributing attestation ids, an evidence flag, and an optional best-metric reference) and the sibling `index.restricted_substances_declared`. The reference validator emits one default-visible `achievements/summary` line naming the documented and claimed counts, plus verbose-only per-theme `achievements/state` findings and a `claimed`-to-`documented` `achievements/roadmap` (attach the certificate to raise a claimed theme to documented).

## How the standard evolves

ULC changes through the Schema Change Proposal (SCP) process described in `CONTRIBUTING.md` and `GOVERNANCE.md`. A proposal is opened as an issue that states what the change is, why it is needed, which records and tools it affects, and whether it is backward-compatible. After discussion and maintainer feedback, an accepted proposal lands as a pull request that updates the schema, at least one example, the relevant documentation, and the changelog together.

Two practices keep that evolution honest. Changes are grounded in real manufacturer cutsheets: example records are sourced from real spec sheets and real IES files, not invented data, so every field that enters the schema is motivated by a real product or a real workflow gap. And the schema is additive by default. New fields and enums extend the standard without breaking existing records; when a constraint genuinely needs tightening, the change is documented explicitly and chosen so that existing valid records remain valid wherever possible.

The result is a standard that grows from the evidence rather than ahead of it. For the consumer-facing walkthrough, see [how-it-works.md](how-it-works.md); for the patterns and primitives, see [authoring-patterns.md](authoring-patterns.md); for governance and the guiding principles, see `GOVERNANCE.md`.
