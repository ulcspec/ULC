# ULC emit from Salsify

Salsify is a cloud PIM popular with consumer-facing brands, retail-syndicated product lines, and mid-market lighting manufacturers distributing through specifier and wholesale channels. This guide shows how a typical Salsify organization can emit ULC records.

## Salsify data model in one page

- **Products** are the core entity. Each product is identified by an internal ID and one or more external IDs (SKU, UPC, MPN).
- **Properties** (Salsify's term for attributes) are typed fields on products. Properties can be strings, numbers, enums, digital assets, or references to other products.
- **Product relationships** model accessories, replacements, and bundle components through typed reference properties.
- **Digital assets** (images, PDFs, IES files as arbitrary binaries) are first-class objects with their own IDs, versioning, and renditions.
- **Channels and recipes** define per-channel export transformations — the hook into which the ULC emitter plugs.
- **APIs** — GraphQL for rich queries, REST for bulk operations and webhooks, CSV for imports.

The ULC emitter is typically implemented as a Salsify channel recipe plus an external transform service. The recipe exports the relevant properties and asset manifests; the transform service produces ULC JSON and posts it wherever ULC records publish (GitHub, manufacturer website, CDN).

## Mapping table

### Identity

| Salsify | ULC path |
|---|---|
| Parent product identifier / catalog model property | `product_family.catalog_model` |
| Variant external ID (SKU) | `configuration.catalog_number` |
| Internal brand property | `product_family.manufacturer.slug`, `product_family.manufacturer.display_name` |
| Product line / series property | `product_family.catalog_line` |
| Derived full slug `<manufacturer>-<sku>-<scenario>` | `record_id` |
| Derived scenario-local slug `<family>-<cct>-<distribution>` | `configuration.photometric_scenario_id` |

A product family in Salsify is typically modeled as a parent product with child variants (one per orderable SKU). The parent carries cutsheet-level shared data and maps to `product_family.*`; each child variant maps to its own `configuration.catalog_number` and a distinct ULC record. See the multi-CCT handling note under "Gotchas" below for the common parent-variant pattern.

### Category and mounting

Salsify's category taxonomy is typically a hierarchical property (for example `luminaire/indoor/recessed/downlight`). The emitter maps the leaf (or a leaf+parent combination) into ULC enums:

| Salsify category | ULC `primary_category` | ULC `mounting_types` |
|---|---|---|
| `.../recessed/downlight` | `downlight` | `["recessed_ceiling"]` |
| `.../pendant/linear` | `linear` | `["pendant"]` |
| `.../exterior/wall-pack` | `bulkhead_wall_pack` | `["surface_wall"]` |
| `.../industrial/high-bay` | `high_bay` | `["pendant", "surface_ceiling"]` |
| `.../outdoor/bollard` | `bollard` | `["surface_floor"]` |

### Dimensional properties

Salsify stores each dimension as a typed numeric property plus a unit property (or a combined "dimension" property type). Common pattern: one value in the authoritative unit, convert at emit time.

| Salsify property | ULC path | Notes |
|---|---|---|
| `overall_diameter_mm` | `product_family.physical_dimensions.overall_diameter.{mm, in}` | Convert mm → in at emit |
| `overall_length_mm` | `product_family.physical_dimensions.overall_length.{mm, in}` | For linear fixtures |
| `recess_depth_mm` | `product_family.physical_dimensions.recess_depth.{mm, in}` | Downlights only |
| `ceiling_aperture_mm` | `product_family.physical_dimensions.ceiling_aperture.{mm, in}` | Downlights only |
| `weight_kg` | `product_family.physical_dimensions.luminaire_mass.{kg, lb}` | Convert kg → lb |

All dimensional fields carry provenance (`source: "datasheet_pdf"`, `method: "transcribed"`) unless the PIM tracks a more specific origin.

### Electrical and photometric

Salsify stores these as scalar properties, one per tested scenario. For a multi-CCT family, each CCT typically gets its own property group or a variant product.

| Salsify property | ULC path |
|---|---|
| `input_power_w` | `electrical.input_power_w.value` |
| `rated_voltage_v` | `electrical.input_voltage_v.value` and `operating_point.input_voltage_v.value` |
| `driver_protocol` (enum property) | `electrical.driver_protocol` |
| `total_luminous_flux_lm` | `photometry.total_luminous_flux_lm.value` |
| `luminaire_efficacy_lm_per_w` | `photometry.luminaire_efficacy_lm_per_w.value` |
| `beam_angle_deg` | `photometry.beam_angle_deg.value` |
| `nominal_cct_k` | `colorimetry.nominal_cct_k`, `configuration.tested_conditions.nominal_cct_at_test` |
| `cri_ra` | `colorimetry.cri_ra.value` |

### Digital assets

Salsify's asset model maps cleanly to ULC's `source_files[]`:

| Salsify digital-asset role | ULC `source_files[].file_type` |
|---|---|
| `cutsheet_pdf` | `datasheet_pdf` |
| `photometric_ies` | `ies` |
| `photometric_ldt` | `ldt` |
| `installation_instructions` | `installation_instructions_pdf` |

The emitter streams each asset by its Salsify asset ID, computes SHA-256, and populates `source_files[].reference.{filename, sha256, url, revision_label, revision_date}`. `revision_label` comes from Salsify's asset version number; `revision_date` from the asset's `updated_at` timestamp.

### Relationships to accessories

Salsify models compatible accessories as typed product references:

```
Product "Downlight 30416.023"
  property compatible_junction_boxes → [Product "Junction box 88557.023"]
  property compatible_plaster_frames → [Product "Plaster frame 82969.023"]
```

The emitter walks these references and populates `compatible_accessories[]`:

```json
{
  "accessory_catalog_number": "88557.023",
  "display_name": "Junction box (metal, galvanized)",
  "accessory_type": "junction_box",
  "changes_photometry": false
}
```

Accessory-type classification requires another PIM-to-ULC enum mapping (junction_box, plaster_frame, ic_box, trim, mounting_ring, etc.). Maintain the mapping table next to the category mapping.

## Gotchas

1. **Multi-CCT product families** are commonly modeled in Salsify as a parent product with variant children. One child per CCT becomes one ULC scenario record; the shared `product_family` block derives from the parent. Treat the parent as the record-grouper, not a separate ULC record.
2. **Localization**. Salsify properties support per-locale values. ULC is locale-neutral at the schema level but accepts display-name fields that look best in the manufacturer's primary locale. The emitter chooses a canonical locale (typically en-US or the manufacturer's home market).
3. **Property type mismatches.** Salsify's "number" type is a double. ULC distinguishes `integer` in some fields (minItems counts, step counts). Coerce at emit time — do not emit `1.0` where the schema expects an integer.
4. **Asset digest caching.** Computing SHA-256 on every asset for every record on every run is expensive. Cache by Salsify asset ID + version and invalidate on `updated_at` change.
5. **Missing required values.** If a Salsify property is blank for a given SKU, the emitter must decide: skip the record, emit at a lower `conformance_level`, or fail. The pragmatic default: emit at `conformance_level: "core"` when measured values are missing, and upgrade automatically when the PIM property becomes populated.

## Emit flow

```
1. Salsify channel export ships (product + properties + asset manifest) to transform service
2. Transform service walks products, groups by family, iterates scenarios
3. For each scenario:
   - Map identity (slug, record_id, scenario_id)
   - Map category via enum table
   - Pull dimensions, compute SI/Imperial companion
   - Pull optical properties from the scenario's property group
   - Stream assets, compute SHA-256
   - Walk accessory references, populate compatible_accessories
   - Assemble JSON record and write to a temp file (the CLI is file-based, not stdin-based)
   - Shell out: `ulc build-index <tmpfile>` — writes the computed index back into the file
   - Shell out: `ulc validate <tmpfile>` — exits 1 on ERROR findings
   - On success: publish the file to its destination
   - On validation failure: log, skip, alert integration owner
```

## Example transform skeleton (Python)

```python
# Illustrative pseudocode, not a working implementation.
def emit_ulc_from_salsify(product, scenario):
    record = {
        "ulc_version": "0.3.0",
        "record_id": f"{product.brand_slug}-{product.sku_slug}-{scenario.slug}",
        "record_status": "active",
        "conformance_level": "core",
        "product_family": build_family_from_salsify(product),
        "configuration": build_configuration_from_scenario(scenario),
        "electrical": map_electrical(scenario),
        "photometry": map_photometry(scenario),
        "colorimetry": map_colorimetry(scenario),
        "source_files": build_source_files_with_hashes(product.assets),
    }
    # Both CLIs are file-based; write a temp file, run build-index in place,
    # then run validate against the same path.
    with tempfile.NamedTemporaryFile("w", suffix=".ulc.json", delete=False) as f:
        json.dump(record, f)
        tmp_path = f.name
    subprocess.run(["ulc", "build-index", tmp_path], check=True)
    validation = subprocess.run(["ulc", "validate", tmp_path], capture_output=True)
    if validation.returncode != 0:
        alert(product.sku, validation.stderr)
        return None
    return tmp_path
```

## See also

- [`README.md`](README.md) — shared PIM-to-ULC translation concerns.
- `templates/` — starter ULC skeletons to reference while building the emitter.
- Salsify API documentation (product and digital-asset schemas) for the actual property and asset field names in use.
