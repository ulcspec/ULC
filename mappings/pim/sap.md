# ULC emit from SAP

SAP ERP and adjacent SAP master-data products (SAP MDG, SAP PLM) hold the product data for most large-enterprise luminaire manufacturers. This guide shows how ULC records can be emitted from a typical SAP landscape.

## SAP data model in one page

SAP's product-master story is spread across several modules. Emitters typically pull from more than one:

- **SAP MM (Material Management)** — the core material master (MARA, MAKT, MVKE, etc.) holds identity, units of measure, and sales-relevant data. Every material has a material number (`MATNR`).
- **SAP PLM (Product Lifecycle Management)** — holds engineering master data, CAD revisions, and change-management workflows.
- **SAP MDG (Master Data Governance)** — the enterprise governance layer on top; enforces data quality, workflows, and approval gates.
- **Classification System (CA-CL)** — characteristics (CABN) and classes (KLAH) assigned to materials. This is where luminaire-specific attributes (`BEAM_ANGLE`, `CCT_K`, `INPUT_POWER_W`) live. SAP's classification is the closest analog to a PIM attribute system.
- **Document Info Records (DMS)** — cutsheet PDFs, IES files, and LDT files live as DMS records linked to materials.

Older SAP ECC landscapes use **IDocs** (XML-ish envelopes) for integration. Modern S/4HANA exposes **OData** (REST/JSON) for every master-data entity. The ULC emitter can work with either; OData is the preferred integration path for new work.

## Mapping table

### Identity (MM material master)

| SAP field (OData) | ULC path |
|---|---|
| Parent configurable material MATNR (or PLM product-model identifier) | `product_family.catalog_model`, `product_family.family_id` (slugified) |
| Variant configuration MATNR | `configuration.catalog_number` |
| `MaterialDescription` (MAKT-MAKTX for chosen language) | `configuration.scenario_label` |
| `Brand` (manufacturer-specific Z-field) | `product_family.manufacturer.slug`, `product_family.manufacturer.display_name` |
| `ProductHierarchy` | `product_family.catalog_line` |
| Material group (MARA-MATKL) | Used as input to category mapping, not a direct ULC field |
| Derived full slug `<manufacturer>-<matnr>-<scenario>` | `record_id` |
| Derived scenario-local slug `<family>-<cct>-<distribution>` | `configuration.photometric_scenario_id` |

The "variant configuration MATNR → `configuration.catalog_number`" mapping is the Pattern A default (one record per variant). If a configurable material publishes one ULC record covering many variant configurations via a multiplier table (Pattern B) or per-foot linear scaling (Pattern D), `configuration.catalog_number` carries only the tested-baseline variant's MATNR, and the covered variant range is declared in `applicability.covered_axes.<axis>` with a per-axis derivation rule. See `docs/authoring-patterns.md` for worked examples.

### Category (classification system)

SAP's classification system holds luminaire attributes as **characteristics** (CABN) on **classes** (KLAH). The mapping from a manufacturer's SAP class hierarchy to ULC's `primary_category` enum is manufacturer-specific and lives in the integration configuration:

| SAP class (typical naming) | ULC `primary_category` |
|---|---|
| `LUM_DOWNLIGHT_REC` | `downlight` |
| `LUM_LINEAR_PEND` | `linear` |
| `LUM_WALLPACK_EXT` | `bulkhead_wall_pack` |
| `LUM_HIGHBAY_IND` | `high_bay` |
| `LUM_BOLLARD_OUT` | `bollard` |
| `LUM_SCONCE_INT` | `sconce` |

### Characteristics (classification attributes)

SAP characteristics carry typed values with unit of measure. The emitter reads characteristics assigned to a material and maps them:

| SAP characteristic | ULC path | Notes |
|---|---|---|
| `OVERALL_DIAMETER_MM` (NUM, MM) | `product_family.physical_dimensions.overall_diameter.{mm, in}` | Convert at emit |
| `WEIGHT_KG` (NUM, KG) | `product_family.physical_dimensions.luminaire_mass.{kg, lb}` | Convert at emit |
| `INPUT_POWER_W` (NUM, W) | `electrical.input_power_w.value` |
| `BEAM_ANGLE_DEG` (NUM, DEG) | `photometry.beam_angle_deg.value` |
| `TOTAL_FLUX_LM` (NUM, LM) | `photometry.total_luminous_flux_lm.value` |
| `CCT_K` (CHAR enum) | `colorimetry.nominal_cct_k`, `configuration.tested_conditions.nominal_cct_at_test` |
| `CRI_RA` (NUM) | `colorimetry.cri_ra.value` |
| `DRIVER_PROTOCOL` (CHAR enum) | `electrical.driver_protocol` |

Each enum characteristic maintains its own value-correspondence table in the integration config.

### Units of measure

SAP stores units of measure as codes in the T006 unit table. ULC expects canonical ISO-style units. The emitter maintains a mapping:

| SAP T006 code | ULC unit |
|---|---|
| `MM` | millimeter, emit as `mm` |
| `M` | meter, convert to mm (ULC's `DualUnitLength` is `mm` + `in` only; there is no `m` slot, so always normalize) |
| `CM` | centimeter, convert to mm |
| `KG` | kilogram, emit as `kg` |
| `G` | gram, convert to kg |
| `W` | watt, emit as `W` |
| `LM` | lumen, emit as `lm` |
| `DEG` | degree, emit as `deg` |

Always convert to the ULC canonical unit at emit time. The companion (in, lb, ft, etc.) computes from the canonical value.

### Document Info Records (DMS)

Cutsheet PDFs, IES, LDT, and lab-test reports live as DMS records. The emitter queries DMS records linked to the material (DRAW header, DRAD originals):

| SAP DMS doc type | ULC `source_files[].file_type` |
|---|---|
| `CUTSHEET` | `datasheet_pdf` |
| `PHOTO_IES` | `ies` |
| `PHOTO_LDT` | `ldt` |
| `INSTALL_PDF` | `installation_instructions_pdf` |
| `LAB_REPORT` | (metadata only, referenced via `attestations[].source_document_ref`) |

The emitter streams each DMS original, computes SHA-256, and fills `source_files[].reference`.

### Attestations

Listings, certifications, and test-method conformance typically live as characteristics (Boolean or CHAR) plus an optional DMS reference to a scanned certificate. Map each to a ULC `attestations[]` entry:

```json
{
  "attestation_id": "ul_1598_<material>",
  "program": "ul_1598",
  "status": "listed",
  "value_type": "rated",
  "verification": { "type": "unconditional" },
  "source_document_ref": {
    "filename": "...scanned certificate.pdf",
    "sha256": "..."
  }
}
```

### Variants and SKU expansion

SAP's configurable materials (class-type 300) describe a material with variant characteristics (voltage, CCT, finish). Each variant configuration (`VBAP` → variant) produces one ULC scenario record. The `applicability` block carries the variant axes:

- `applicability.fixed_axes` — per-scenario fixed characteristic values.
- `applicability.covered_axes.<axis_name>` — axes where a single scenario's photometry applies across multiple variants. Each `CoveredAxis` carries a `values[]` list, a `rationale` string, and optionally a `derivation` rule (multiplier table, per-foot linear scaling, voltage-independence rationale) that describes how non-baseline values are computed from the tested baseline.

## Gotchas

1. **IDoc vs OData.** Legacy ECC landscapes without S/4HANA expose data via IDocs (MATMAS, CLFMAS, DOCMAS). The mapping is conceptually identical to OData; the integration engineer just handles XML instead of JSON. Modern landscapes should prefer OData.
2. **Language keys.** SAP descriptions are per-language (SPRAS). The emitter chooses a canonical language key (typically `EN` or the manufacturer's primary market language). Record this choice in the integration config.
3. **Change-document tracking.** Materials with active change documents (ECR/ECO in progress) should be skipped or emitted at `conformance_level: "core"` — the data is not authoritative yet.
4. **Classification sparseness.** Not every luminaire in the material master will have every expected characteristic populated. The emitter decides per-field whether absence is fatal (skip record) or graceful (emit at `core`, mark missing in provenance).
5. **Unit rounding errors.** SAP often stores dimensions at coarse precision (integers of mm). When converting mm → in, preserve decimal precision in the computed Imperial value.
6. **Localized currency and format**. SAP stores numbers with locale formatting rules (decimal comma vs period). The OData / IDoc layer typically normalizes, but the emitter should defensively parse.

## Emit flow

```
1. Triggered by change-event publisher (SAP Event Mesh) or scheduled delta query
2. Emitter queries OData:
   - /API_PRODUCT_SRV/Product?$filter=MaterialType eq 'FERT' and LastChangeDate gt LAST_RUN
   - /API_CLFN_CHARACTERISTIC_SRV/Characteristic?$filter=Product eq 'X'
   - /API_DMS_DOC_SRV/Document?$filter=Product eq 'X'
3. For each material:
   - Map classification class → primary_category
   - Pull characteristics, apply unit-code table
   - For each variant configuration:
     - Assemble scenario record
     - Stream DMS originals, compute SHA-256
     - Walk attestation-referenced DMS (lab reports)
     - ulc build-index + validate
     - Publish on success
4. Push failures to the integration monitoring channel
```

## Example transform skeleton (ABAP / Python hybrid)

In practice, ABAP handles data extraction (CDS views or SAP CAP/RAP) and a Python microservice handles JSON assembly and the ULC CLI shell-out:

```python
# Illustrative pseudocode for the Python side.
def emit_ulc_from_sap(material, variant, characteristics, dms_docs):
    primary_category = CLASS_TO_ULC_CATEGORY[material['class']]
    record = {
        "ulc_version": "0.3.0",
        "record_id": slug(f"{material['brand_slug']}-{material['matnr']}-{variant['scenario_slug']}"),
        "record_status": "active",
        "conformance_level": "core",
        "product_family": build_family(material, primary_category),
        "configuration": build_configuration(variant, characteristics),
        "electrical": map_electrical(characteristics),
        "photometry": map_photometry(characteristics),
        "colorimetry": map_colorimetry(characteristics),
        "source_files": build_source_files_from_dms(dms_docs),
    }
    # Both CLIs are file-based. Write the record to a temp file, run
    # build-index in place, then validate against the same path.
    with tempfile.NamedTemporaryFile("w", suffix=".ulc.json", delete=False) as f:
        json.dump(record, f)
        tmp_path = f.name
    subprocess.run(["ulc", "build-index", tmp_path], check=True)
    validation = subprocess.run(["ulc", "validate", tmp_path], capture_output=True)
    return tmp_path if validation.returncode == 0 else None
```

## See also

- [`README.md`](README.md) — shared PIM-to-ULC translation concerns.
- SAP S/4HANA OData API Business Hub documentation (`API_PRODUCT_SRV`, `API_CLFN_CHARACTERISTIC_SRV`, `API_DMS_DOC_SRV`) for actual endpoint shapes.
- `templates/` — ULC skeletons for structure reference.
