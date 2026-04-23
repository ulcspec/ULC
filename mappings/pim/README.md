# PIM mapping guides

How to emit ULC records from a Product Information Management (PIM) system or equivalent product-master database.

## Why this exists

Manufacturers with catalog-scale output — hundreds or thousands of SKUs across multiple product lines — do not hand-author ULC records. They emit them programmatically from the PIM that already holds their product data. The question is how to translate PIM-shaped data (products, attributes, categories, assets, relationships) into ULC-shaped records without losing provenance or conformance.

These guides describe, per PIM platform, how the typical PIM data model maps into the ULC schema. They are architectural and pragmatic, not prescriptive — every manufacturer's PIM implementation differs. Treat them as starting points that an integration engineer adapts to the actual attribute taxonomy in use.

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

### 5. Category and enum mapping

PIMs use free-string or manufacturer-specific category taxonomies. ULC uses closed enums (`primary_category`, `mounting_types`, `environment_rating`, `housing_material`, and dozens more).

The integration must maintain a **mapping table from PIM category labels to ULC enum values** alongside the ETL code. Typical pattern:

```
PIM "Recessed Downlight"     → primary_category: "downlight", mounting_types: ["recessed_ceiling"]
PIM "Pendant Linear"         → primary_category: "linear", mounting_types: ["pendant"]
PIM "Exterior Wall Pack"     → primary_category: "bulkhead_wall_pack", mounting_types: ["surface_wall"]
PIM "Industrial High Bay"    → primary_category: "high_bay", mounting_types: ["pendant", "surface_ceiling"]
```

The mapping table lives in the integration code, version-controlled, and reviewed with every new ULC schema release (new enum values may land that expand the mapping options).

### 6. Index generation

ULC's `index` block is a denormalized projection of values from the deep blocks. It must NOT be hand-authored — the reference `ulc build-index` CLI produces it deterministically. The PIM emitter pipeline looks like:

```
PIM data → transform to deep blocks → write .ulc.json → run `ulc build-index --stdout` → merge index → run `ulc validate` → publish
```

This is easier than it looks. The transform emits the deep blocks; the CLI handles the index + validation. Shell out to the Go `ulc` binary from the emitter (Python, Java, Node, whatever) and pipe JSON through it.

## Out of scope

These guides describe the PIM-to-ULC transformation. They do not cover:

- **PDF extraction** — if the PIM is downstream of a hand-maintained cutsheet PDF with no structured product data, extracting ULC from the PDF is a separate upstream task (not a PIM-mapping task). A dedicated extraction pipeline would sit upstream of the PIM and populate the PIM's attributes.
- **IES / LDT parsing** — similar: parsing photometric files into the PIM is a separate pipeline that produces the PIM's optical attributes.
- **Schema validation** — the `ulc validate` CLI handles that. The emitter calls it as a post-step.

## See also

- `templates/` — starter `.ulc.json` skeletons for hand-authoring one-off records.
- `schema/ulc.schema.json` — the target schema the emitter produces.
- `docs/authoring-patterns.md` — the four manufacturer authoring patterns and how to pick which one applies per SKU family.
- `tools/validator/README.md` — CLI reference for `ulc build-index` and `ulc validate`.
