# Changelog

All notable changes to the ULC specification are recorded here.

ULC uses semantic versioning. Major versions indicate breaking changes to record structure or required behavior. Minor versions indicate backward-compatible additions or clarifications. Patch versions indicate corrections and non-structural edits.

Each ULC record declares the specification version it conforms to via the `ulc_version` field.

## Release process

A version is **unreleased** until it is tagged in git. Being visible on `main` is not the same as being released; consumers who want a stable version pin to a git tag.

When a version is ready to release:

1. Replace the `(unreleased)` marker next to the version heading below with the release date in ISO 8601 format, e.g. `## 0.1.0 (2026-06-15)`.
2. Commit that change to `main`.
3. Create an annotated git tag matching the version: `git tag -a v0.1.0 -m "ULC v0.1.0"`.
4. Push the tag: `git push origin v0.1.0`.
5. Optionally create a GitHub Release pointing at the tag, copying the version's changelog entry into the release notes.

## 0.3.0 (unreleased)

Schema refinement informed by the four reference records. The vast majority of changes are additive; one field was tightened (see below) but no existing records' data was invalidated. Larger breaking semantic changes (single-valued fields becoming arrays, single references becoming plural) are deferred to a later revision so pilot-program feedback can inform them.

### One compatibility-tightening change

- `Configuration.tested_axes.cri_tier` changed from free-string to a closed-enum reference (`taxonomy.schema.json#/$defs/CriTier`). Strictly this narrows the accepted values, so per semver it is compatibility-tightening rather than purely additive. Practical impact on existing records is zero: all four v0.2 reference records use values already enumerated by the new CriTier (`cri_80`, `cri_90`, and so on), and those values remain valid. Authors of records that used non-enumerated CRI strings must migrate to an enumerated value.

### Schema additions

- `Photometry.cutoff_angle_from_horizontal_deg` — architectural cutoff angle for glare control, distinct from the deprecated IES outdoor cutoff classification.
- `Photometry.ugr_4h_8h_bound_operator` — sibling to `ugr_4h_8h`. Carry `lte` when a manufacturer declares "UGR as low as X" rather than a specific measured value, matching the flicker metric bound-operator pattern.
- `Photometry.declared_by_length[]` — native home for Pattern D length-scaled photometric arrays. Mirrors the existing `declared_by_cct[]` shape but keyed on fixture length via DualUnitLength.
- `Electrical.dimming_range_percent: {min, max}` and `Electrical.dimming_method` — structured dimming depth and driver method. `dimming_method` is a new enum with values `ccr`, `pwm`, and `hybrid`.
- `ProductFamily.technical_region` — market-variant declaration. New enum `TechnicalRegion` with values `120v_60hz_north_america`, `230v_50hz_europe`, `100v_50_60hz_japan`, and `universal`.
- `ProductFamily.physical_dimensions` — block with slots for overall dimensions, luminaire mass, linear mass per foot, lens width, ceiling aperture, recess depth, ceiling thickness accommodation, connection cable length, driver dimensions, and EPA for pole-top outdoor products.
- `ProductFamily.shared_mechanical.reflector_material` — free-string slot for internal reflector material descriptions.
- `CompatibleAccessory.is_compatible_with_this_sku` (boolean, default true) and `incompatibility_reason` — lets records declare accessories that are listed at the family level but not compatible with the specific SKU the record represents.
- `Index.required` no longer includes `nominal_cct_k`. Color-changing fixtures (RGB, RGBW, RGBA, multichannel) legitimately have no nominal CCT and now produce a valid index without a placeholder.

### Taxonomy additions

- `AttestationProgram.lm_79_08` — the 2008 original edition of LM-79 is now a first-class enum value; previously required the generic `lm_79` family label with a free-text `standard_revision` workaround.
- `TestedProductType.led_package` — canonical DUT for LM-80-21 LED package lumen-maintenance testing.
- `DimmingProtocol.lumentalk` — promoted from `extensions.manufacturer_specific` because it is used across multiple fixture manufacturers under license.
- `HousingMaterial.aluminum_unspecified` — for cutsheets that describe aluminum housings without distinguishing cast, die-cast, extruded, or sheet variants.
- `LensMaterial.cone_only` — for darklight-reflector architectural downlights with no lens element beyond the reflective cone.
- `SourceFileType.supplementary_pdf` and `ProvenanceSource.supplementary_pdf` — for certifications cheatsheets, end-of-life guidelines, IES road reports, and similar ancillary PDFs previously classified as `article_text`.
- `SustainabilityDeclarationType.manufacturer_recycle_program` — for manufacturer-operated repair-restore-recycle initiatives (for example Lumenpulse's Lumencycle program).
- New `DimmingMethod` enum: `ccr`, `pwm`, `hybrid`.
- New `TechnicalRegion` enum (values listed above).
- New `CriTier` enum: `cri_70`, `cri_80`, `cri_90`, `cri_95`. `Configuration.tested_axes.cri_tier` now references this enum instead of accepting free-string values.

### Builder

- `tools/build-index.py` bumped to `BUILDER_VERSION 0.2.0` to signal the Index.required change. Records' stored indices automatically re-stamp to `0.2.0` on the next `build-index.py` run.

### Reference record migrations

All four reference records were migrated from `extensions.manufacturer_specific.<slug>.*` parking spots into the new native fields where applicable:

- `examples/erco-quintessence-30416-023.ulc` — physical dimensions, cutoff angle, dimming range and method, LM-79-08 and LM-80-08 program values, technical region, reflector material, cone-only lens material, and led_package tested product type on the lumen-maintenance package entry all moved to native. Internal manufacturer code, environmental flags, and other genuinely extension-appropriate content retained in extensions.
- `examples/vode-nexa-suspended-807-so-3500k-90cri-hl-black-48in.ulc` — `photometry_declared_by_length` moved into native `photometry.declared_by_length[]`, UGR bound operator moved to native, physical dimensions with linear mass per foot moved to native, technical region set to `universal`, and the certifications cheatsheet file type changed from `article_text` to `supplementary_pdf`.
- `examples/selux-aya-pole-sr-ho-3000k.ulc` — physical dimensions including EPA, technical region, lens material, LM-79-08 program value, and IES Road Report file type all moved to native. Multi-variant data (pendant vs pole-top masses and EPAs) remains extension-parked pending a future multi-variant pattern.
- `examples/lumenpulse-lumenfacade-loi-12-rgb-30x60-ts0.ulc` — dropped the placeholder `nominal_cct_at_test: "6500"` entirely (RGB has no nominal CCT), dropped the matching colorimetry placeholder, moved the LOI-JBOX incompatibility into a native `compatible_accessories[]` entry with `is_compatible_with_this_sku: false`, changed the end-of-life guidelines PDF type to `supplementary_pdf`, and updated physical dimensions and technical region to native slots.

Records' `ulc_version` fields bumped from `0.1.0` to `0.3.0` reflecting their dependence on v0.3 schema additions.

### Not yet addressed

Breaking semantic changes are intentionally deferred. These remain extension-parked or schema-flagged as future work:

- `declaration_framework` single-valued becoming an array
- `attestation_ref` single-string becoming plural (to cite both data-collection and method-definition attestations)
- `manufacturer_rated_claim` single-claim becoming multi-claim
- `DerivationRule.linear_rate` single-number becoming named slots for flux and power

These are expected to land in a future major revision informed by the reference CLI validator (a later batch) and manufacturer pilot feedback.

## 0.2.0 (2026-04-22)

Canonical reference records for the four manufacturer authoring patterns, plus minor schema and tooling cleanups.

- Added four reference records in `examples/`, each drafted from a real manufacturer spec sheet and IES file and validated against the schema:
  - Pattern A (single SKU per cutsheet): Erco Quintessence 30416.023 recessed indoor downlight
  - Pattern B (per-photometric-scenario with applicability): Selux AYA Pole SR-HO-3000K with CCT multiplier table covering 2200 K through 5000 K
  - Pattern C (per-IES with provenance classes): Lumenpulse Lumenfacade LOI color-changing inground luminaire at the 12 in RGB 30x60 scenario, demonstrating `extended_photometry` provenance with `base_attestation_ref` pointing at the original LM-79 test
  - Pattern D (per-foot linear scaling with conditional attestations): Vode Nexa Suspended 807 at the Standard Output, 3500 K, 90 CRI, Honeycomb Louver Black Anodized, 48 in scenario, exercising option-conditional and case-by-case attestation patterns
- Removed `AttestationVerificationType.requires_project_documentation` from the taxonomy: the value introduced project-context semantics that crossed the fixture-relevance rubric boundary
- Corrected a path reference in the `ProvenanceMethod` description from `source.base_attestation_ref` to `provenance.base_attestation_ref` to match the schema
- Expanded the automated pre-merge review workflow's file-match pattern to include `templates/**/*.ulc` and `tools/hooks/**`
- Removed a dead `has_fragment` variable in `tools/schema-drift-guard.py`

## 0.1.0 (2026-04-22)

Foundation of the ULC specification: schemas, authoring patterns, drift-guard tooling.

- Established repository structure, governance, and contribution guidelines
- Set foundational project documentation (README, CONTRIBUTING, GOVERNANCE, CODE_OF_CONDUCT)
- Selected MIT License
- Reserved namespace `https://ulcspec.org/schema/` for schema identifiers
- Shipped the split schema foundation: `schema/taxonomy.schema.json` (78 closed enums grounded in study of IES LM-79, LM-80, TM-21, LM-84, TM-28, TM-30, TM-35, RP-8, RP-46, TM-15, LM-75, LM-82, LM-90, and related standards, with `AttestationProgram` carrying 102 values across trade-body, product-safety, energy-code, domestic-procurement, sustainability, and test-method programs) and `schema/ulc.schema.json` (record structure with product family, configuration, applicability, photometry, colorimetry, alpha-opic, flicker, outdoor classification, lumen maintenance, chromaticity shift, sustainability declaration, attestations, and a generated index block)
- Shipped `docs/authoring-patterns.md` describing the four manufacturer authoring patterns observed in real cutsheet evaluation and the architectural primitives the schema provides to support them
- Shipped `tools/schema-drift-guard.py` to validate every `$ref` resolves across the split, `tools/build-index.py` as the canonical index deriver (the index is generated, never hand-authored), and `tools/builder-parity-guard.py` to confirm builder-schema alignment
- Shipped `tools/hooks/pre-commit` as a tracked sample hook that mirrors the CI guards locally
- Added CI workflow at `.github/workflows/schema-drift-guard.yml` running drift, parity, and record-index checks on every pull request touching schema, the builder, or example records
