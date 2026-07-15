# PIM mapping guides

How to emit ULC records from a Product Information Management (PIM) system or equivalent product-master database.

## Why this exists

Manufacturers with catalog-scale output (hundreds or thousands of SKUs across multiple product lines) do not hand-author ULC records. They emit them programmatically from the PIM that already holds their product data. The question is how to translate PIM-shaped data (products, attributes, categories, assets, relationships) into ULC-shaped records without losing provenance or conformance.

These guides describe, per PIM platform, how the typical PIM data model maps into the ULC schema. They are architectural and pragmatic, not prescriptive; every manufacturer's PIM implementation differs. Treat them as starting points that an integration engineer adapts to the actual attribute taxonomy in use.

## Guides

| PIM | Guide | Typical user |
|---|---|---|
| Salsify | [`salsify.md`](salsify.md) | Consumer brands, retail-focused lines, mid-market lighting |
| Akeneo | [`akeneo.md`](akeneo.md) | European mid-market, PHP-stack manufacturers |
| SAP | [`sap.md`](sap.md) | Large-enterprise manufacturers with SAP ERP |
| Custom / in-house | [`custom-pim.md`](custom-pim.md) | Legacy in-house product databases, common in traditional lighting manufacturers |

## Shared translation concerns

Every PIM mapping faces the same six problems. The per-PIM guides cover each system's specifics; the high-level pattern is common.

### 1. Product identity and the ULC record-per-scenario model

ULC separates **product family** (a cutsheet, shared across all SKUs in a family) from **configuration** (a single attested photometric scenario). Most PIMs model products at the SKU level without a separate scenario concept. The integration must decide how to project:

- One PIM product row → one ULC record per photometric scenario (typically one per CCT-and-distribution combination), with `product_family` replicated across each record in the family.
- Scenario identity in ULC is `<manufacturer-slug>-<catalog-slug>-<scenario-slug>`. The scenario-slug encodes axes that vary by IES file (CCT, distribution, wattage tier) even when they don't change the SKU.

The `applicability` block expresses which PIM SKUs each scenario record applies to. See `docs/authoring-patterns.md` for the four patterns (A/B/C/D) this maps into.

**Pattern-specific handling of `configuration.catalog_number`** (important: an emitter built without this distinction produces structurally wrong records for Pattern B and D):

- **Pattern A** (one record per SKU): `configuration.catalog_number` equals the variant SKU. `applicability.fixed_axes.catalog_number` equals the same value, and `applicable_sku_count_estimate: 1`. This is the default shape the per-platform identity mapping tables describe.
- **Pattern B** (one record per photometric scenario covering many SKUs via multiplier table): `configuration.catalog_number` carries only the tested-baseline SKU. The covered range of order codes lives in `applicability.covered_axes.<axis>` with a `derivation` rule containing a `multiplier_table`.
- **Pattern C** (one record per IES with provenance classes): each IES file is its own record; PIM-emit is essentially Pattern A with per-record `provenance.method` variation between `extracted` (for measured IES), `optical_simulation` (for simulated IES), and `extended_photometry` / `scaled` (for derived IES), with `base_attestation_ref` pointing at the root measured test.
- **Pattern D** (per-foot linear scaling): `configuration.catalog_number` is typically omitted; `applicable_catalog_pattern` uses a `{LENGTH}` placeholder; `covered_axes.length.derivation.method: "per_foot_linear_scaling"` carries the `linear_rate`; `photometry.per_length_normalized` and `photometry.declared_by_length[]` carry the per-length values.

Pick the pattern up front per product family, not per record. Building a Pattern-A-only emitter and retrofitting it later is substantially more rework than selecting the right pattern from the start.

### 2. Dual-unit fields

ULC requires SI-authoritative values with a derived Imperial companion for every length, mass, temperature, area, and mass-per-length field. PIMs typically store one unit. Emitters must:

- **Pick a canonical unit in the PIM** (millimeters for length, kilograms for mass, Celsius for temperature).
- **Compute the companion at emit time.** Do not round-trip through rounding that loses precision; `25.4 mm` is exactly `1.0 in`, not `1`.
- **Preserve significant figures.** If the PIM stores `113 mm`, emit `{mm: 113, in: 4.45}` not `{mm: 113, in: 4}`.

Imperial-first conversion (rare but possible in US-only manufacturer PIMs) works the same way in reverse.

### 3. Provenance

Every unit-bearing field in a ULC record carries a `provenance` object: `source` (for example `datasheet_pdf`, `ies`, `manufacturer_direct`) and `method` (`extracted`, `computed`, `transcribed`, `optical_simulation`).

For PIM-sourced values, the default provenance is:

```json
"provenance": {
  "source": "manufacturer_direct",
  "method": "transcribed"
}
```

More specific provenance applies when the PIM tracks the original source. If the PIM has a field like "source document: lab report LU-04412", the emitter should set `source: "ies"` or `source: "datasheet_pdf"` with an `attestation_ref` pointing at the attestation-id for that lab report. See the per-PIM guides for attribute-lineage patterns.

### 4. Source file references

ULC's `source_files[]` array requires a SHA-256 hash for every source file (cutsheet PDF, IES, LDT, TM-33, ULD). PIMs rarely store hashes natively. The emitter must:

- **Stream the file from PIM asset storage** at emit time.
- **Compute SHA-256** and include it in the `reference.sha256` field.
- **Pin the revision.** If the PIM has asset versioning, emit the specific version's hash, not whatever the "latest" pointer resolves to.

If the manufacturer serves cutsheet PDFs from a CDN, the `reference.url` field can point to the stable URL; `sha256` still anchors integrity even if the URL rots.

**The cutsheet file lives in two places in a ULC record.** A complete record carries `product_family.cutsheet` as its own `FileReference` (the family-level cutsheet pointer) in addition to the `source_files[]` entry with `file_type: "datasheet_pdf"`. An emitter that populates only `source_files[]` produces a record that grades `incomplete` rather than `core`, because `product_family.cutsheet` is a graded core requirement (it is not schema-required, so the record still validates and carries a roadmap naming the cutsheet). Populate both places with the same filename, sha256, revision label, and revision date from the single computed hash; the two blocks carry different consumer semantics (family identity vs. integrity-tracked source-file list) but refer to the same byte-identical file.

### 5. Category and enum mapping

PIMs use free-string or manufacturer-specific category taxonomies. ULC uses closed enums (`primary_category`, `mounting_types`, `environment_rating`, `housing_material`, and dozens more).

The integration must maintain a **mapping table from PIM category labels to ULC enum values** alongside the ETL code. Typical pattern:

```
PIM "Recessed Downlight"     → primary_category: "downlight", mounting_types: ["recessed_ceiling"]
PIM "Pendant Linear"         → primary_category: "linear", mounting_types: ["pendant"]
PIM "Exterior Wall Pack"     → primary_category: "bulkhead_wall_pack", mounting_types: ["surface_wall"]
PIM "Industrial High Bay"    → primary_category: "high_bay", mounting_types: ["pendant", "surface_ceiling"]
PIM "Exit Sign"              → primary_category: "exit_sign", mounting_types: ["surface_wall", "surface_ceiling"]
PIM "Emergency Bug-Eye"      → primary_category: "emergency_luminaire", mounting_types: ["surface_wall"]
```

The mapping table lives in the integration code, version-controlled, and reviewed with every new ULC schema release (new enum values may land that expand the mapping options).

An `exit_sign` record grades against the exit-sign dataset (legend, illumination mode, face luminance) and an `emergency_luminaire` against the emergency-power dataset, not against architectural photometry. Map those PIM categories to their own ULC classes rather than forcing them into a general luminaire category, or the grader measures the record against the wrong requirements.

### 6. Index generation

ULC's `index` block is a denormalized projection of values from the deep blocks. It must NOT be hand-authored; the reference `ulc build-index` CLI produces it deterministically. The PIM emitter pipeline looks like:

```
PIM data → transform to deep blocks → write record to a temp .ulc.json file
         → run `ulc build-index <tmpfile>` (writes the computed index in place)
         → run `ulc validate <tmpfile>` (exits 1 on ERROR findings)
         → on success, publish the file
```

Both `ulc build-index` and `ulc validate` are file-path CLIs: they read from and write to files on disk, they do not read from stdin. `ulc build-index --stdout` emits only the computed index object (not a full merged record), so most emitters use the default in-place mode and then call `ulc validate` on the same file. The transform code emits the deep blocks; the CLI handles the index plus validation. Shell out to the Go `ulc` binary from the emitter (Python, Java, Node, whatever).

`ulc build-index` stamps three computed grading rollups into the index (alongside its denormalized projections of manufacturer, category, photometry, and source files), all derived from the deep blocks, all parity-guarded, none hand-authored:

- `index.conformance_level`: the achieved data-completeness grade on the four-level ladder `incomplete` < `core` < `standard` < `full`. A record reaches whatever level its data supports; PIMs get the level for free and never set it.
- `index.achievements`: the per-theme Product Achievements picture, the second grading axis, orthogonal to conformance. An attestation whose program is assigned to an achievement theme contributes `claimed` on that token alone, and reaches `documented` only when it carries a `source_document_ref` (a filename plus a 64-hex SHA-256) that attaches evidence. Only theme-assigned programs feed a theme here; an unthemed program (a safety listing, a test method, a market-access mark) is still projected into `index.attestation_programs` but contributes to no achievement state. Carry `valid_until` on any dated attestation so its expiry is evaluable, and put a `sustainability_declaration`'s dated `expiration_date` on that block, never on an attestation. See `docs/methodology.md` for the achievement themes and the documented-vs-claimed rule.
- `index.restricted_substances_declared`: the sibling legal-floor flag listing the restricted-substances programs (RoHS, REACH, and similar) the ledger declares. Table-stakes legality, never a prestige achievement, so it sits beside the themes rather than inside one.

Because the index is a projection, re-run `ulc build-index` after any deep-block edit so the stored index stays parity-valid, and re-stamp `record_status_as_of` to the edit date so record-relative expiry is evaluated against the current review rather than a stale one. (Re-stamping only changes the index when the new date crosses an attestation's expiry boundary; its own job is accurate expiry dating, not parity.) Schedule a periodic `ulc validate --expiry` over the published corpus to preview attestation and declaration expiry before a re-stamp makes a downgrade normative.

## Out of scope

These guides describe the PIM-to-ULC transformation. They do not cover:

- **No PIM?** These guides assume a PIM or other structured master. If your product data lives in spreadsheets instead, use the `ulc from-sheet` converter (`tools/validator/`, with the workbook template in `templates/workbook/`): it turns a workbook into validated records directly, with no mapping integration to build.
- **Reading data out of PDFs or photometric files.** ULC never parses a cutsheet PDF, IES, or LDT for its data; those are referenced as source files by SHA-256 (the bytes are read only to compute and verify that hash, never parsed for content). Capturing data that exists only in unstructured documents into structured form (a spreadsheet or the PIM) is a one-time task upstream of both these guides and the converter.
- **Schema validation.** The `ulc validate` CLI handles that; the emitter calls it as a post-step.

## See also

- `templates/workbook/`: the fill-in workbook for the `ulc from-sheet` converter, the path for authoring records without a PIM.
- `schema/ulc.schema.json`: the target schema the emitter produces.
- `docs/authoring-patterns.md`: the four manufacturer authoring patterns and how to pick which one applies per SKU family.
- `tools/validator/README.md`: CLI reference for `ulc build-index` and `ulc validate`.
