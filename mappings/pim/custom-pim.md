# ULC emit from a custom or in-house PIM

Traditional lighting manufacturers often run a home-grown product-master database rather than a commercial PIM. Typical stacks are Oracle or PostgreSQL databases with a web admin built in Django, Rails, Spring, or .NET. This guide shows the patterns that apply to any in-house schema.

## Why a pattern guide instead of a platform-specific one

Every in-house PIM is different. The specifics of table names, ORM classes, and admin workflows are manufacturer-specific. What's consistent is the shape of the problem: converting a normalized relational product schema into the denormalized ULC JSON record per photometric scenario.

This guide describes the common patterns. Adapt them to the actual schema in use.

## Typical in-house data model

The common shape, regardless of framework:

- **Products table** — one row per SKU or per product family, with columns for identity, category, basic dimensions, and rating.
- **Attributes table** (key/value or EAV) — for values that vary per product or per scenario, often stored as a property bag to avoid schema churn.
- **Categories table** — hierarchical or flat.
- **Files / assets table** — cutsheet PDFs, IES files, LDT files; often with a file-storage pointer (S3 key, SAN path) rather than the file bytes.
- **Relationships table** — accessory compatibility, replacement chains, kit composition.
- **Test reports table** (sometimes) — lab attestations with doc references.

Sometimes the schema is fully normalized (one column per attribute, strongly typed); sometimes it's a "products + JSON blob" hybrid. Both work for ULC emit.

## Mapping patterns

### Identity

```sql
-- Typical query
SELECT p.id, p.manufacturer, p.model, p.series, p.category_id
FROM products p
WHERE p.status = 'active'
```

| SQL / ORM column | ULC path |
|---|---|
| `manufacturer` | `product_family.manufacturer.slug`, `product_family.manufacturer.display_name` |
| `model` | `product_family.catalog_model`, `configuration.catalog_number` |
| `series` | `product_family.catalog_line` |
| Derived full slug `<manufacturer>-<sku>-<scenario>` | `record_id` |
| Derived scenario-local slug `<family>-<cct>-<distribution>` | `configuration.photometric_scenario_id` |

### Category

The integration maintains a mapping table from the in-house `category_id` (or `category_label`) to ULC enum values. Store this table as a version-controlled config file, not as another database table — the ULC enum changes with schema releases and should track the spec version.

```python
# Example config
CATEGORY_TO_ULC = {
    "recessed_downlight": ("downlight", ["recessed_ceiling"]),
    "surface_downlight": ("downlight", ["surface_ceiling"]),
    "linear_pendant": ("linear", ["pendant"]),
    "wallpack_exterior": ("bulkhead_wall_pack", ["surface_wall"]),
    "highbay_industrial": ("high_bay", ["pendant", "surface_ceiling"]),
    "bollard_outdoor": ("bollard", ["surface_floor"]),
    "sconce_interior": ("sconce", ["surface_wall"]),
}
```

### Dimensional fields

In-house PIMs usually store dimensions in a single authoritative unit. Convert at emit time to dual-unit:

```python
def dual_unit_length(mm: float) -> dict:
    return {
        "mm": mm,
        "in": round(mm / 25.4, 4),
        "value_type": "rated",
        "provenance": {
            "source": "datasheet_pdf",
            "method": "transcribed",
        },
    }
```

Common columns to mappings:

| DB column | ULC path |
|---|---|
| `overall_diameter_mm` | `product_family.physical_dimensions.overall_diameter` |
| `overall_length_mm` | `product_family.physical_dimensions.overall_length` |
| `recess_depth_mm` | `product_family.physical_dimensions.recess_depth` |
| `weight_kg` | `product_family.physical_dimensions.luminaire_mass` |

### Scenario expansion

In-house schemas often have a single product row with multiple test reports or IES files linked, one per scenario (CCT, distribution). The emit pattern:

```
For each product:
    For each scenario (CCT × distribution combination):
        Emit one ULC record
        product_family block is shared (derived from product row)
        configuration, electrical, photometry blocks come from the scenario-specific IES + test report
```

The scenario enumeration typically joins the products table to a photometric-test-report table:

```sql
SELECT p.id AS product_id, t.test_id, t.cct_k, t.distribution_label,
       t.input_power_w, t.total_flux_lm, t.beam_angle_deg,
       t.cri_ra, t.ies_file_id, t.report_pdf_id
FROM products p
JOIN photometric_tests t ON t.product_id = p.id
WHERE p.status = 'active' AND t.is_current = TRUE
```

### Assets and file hashing

```python
def build_source_file_entry(file_record):
    with open(file_record.storage_path, 'rb') as f:
        data = f.read()
        sha = hashlib.sha256(data).hexdigest()
    return {
        "file_type": FILE_TYPE_MAP[file_record.kind],
        "reference": {
            "filename": file_record.original_filename,
            "sha256": sha,
            "revision_date": file_record.uploaded_at.date().isoformat(),
        },
    }
```

If files live on S3 or a remote store, stream via the storage SDK rather than downloading into memory for large files. Cache the (storage-key, version) → hash mapping; the hash is stable as long as the bytes don't change.

### Attestations

If the in-house schema tracks test reports as first-class rows, map each to a ULC `attestations[]` entry:

```sql
SELECT attestation_id, program_code, status, issued_date, test_report_id
FROM product_attestations
WHERE product_id = :product_id
```

Enum mapping for `program` values comes from a config table (similar to category mapping), tracking in-house codes (`UL1598`, `DLC_PREMIUM`) to ULC enum values (`ul_1598`, `dlc_premium`).

## Gotchas

1. **Schemas without scenarios.** If the in-house PIM has one row per SKU without a concept of "test scenario," the emitter must synthesize scenarios from available photometric data (one scenario per IES file). If there's only one IES file per SKU and no CCT-variation metadata, that's Pattern A (simplest).
2. **Legacy free-text fields.** In-house PIMs frequently have free-text description fields that mix structured data (`"90 CRI, 3000K, 120V"`). Extract these via regex into structured fields during ETL, or flag the record for manual review before emit.
3. **Unit-of-measure inconsistency.** Older in-house schemas may mix metric and Imperial in the same column (`"4.5 in"` in a string column). Normalize during ETL before emit.
4. **Missing provenance history.** If the PIM was hand-populated over years with no source tracking, default all field provenance to `{source: "manufacturer_direct", method: "transcribed"}` and plan a follow-up pass to enrich provenance when lab reports get linked.
5. **No change tracking.** Without change-data-capture, the emitter must either run full sweeps periodically or poll `updated_at` columns. Full sweeps are the simpler starting point.
6. **Accessory compatibility stored as free text.** If compatible accessories are free-text lists rather than typed relationships, the emitter can do a best-effort parse but should flag unmapped entries for manual review rather than drop them silently.

## Migration path

For a manufacturer moving from in-house PIM → commercial PIM (Salsify, Akeneo, SAP) in the future, the ULC emitter pattern remains the same; only the input-side query changes. The transform logic (category enum mapping, dual-unit conversion, SHA-256 hashing, attestation mapping, `ulc build-index` + `validate` shell-out) is portable.

Design the emitter to take a normalized "flattened product record" as input and return a ULC JSON. The input-side adapter (SQL queries, ORM calls) becomes a thin layer that future migrations can replace.

## Emit flow

```
1. Scheduled job queries products table (and joined test-report, file, attestation tables)
2. For each row:
   - Group by product family; iterate scenarios
   - Map category via config table
   - Convert dimensions to dual-unit
   - Pull files, compute SHA-256
   - Walk attestations, map program codes
   - Assemble JSON
   - Shell out to ulc build-index + ulc validate
   - On success: publish to chosen destination (GitHub, CDN, internal staging)
   - On failure: log with product identifier + validation output
```

## Example transform skeleton (Python + SQLAlchemy)

```python
# Illustrative pseudocode.
def emit_ulc(session: Session):
    products = session.query(Product).filter_by(status='active').all()
    for product in products:
        primary_category, mounting = CATEGORY_TO_ULC[product.category.code]
        family = build_family(product, primary_category, mounting)
        for scenario in product.photometric_scenarios:
            record = {
                "ulc_version": "0.3.0",
                "record_id": slug(f"{product.manufacturer}-{product.model}-{scenario.slug}"),
                "record_status": "active",
                "conformance_level": "core",
                "product_family": family,
                "configuration": build_configuration(scenario),
                "electrical": build_electrical(scenario),
                "photometry": build_photometry(scenario),
                "colorimetry": build_colorimetry(scenario),
                "source_files": build_source_files(scenario.files + [product.cutsheet]),
            }
            # Both CLIs are file-based; write a temp file, run build-index
            # in place, then run validate against the same path.
            with tempfile.NamedTemporaryFile("w", suffix=".ulc.json", delete=False) as f:
                json.dump(record, f)
                tmp_path = f.name
            subprocess.run(["ulc", "build-index", tmp_path], check=True)
            # Capture stdout; the CLI writes its findings report there and
            # reserves stderr for parse-failure diagnostics.
            validation = subprocess.run(["ulc", "validate", tmp_path], capture_output=True, text=True)
            if validation.returncode == 0:
                publish(tmp_path)
            else:
                log_failure(product, scenario, validation.stdout or validation.stderr)
```

## See also

- [`README.md`](README.md) — shared PIM-to-ULC translation concerns.
- [`salsify.md`](salsify.md), [`akeneo.md`](akeneo.md), [`sap.md`](sap.md) — platform-specific guides. A manufacturer planning to migrate to a commercial PIM may find those useful as the target architecture.
- `templates/` — ULC skeletons for structure reference.
