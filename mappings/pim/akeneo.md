# ULC emit from Akeneo

Akeneo is an open-source PIM popular with European manufacturers, PHP-stack shops, and mid-market lighting brands. This guide shows how a typical Akeneo installation can emit ULC records.

## Akeneo data model in one page

- **Products** are the core entity, identified by identifier (SKU).
- **Families** group products that share an attribute set. A downlight family might include `input_power_w`, `beam_angle_deg`, `recess_depth_mm`; a linear-pendant family adds `overall_length_mm` and `linear_mass_per_foot`.
- **Attribute groups** organize attributes within a family (`electrical`, `photometry`, `colorimetry`, `physical`).
- **Variants and variant groups** model SKU-level variation within a family. A parent product-model holds shared attributes; variants carry the per-SKU differences (CCT, distribution, finish).
- **Categories** are a separate hierarchy from families — used for marketing navigation and channel filtering.
- **Channels** define per-channel localization and completeness rules. A ULC emit channel can enforce that only records with complete photometric data ship.
- **Assets** (PIM Enterprise Edition) or **reference entities** (Community Edition) hold cutsheet PDFs, IES files, and LDT files.
- **API** — REST with OAuth2 for both reads and writes.

The ULC emitter is typically implemented as an external service that pulls from Akeneo's REST API on a scheduled export or webhook trigger.

## Mapping table

### Identity

| Akeneo | ULC path |
|---|---|
| Product `identifier` | `configuration.catalog_number` |
| Product model code (parent) | `product_family.family_id`, `product_family.catalog_model` |
| Brand attribute | `product_family.manufacturer.slug`, `product_family.manufacturer.display_name` |
| Product line / series | `product_family.catalog_line` |
| Derived full slug `<manufacturer>-<sku>-<scenario>` | `record_id` |
| Derived scenario-local slug `<family>-<cct>-<distribution>` | `configuration.photometric_scenario_id` |

The "Product `identifier` → `configuration.catalog_number`" mapping is the Pattern A default (one record per SKU). If a product-model family publishes one ULC record that covers many variant SKUs via a multiplier table (Pattern B) or per-foot linear scaling (Pattern D), `configuration.catalog_number` carries only the tested-baseline SKU, and the covered SKU range is declared in `applicability.covered_axes.<axis>` with a per-axis derivation rule. See `docs/authoring-patterns.md` for worked examples.

### Family to primary category

Akeneo's family codes map to ULC primary categories one-to-one in a well-designed taxonomy:

| Akeneo family | ULC `primary_category` | ULC `mounting_types` |
|---|---|---|
| `downlight_recessed` | `downlight` | `["recessed_ceiling"]` |
| `linear_pendant` | `linear` | `["pendant"]` |
| `wall_pack_exterior` | `bulkhead_wall_pack` | `["surface_wall"]` |
| `high_bay_industrial` | `high_bay` | `["pendant", "surface_ceiling"]` |
| `bollard_outdoor` | `bollard` | `["surface_floor"]` |
| `sconce_interior` | `sconce` | `["surface_wall"]` |

If the manufacturer's family taxonomy differs from this pattern, the mapping table is the bridge between their convention and ULC's closed enum.

### Dimensional attributes

Akeneo attributes with type `METRIC` carry both value and unit (`{amount: 113, unit: "MILLIMETER"}`). The emitter converts to ULC dual-unit:

| Akeneo attribute | ULC path |
|---|---|
| `overall_diameter` (METRIC, MILLIMETER) | `product_family.physical_dimensions.overall_diameter.{mm, in}` |
| `overall_length` (METRIC, MILLIMETER) | `product_family.physical_dimensions.overall_length.{mm, in}` |
| `recess_depth` (METRIC, MILLIMETER) | `product_family.physical_dimensions.recess_depth.{mm, in}` |
| `weight` (METRIC, KILOGRAM) | `product_family.physical_dimensions.luminaire_mass.{kg, lb}` |
| `ambient_min` (METRIC, CELSIUS) | `product_family.shared_mechanical.ambient_operating_range.min.{c, f}` |

Attributes with type `NUMBER` are unitless; the emitter pairs them with the expected ULC unit from the schema. The mapping table must record expected unit per attribute.

### Electrical and photometric

In Akeneo, these typically live on the variant (the SKU-level product) rather than the parent product-model, since they vary per CCT/distribution:

| Akeneo attribute (on variant) | ULC path |
|---|---|
| `input_power_w` | `electrical.input_power_w.value` |
| `rated_voltage_v` | `electrical.input_voltage_v.value` |
| `driver_protocol` (SIMPLESELECT enum) | `electrical.driver_protocol` |
| `total_luminous_flux_lm` | `photometry.total_luminous_flux_lm.value` |
| `beam_angle_deg` | `photometry.beam_angle_deg.value` |
| `cct_k` (SIMPLESELECT) | `colorimetry.nominal_cct_k`, `configuration.tested_conditions.nominal_cct_at_test` |
| `cri_ra` | `colorimetry.cri_ra.value` |

Akeneo's `SIMPLESELECT` attribute type (enum) is the natural fit for ULC enum fields. Maintain the option code correspondence in the mapping table.

### Assets and reference entities

In PIM Enterprise Edition, cutsheet PDFs and IES files live as **assets** with their own metadata, file reference, and tags. The emitter queries assets linked to a product, filters by asset family (`cutsheet`, `ies`, `ldt`), streams the file, and computes SHA-256.

In Community Edition (no Asset Manager), cutsheet files typically live as plain `FILE` or `IMAGE` attributes on the product. The approach is the same: read the file path, stream, hash.

| Akeneo asset family / attribute | ULC `source_files[].file_type` |
|---|---|
| `cutsheet` asset | `datasheet_pdf` |
| `photometric_ies` asset | `ies` |
| `photometric_ldt` asset | `ldt` |
| `installation_pdf` asset | `installation_instructions_pdf` |

Asset version maps to `reference.revision_label`; `updated_at` to `reference.revision_date`.

The cutsheet asset populates **both** `source_files[]` (as the entry with `file_type: "datasheet_pdf"`) **and** `product_family.cutsheet` (which `ProductFamily.required` makes mandatory). Stream and hash once, write the same `FileReference` fields in both places; an emitter that only populates `source_files[]` produces schema-invalid records.

### Locales and channels

Akeneo's localized attributes deliver per-language values. ULC is locale-neutral at the schema level. The emitter chooses the canonical locale (typically `en_US` for North American markets, `en_GB` for Europe, or the manufacturer's primary locale).

The ULC emit can be modeled as an Akeneo channel (`ulc_export`) with its own completeness rule: a product is not ULC-exportable until all required attributes for the target `conformance_level` are populated.

## Gotchas

1. **Product-model vs variant split.** Akeneo's 2-level model (parent product-model holding shared attributes, variants holding per-SKU) maps exactly onto ULC's `product_family` (shared) vs `configuration` (per-scenario) split. Families with 3+ levels of variation (rare) require a chained emit.
2. **Attribute completeness.** Akeneo tracks per-channel-and-locale completeness. Tie the ULC emit to the `ulc_export` channel's completeness so partially-populated SKUs don't ship as broken ULC records.
3. **METRIC unit drift.** If different variants store the same attribute in different units (unusual but possible), normalize to the canonical unit before computing the companion.
4. **Reference entity depth.** In PIM Enterprise, an attestation (UL-listed, DLC-qualified) can be modeled as a reference entity with its own structured attributes. Deep-walk these into ULC `attestations[]` entries with `program`, `status`, `value_type`, and `verification` populated.
5. **Published vs draft.** Akeneo's workflow distinguishes products in draft state from published products. Never emit ULC from a draft product — wait for the workflow to publish.

## Emit flow

```
1. Scheduled job or webhook triggers emitter
2. Emitter queries Akeneo REST API:
   - GET /api/rest/v1/products?search={category=in:[LUMINAIRE_CATEGORIES],updated_at>LAST_RUN}
   - Paginate until complete
3. For each product:
   - Look up product-model parent (if any)
   - For each variant (or for the product itself if no variants):
     - Map identity
     - Map family → primary_category
     - Pull METRIC attributes, convert to dual-unit
     - Pull enum attributes, map via code table
     - Stream assets, compute SHA-256
     - Assemble JSON
     - ulc build-index + validate
     - Publish on success
4. Log failures with SKU + validation output for integration-engineer review
```

## Example transform skeleton (PHP, Akeneo's native stack)

```php
// Illustrative pseudocode, not a working implementation.
function emitUlcFromAkeneo(Product $product, Variant $variant): ?string {
    $family = $product->getFamily()->getCode();
    $primaryCategory = FAMILY_TO_ULC_CATEGORY[$family] ?? null;
    if ($primaryCategory === null) {
        return null;
    }
    $record = [
        'ulc_version' => '0.3.0',
        'record_id' => slug("{$product->getBrand()}-{$product->getIdentifier()}-{$variant->getScenarioSlug()}"),
        'record_status' => 'active',
        'conformance_level' => 'core',
        'product_family' => buildFamily($product, $primaryCategory),
        'configuration' => buildConfiguration($variant),
        'electrical' => mapElectrical($variant),
        'photometry' => mapPhotometry($variant),
        'colorimetry' => mapColorimetry($variant),
        'source_files' => buildSourceFiles($product->getAssets()),
    ];
    // Write $record to a temp .ulc.json file, run `ulc build-index <path>`
    // (writes the computed index back in place), then `ulc validate <path>`.
    // Both CLIs take a file path; neither reads stdin.
    $tmpPath = writeTempUlcRecord($record);
    exec("ulc build-index " . escapeshellarg($tmpPath), $out, $rc);
    if ($rc !== 0) { return null; }
    exec("ulc validate " . escapeshellarg($tmpPath), $out, $rc);
    return $rc === 0 ? $tmpPath : null;
}
```

## See also

- [`README.md`](README.md) — shared PIM-to-ULC translation concerns.
- Akeneo REST API documentation for the actual endpoint shapes.
- `templates/` — starter ULC skeletons for structure reference.
