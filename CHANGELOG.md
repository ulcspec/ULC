# Changelog

All notable changes to the ULC specification are recorded here.

ULC uses semantic versioning. Major versions indicate breaking changes to record structure or required behavior. Minor versions indicate backward-compatible additions or clarifications. Patch versions indicate corrections and non-structural edits.

Each ULC record declares the specification version it conforms to via the `ulc_version` field.

## Release process

A version is **unreleased** until it is tagged in git. Being visible on `main` is not the same as being released; consumers who want a stable version pin to a git tag.

Releases are automated. To ship a release:

1. Cut a branch named `release/vX.Y.Z` (e.g., `release/v0.6.0`).
2. Update CHANGELOG.md: add a dated section header at the top of the release entries below, e.g., `## 0.6.0 (2026-06-15)`. Fill in the section.
3. Open a pull request against `main`. The `Release notes check` workflow validates that the branch name, the CHANGELOG section, and the date are present and consistent.
4. Merge the PR after review.
5. The `Release on merge` workflow runs automatically. It tags the merge commit with `vX.Y.Z` (annotated), extracts the CHANGELOG section as release notes, runs goreleaser to build cross-platform binaries, and publishes the GitHub Release with the binaries and notes attached.

For emergency manual releases (bypassing the PR flow), trigger the `Release on merge` workflow manually via `workflow_dispatch`, providing the version input.

## 0.8.1 (2026-06-23)

Corrects the `source_files` field description in `schema/ulc.schema.json`, which contradicted the rest of the schema. The `source_files` key is a required member of the record envelope (present on every record), while its array may be empty (`minItems` 0) and its entries are neither required nor graded. The description previously said the array was "neither schema-required," disagreeing with the root `required` array and the record-envelope descriptions in the root schema and the `ConformanceLevel` definition. This is a description-only correction: no structural change, no enum or behavior change, and no re-grade. Records continue to declare `ulc_version` `0.8.0`; a 0.8.0 record is unchanged and valid under 0.8.1.

## 0.8.0 (2026-06-23)

`incomplete` becomes the true floor of the conformance model. The tooling never fails a record on data completeness: a record with identity but zero source documents is valid, grades `incomplete`, and carries a roadmap to core. The roadmap now decomposes per grade all the way to full, showing the grades a record already satisfies and, for each grade it has not yet reached, only that grade's own remaining fields. The model is now framed as three grades (`core`, `standard`, `full`) above an `incomplete` floor.

### Behavior change (important for consumers)

- Records that previously failed `ulc validate`, `ulc build-index`, or `ulc from-sheet` because they were anchorless or missing required index keys now SUCCEED (exit 0) and grade `incomplete` with a roadmap. Any external tooling or CI that treated a nonzero exit as "this record is unacceptable" must now treat `incomplete` as a valid, expected, below-core state. The subcommands still exit nonzero on malformed input, missing record identity, source-file integrity failures (hash mismatch, unreadable files), or a stored-vs-recomputed conformance drift; `ulc validate` and `ulc from-sheet` additionally exit nonzero on JSON Schema invalidity, while `ulc build-index` runs no schema validation and instead gates on the builder's required index keys. No data-completeness condition produces a nonzero exit.
- `index.conformance_level` is now always present, including `incomplete`. The internal below-floor sentinel (`none`) is removed: the grader never returns it and nothing renders it.

### Schema (additive, pre-1.0)

- Three `required` sites are loosened so an identity-only record is representable: the index drops the three photometric projections (`primary_category`, `nominal_total_lumens`, `nominal_input_power_w`) from its required keys, `product_family` drops `cutsheet`, and `source_files` may be empty (`minItems` 1 to 0). All three are loosenings, so no existing record becomes invalid.
- The `ConformanceLevel` description is reframed: `incomplete` is the floor (a record that has not yet met core), never a published grade, always traveling with a roadmap. The enum values are unchanged.

### Grading

- The cutsheet moves from a schema requirement to a graded core item, so an unattached cutsheet now grades `incomplete` with a roadmap entry naming `/product_family/cutsheet` instead of failing schema validation. This is the only rubric membership change; which fields gate which grade is otherwise unchanged. Example grades do not change.
- `AchievedLevel` floors at `incomplete` (the zero value) and never returns a below-floor sentinel.
- The roadmap is emitted per grade through full. Each grade reports one of three states: satisfied (a new `conformance/grade-satisfied` info finding), an outstanding delta (the existing `conformance/gap` roadmap), or gated (a new `conformance/grade-gated` info finding when a grade's own requirements are met but a lower grade is not, naming the grade to reach to unlock it). The structured `Finding` fields are unchanged, so existing JSON consumers keep parsing; they see two new info codes. A consumer keying on `conformance/grade-gated` must treat it as NOT achieved; the achieved grade is `index.conformance_level`.

### Builder and CLI

- The builder always stamps `conformance_level`. The index may be sparse for an `incomplete` record (photometric projections are omitted when their data is absent). `BuilderVersion` is `0.5.0`, so every stored index re-stamps on the next `ulc build-index`.
- `ulc from-sheet` accepts a workbook record with an empty `cutsheet_file`: it converts the record (omitting `product_family.cutsheet` and the synthesized datasheet source-file entry) instead of failing, so a cutsheet-less record grades `incomplete` with a roadmap. It writes an `incomplete` converted record to its output directory with a console notice that it is below core; it no longer skips such a record.

### Examples and docs

- The five reference records re-stamp to `ulc_version` `0.8.0` and `builder_version` `0.5.0`. Grades are unchanged (erco, selux, and both Lumenpulse records at `standard`; Vode at `core`); their roadmaps now render the full per-grade decomposition.
- `methodology.md`, `how-it-works.md`, `authoring-patterns.md`, the README, and the examples README are reframed from "four levels" to "three grades above an incomplete floor."

## 0.7.0 (2026-06-20)

The conformance rubric is redesigned from a thin 16-field check into an exhaustive, document-aware rule table, and the conformance ladder gains an `incomplete` tier below `core`. Stored indices recompute, so several records change level.

### Schema (additive, pre-1.0)

- `ConformanceLevel` gains an `incomplete` token, so the ladder is now `incomplete` < `core` < `standard` < `full`. An `incomplete` record is a real photometric record (it carries the flux, input-power, and primary-category anchors, so it indexes) that has not yet met every core requirement; it still receives an index and a roadmap naming the core fields it is missing. Additive: no existing record becomes invalid, and a record carries the token only after it re-stamps.
- `SourceFileType` and `ProvenanceSource` gain `test_report`, `compliance_documents`, and `driver_datasheet_pdf`.
- `AttestationProgram` gains 43 tokens spanning regional market-access marks, additional North American safety listings, EMC declarations, food-zone safety, energy programs, and environmental, material, and origin declarations. Only a small subset of safety listings is read by the grader's core safety gate; the rest are presence-tracked for catalog and search and never affect the conformance level.
- The `Attestation` block gains optional `list_date` and `list_version` for list-versioned badges (REACH SVHC, California Prop 65, conflict minerals).
- `DimmingProtocol` gains `dali_2_dt6`, `d4i`, `lutron_ecosystem`, and `bluetooth_mesh`, covering the DALI-2 DT6 device type, the D4i intra-luminaire standard, Lutron's EcoSystem protocol, and the Bluetooth mesh topology (distinct from point-to-point `bluetooth`).
- A new optional `electrical.dimming_curve` field, backed by a new `DimmingCurve` taxonomy (`logarithmic` or `linear`), records the dim-command-to-output mapping when the driver datasheet publishes it. Ungated and absent unless declared.

### Grading (breaking for stored indices, pre-1.0)

- The conformance rubric is now a declarative, taxonomy-mapped rule table with a composable applicability-predicate layer, replacing the inline checks. Core is richer (full identity, headline photometric and electrical numbers, one-line colorimetry, and a market safety listing). Standard and full gain white-light, directional, outdoor-site, linear, analog-dimming, and wet-location conditionals. `zonal_lumens` is a full-tier requirement (it is IES-derived, not datasheet-published), and the MacAdam SDCM step is a standard requirement for primarily white-light fixtures.
- The dimming-method and dimming-range standard gates apply only to analog and phase-cut drivers (0-10V, 1-10V, phase, PWM-input), whose dim floor and electrical method are published driver specs a designer selects on. Digital control protocols (the DALI family, DMX, DSI), wireless protocols, and non-dimming drivers are exempt, so a complete digital-control datasheet is not held back for a value its cutsheet does not carry.
- The safety-listing core gate checks for the presence of a self-asserted listing claim, not third-party verification of it. A conformance level is a data-completeness grade, never a safety certification.
- Each missing requirement now carries a machine-readable roadmap (the conformance level it unlocks, the source document that supplies it, and the governing standard) at every climb, including out of `incomplete` into `core`. This is also an output-shape change for tooling: the gap is now emitted as one finding per missing requirement, anchored at that field's own path, replacing the single aggregate finding previously emitted at `/index/conformance_level`. Consumers that parsed the old aggregate finding must read the per-field findings instead.
- Recomputed example levels: `erco-quintessence` and `selux-aya-pole` move full to standard, and `vode-nexa` moves full to core (its only standard gap is the SDCM step). The `lumenpulse-lumenfacade` RGB record moves core to standard, and a new RGBW companion record is added at standard; both are color-mixing DMX/RDM fixtures whose digital driver is exempt from the analog and phase-cut dimming gates.

### Builder

- `BuilderVersion` bumped `0.3.0` to `0.4.0`. Stored indices re-stamp (and may change level) on the next `ulc build-index`, and the builder now stamps the `incomplete` token.

### Validator

- `ulc validate` gains a `--verbose` flag. By default the text report shows the achieved level plus the roadmap and omits the non-gating conformance observations (the comprehensive-depth nudges); `--verbose` includes them. JSON output always includes everything.

### Documentation

- New `docs/compliance-attestation.md`: a glossary of every `AttestationProgram` token, its governing program, its category, and whether the grader reads it for the core safety gate, headed by the trust-boundary note that grading checks claim presence, not verification.
- `docs/methodology.md` gains a top-level conformance-rubric section documenting the four tiers, the membership tables, and a dedicated applicability-predicates subsection.
- `docs/how-it-works.md` gains a "How records are authored" section documenting the two authoring paths (filling the `templates/workbook/` spreadsheet for the deterministic `ulc from-sheet` converter, or emitting from a PIM), which source documents to gather, and why the compiled output is accurate by construction.
- `docs/methodology.md` adds a "Why the tiers fall where they do" subsection: the conformance tiers mirror how a construction specification escalates its submittal requirements (mandatory product data plus a safety listing, then selection-grade performance specifications, then independently-certified test reports), ordered by evidence depth and scoped by applicability. The root `README.md` conformance paragraph gains the one-line principle and points to it.
- `ROADMAP.md` records new deferred-schema items: emergency-lighting operational data, entertainment fixture capabilities, NEMA flood beam-spread classification, and structured safety-listing detail.

### Repository

- The six per-category hand-fill JSON skeleton templates (`templates/downlight.ulc.json` and siblings, with their `.md` guides) are removed. Hand-authoring JSON is not a recommended path: a record is compiled from the manufacturer's source documents by a deterministic tool regardless of luminaire type, so the curated `examples/` plus the schema serve the reference need better than an arbitrary subset of category skeletons. The fill-in workbook template at `templates/workbook/` (the input to `ulc from-sheet`) is retained.

### Examples

- The `lumenpulse-lumenfacade-loi-12-rgb` reference record is corrected to the `-ASL` (anti-slip lens) order option it documents: the catalog number and the referenced IES filename and SHA-256 are updated to the ASL photometry, and the headline figures are re-scaled to that file (total luminous flux 700 to 637 lm, with the maximum-intensity, efficacy, and candela-multiplier values that follow).
- A second Lumenpulse facade record, `lumenpulse-lumenfacade-loi-12-rgbw30k-10x60-ts2-5`, is added: the RGBW (3000 K white channel) counterpart to the RGB facade record. Its white channel is graded for nominal CCT, while the white-light-quality gates (CRI, SDCM, TM-30) are waived for the color-mixing architecture.

## 0.6.1 (2026-06-03)

### Tools

- New `ulc from-sheet` subcommand: a deterministic, offline converter that turns a manufacturer-authored workbook into validated ULC records, no LLM involved. It accepts either a CSV bundle (a directory of `<sheet>.csv` files) or a native `.xlsx` workbook (one tab per sheet, read with the standard library only), classifies each record into one of the four authoring patterns by sheet and column presence, and assembles the deep blocks: dual-unit companions, SHA-256 hashes (with the cutsheet dual-write), and default provenance are computed, then the index is built (stamping `conformance_level`) and each record is validated against the schema. Pattern B generates `photometry.declared_by_cct` from a CCT multiplier table; Pattern D generates or echoes `photometry.declared_by_length` from per-foot rates. Optional comprehensive sheets (`alpha_opic`, `flicker_metrics`, `lumen_maintenance_package`, `zonal_lumens`, `lcs_zonal_lumens`, `ingredient_list`, `cie97_lmf` / `cie97_llmf`) add full-level depth when present. A fill-in workbook template ships at `templates/workbook/`.

## 0.6.0 (2026-06-03)

Conformance level becomes a computed, builder-stamped value rather than a hand-declared field.

### Schema (BREAKING, pre-1.0)

- The top-level `conformance_level` field is removed from `required[]` and from `properties` in `schema/ulc.schema.json`. The conformance level is no longer declared by the author; the reference builder computes it from the record's populated fields (`grade.AchievedLevel`) and stamps it into the generated index as `index.conformance_level` (`core` / `standard` / `full`). Downstream consumers that read the top-level field must read `index.conformance_level` instead.
- A top-level `conformance_level` is now explicitly forbidden (a `false` subschema on the removed property), so a stale or hand-authored value is a hard validation `ERROR` rather than being silently accepted. Records that still set it must drop it and re-run `ulc build-index`.

### Builder

- `BuilderVersion` bumped `0.2.0` → `0.3.0` to signal that `conformance_level` is now a generated index field. Records' stored indices re-stamp to `0.3.0` on the next `ulc build-index` run.

### Validator

- `ulc validate` now grades the record against the conformance rubric and reports the achieved level plus the gap to the next level up as `INFO` findings. No `WARNING` is emitted: there is no declared level for the record to fall short of, so a record simply is whatever level its populated data achieves.

### Reference records and templates

- All four reference records (`examples/*.ulc`) and all six templates (`templates/*.ulc.json`) were migrated to the computed-conformance shape: the hand-declared top-level `conformance_level` was removed and the builder-computed `index.conformance_level` now carries the level. Stored indices re-stamp `builder_version` `0.2.0` → `0.3.0`. The lumenpulse RGB record now grades `core` (it previously self-declared `standard`); the other three grade `full`. Their `ulc_version` is bumped from `0.3.0` to `0.6.0`, reflecting the breaking removal of the top-level `conformance_level` field.

### Documentation and taxonomy descriptions

- Reworded the `ConformanceLevel` and `index.conformance_level` schema descriptions from "declared" to "computed and builder-stamped", and clarified that `full` is hard-gated only on operating-point qualifiers (and a BUG rating for outdoor products); TM-30 hue bins, method-backed lumen-maintenance projections, measurement uncertainty, and instrumentation depth are reported as `INFO` observations, not hard requirements.
- Relaxed the `attestation_ref` description from "Required for" to "Expected on" fields with `value_type: measured`, and reworded `measured_quantities` from "Validators check that ..." to "... is expected to reference ...", reflecting that the provenance and cross-record checks are planned, not yet implemented in the reference validator.
- Corrected the `gldf` SourceFileType description: GLDF expands to Global Lighting Data Format (not Global Luminaire Data Format).

## 0.5.1 (2026-04-27)

Repository infrastructure release. No normative schema, taxonomy, validator, or mapping changes — all v0.5.0 reference records and templates pass `ulc validate` unchanged at the v0.3.0 schema level.

### Open-source governance baseline (new)

A complete set of community-facing files brings the repository up to a typical open-source pattern:

- `SECURITY.md` documenting reporting scope and the GitHub Private Vulnerability Reporting flow.
- `ROADMAP.md` with the versioning model, v1.0.0 criteria, and an explicit out-of-scope list.
- `.github/CODEOWNERS` routing review requests to the new `ulcspec/maintainers` org team.
- `.github/PULL_REQUEST_TEMPLATE.md` with Summary / Test plan / Callouts sections.
- Seven issue templates covering bug report, spec clarification, schema change proposal, mapping issue, feature request, validator install issue, plus a config that routes general questions to Discussions.
- `.github/dependabot.yml` for `gomod` and `github-actions` weekly updates with automatic `dependencies` labeling.
- `.github/FUNDING.yml` placeholder; founding partners not yet identified.
- `CONTRIBUTING.md` PR conventions section: Conventional Commit prefixes, branch-naming convention, and `Closes #N` issue linking.

### Code review tooling (new)

- `greptile.json` configuring the Greptile reviewer with eight ULC-specific rules covering schema breaking-change detection, schema-to-mappings-to-templates cross-file consistency, validator-matches-schema parity, dual-unit SI authority, real-cutsheet-only example data, CHANGELOG discipline, and the metadata-only regulatory constraint.

### Release automation (rewritten)

The release pipeline shifts from manual tag-and-push to CHANGELOG-driven auto-tag-on-merge:

- `.github/workflows/release.yml` triggers on PR closed when the head ref matches `release/v*`. Resolves the target ref before checkout (so retries via `workflow_dispatch` check out the existing tag), tags the merge commit with an annotated tag, extracts the matching CHANGELOG section as release notes, and runs goreleaser to build cross-platform binaries. Includes idempotent retry, concurrency serialization, an emergency `workflow_dispatch` fallback, env-passed user-controlled inputs to defend against shell injection, and SHA-pinned action references.
- `.github/workflows/release-notes-check.yml` (new CI gate) validates that every `release/v*` PR has a dated CHANGELOG section, a well-formed branch name, and no pre-existing tag on origin.
- Both workflows escape version dots before regex matching, so a malformed CHANGELOG heading like `0x6x0` cannot satisfy a `0.6.0` release.

### Documentation

- `CHANGELOG.md` Release process section rewritten to describe the automated flow and the `release/vX.Y.Z` branch convention.
- `ROADMAP.md` versioning narrative softened to acknowledge that pre-1.0 releases may include compatibility-tightening changes when documented in the changelog (matching the v0.3.0 `cri_tier` precedent).

### Dependency updates

- `actions/github-script` v7.1.0 → v9.0.0
- `openai/codex-action` v1.4 → v1.7
- `tj-actions/changed-files` v44.0.0 → v47.0.6

### Scope note

This release is repository-infrastructure-only. It introduces no normative schema, taxonomy, validator, or mapping changes. Records, templates, and mapping guides shipped in v0.5.0 are unaffected.

## 0.5.0 (2026-04-23)

Author-facing documentation: per-category `.ulc.json` authoring templates and PIM platform mapping guides. No normative schema, taxonomy, or validator changes — all four canonical reference records and the new templates pass `ulc validate` unchanged at the v0.3.0 schema level.

### Templates (new)

Six per-category starter templates under `templates/`, each a structurally valid ULC record with category-typical defaults pre-filled and obvious placeholders (`"TODO ..."` strings, sentinel `0` numerics, 64-zero SHA-256 sentinel hashes) for author-supplied values. Each `.ulc.json` pairs with a sibling `.md` authoring guide that walks through the category-specific field conventions.

- `templates/downlight.ulc.json` + `downlight.md` — recessed or surface-mount ceiling downlight.
- `templates/linear-pendant.ulc.json` + `linear-pendant.md` — suspended linear luminaire.
- `templates/wall-pack.ulc.json` + `wall-pack.md` — exterior wall-mount bulkhead.
- `templates/high-bay.ulc.json` + `high-bay.md` — industrial high-ceiling luminaire.
- `templates/bollard.ulc.json` + `bollard.md` — exterior ground-mount pathway bollard.
- `templates/wall-sconce.ulc.json` + `wall-sconce.md` — interior wall-mount sconce.

Templates bootstrap a first record. The conformance level is not declared by the author: the builder computes it from the populated fields and stamps `index.conformance_level`, so the level rises from `core` toward `standard` and `full` as data is added and the builder regrades. Not a production authoring surface for catalog-scale manufacturers; that use case is served by PIM emit, documented below.

`templates/README.md` is rewritten from a stub (that referenced a non-existent `ulc.template.json`) into a category index and workflow guide covering placeholder conventions, slug naming, and the copy → fill → build-index → validate flow.

### PIM mapping guides (new)

Four platform-specific guides for emitting ULC records at catalog scale from a Product Information Management (PIM) system:

- `mappings/pim/salsify.md` — cloud PIM popular with consumer-facing and mid-market lighting brands.
- `mappings/pim/akeneo.md` — open-source PIM popular with European PHP-stack manufacturers.
- `mappings/pim/sap.md` — SAP MM, PLM, MDG, and Classification System integration for large-enterprise manufacturers.
- `mappings/pim/custom-pim.md` — architectural patterns for manufacturers running an in-house product-master database.

`mappings/pim/README.md` consolidates the six shared translation concerns (record-per-scenario model, dual-unit handling, provenance defaults, source-file hashing, category enum mapping, index generation) that every PIM emitter must address, so per-platform guides can focus on platform specifics.

`mappings/README.md` is updated to cover both categories of mapping: adjacent data standards (GLDF, ETIM, IES LM-63, EULUMDAT — still planned) and PIM platforms (shipped in this release).

### Scope note

Batch 5 is documentation-only. The schema and validator surfaces are unchanged from v0.4.0. Template skeletons and PIM guides are authoring aids, not normative artifacts — they do not alter what "conforms to ULC" means. The reference records in `examples/` remain the authoritative per-pattern references; templates are on-ramps for a manufacturer's first hand-authored record.

## 0.4.0 (2026-04-23)

Reference CLI validator and index builder. No normative schema or taxonomy changes: enum values, required fields, and validation rules are unchanged. Two `$defs/Index` description strings in `schema/ulc.schema.json` were edited to reference the new `ulc build-index` CLI instead of the retired `tools/build-index.py`, but the structural surface of the schema is identical to v0.3.0 and all four canonical reference records pass the new validator end-to-end unchanged.

### `ulc` command-line tool (new)

- **Language:** Go 1.22+, using `santhosh-tekuri/jsonschema/v6` for JSON Schema Draft 2020-12 validation with cross-file `$ref` resolution. Selected on 2026-04-22 after an independent re-evaluation that pivoted from an earlier tentative TypeScript + AJV choice — Go's stronger Draft 2020-12 compliance pedigree, static-binary distribution story, and manufacturer-CI fit made it the better language for the reference validator.
- **Module layout:** `go.mod` lives at the repo root (module `github.com/ulcspec/ULC`). The canonical `schema/` directory holds a small `embed.go` that bakes the JSON files into the binary via `//go:embed`, so the schemas have exactly one location in the tree and the shipped CLI always carries the matching spec version.
- **Location:** `tools/validator/`
- **Subcommands:**
  - `ulc validate <record.ulc>` runs JSON Schema Draft 2020-12 structural validation, builder parity (stored index matches the deterministic projection), source-file SHA-256 hash verification for files reachable on the local filesystem, and a conformance-grading stub. Emits `ERROR` / `WARNING` / `INFO` findings through a structured report.
  - `ulc build-index <record.ulc>` regenerates the record's index block. Supports `--check` and `--stdout` modes. Becomes the authoritative builder; the Python `tools/build-index.py` is retired.
  - `ulc version`, `ulc help`.
- **Output modes:** human-readable text by default, `--json` for machine-readable findings.
- **Distribution:** single-file binaries for Linux / macOS / Windows × x64 / arm64 via GoReleaser, built on tag push by `.github/workflows/release.yml`. Schemas are embedded into the binary via `go:embed`, so the CLI runs outside the source repository with no external files required.
- **Conformance grading rubric beyond structural / parity / hash checks is deferred** to a follow-up CLI release informed by manufacturer pilot feedback on what the `standard` and `full` levels should require.

### Retired Python tooling

- `tools/build-index.py` — retired. The Go CLI is the single source of truth for index projection logic. Users and CI invoke `ulc build-index` instead.
- `tools/builder-parity-guard.py` — retired. Parity is guaranteed by construction inside the Go binary.
- `tools/schema-drift-guard.py` — kept; still Python, still internal-only. Continues to verify every `$ref` pointer resolves across `taxonomy.schema.json` and `ulc.schema.json`.

### CI and automation updates

- Added `.github/workflows/release.yml` — cuts platform binaries on tag push via GoReleaser.
- Added `.github/workflows/validator-ci.yml` — `go vet`, `go test -race`, `go build`, end-to-end validation of every example record, and `goreleaser check` on every pull request.
- Updated `.github/workflows/schema-drift-guard.yml` — drops the Python builder-parity step; now builds the Go CLI and runs `ulc build-index --check` against example and template records.
- Updated `tools/hooks/pre-commit` — auto-detects `ulc` on `PATH` or at `tools/validator/bin/ulc`, auto-builds on first run when Go is available, and fails with a clear installation hint when neither the binary nor Go is present.

### Documentation

- Updated `README.md`, `CONTRIBUTING.md`, and `docs/authoring-patterns.md` to reference the Go CLI in place of the retired Python scripts.
- Added `tools/validator/README.md` with build instructions, feature checklist, and the relationship between the Go CLI and the (retained) Python drift guard.

## 0.3.0 (2026-04-22)

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
