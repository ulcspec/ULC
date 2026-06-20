# ULC workbook template

This is the input template for `ulc from-sheet`, the deterministic converter that
turns a manufacturer-authored workbook into validated `.ulc` records. Fill in the
sheets you have data for, run the converter, and it produces schema-valid records
with the index, dual-unit companions, SHA-256 hashes, and provenance computed for
you. No LLM is involved: a spreadsheet is structured data, so every column maps
to a field mechanically.

## Two ways to hand it to the converter

The converter reads either shape, and the two are interchangeable:

- **A CSV bundle**: this directory of `<sheet>.csv` files. Fill them in place and
  run `ulc from-sheet path/to/workbook/`.
- **A single `.xlsx`**: one workbook with one tab per sheet, each tab named
  exactly as the CSV is here (`records`, `source_files`, `attestations`, ...).
  Run `ulc from-sheet path/to/workbook.xlsx`.

```
ulc from-sheet ./workbook        --out ./out --assets ./assets
ulc from-sheet ./workbook.xlsx   --out ./out --assets ./assets
```

`--out` is where the `<record_id>.ulc.json` files are written. `--assets` is the
directory your referenced files (cutsheet PDF, IES, attestation documents) live
in; it defaults to the workbook directory.

## The join key

Every sheet is keyed by `record_id`. It is unique on `records` (one row per ULC
record) and a repeatable foreign key on every other sheet. To attach four CCT
rows or twelve CIE-97 rows to a record, repeat its `record_id` on each row.

## What you never author

Three things are computed, never typed into the workbook:

- The entire `index` block (a deterministic projection, including the graded
  `conformance_level`).
- Every Imperial companion leaf (`in`, `lb`, `f`, `ft2`, `lb_per_ft`) on a
  dual-unit field. You author the SI side only, in the dual-unit `*_mm` / `*_kg`
  / `*_c` columns; the converter writes both. (Not every `*_c` column is
  dual-unit: the schema types the lumen-maintenance-package `test_temperature_c`
  as a scalar `ProvenancedNumber`, which is inconsistent with the dual-unit
  temperature fields, so the converter does not author it pending a schema
  reconciliation.)
- Every `sha256`. You name the file in a path column; the converter hashes it.

## Provenance defaults and overrides

Measured and rated values carry a `value_type` and a `provenance {source,
method}`. The converter fills sensible per-column defaults (a photometric anchor
defaults to measured and auto-links to your single LM-79 attestation; a datasheet
dimension defaults to rated). To override any provenanced column `X`, add the
companion columns `X__value_type`, `X__prov_source`, `X__prov_method`, and
`X__attestation_ref`. For derived photometry (Pattern C and the generated B/D
tables), `X__extension_method` names the scaling rule and `X__base_attestation_ref`
names the base measurement: any value whose method is `extended_photometry`,
`optical_simulation`, or `scaled` must carry a `base_attestation_ref`, which the
converter auto-links to your single LM-79 attestation when you do not set it
explicitly. Leave companion columns blank to take the default.

## The smallest valid workbook

`records` (one row), plus a `source_files` IES row for the default measured
photometry path (a rated-only record relies on the cutsheet `datasheet_pdf`
source the converter auto-injects, and needs no IES row). Even the smallest
record needs a few required `records` columns: the identity set `family_id`,
`manufacturer_slug`, `manufacturer_display_name`, `catalog_model`, and
`cutsheet_file` (the cutsheet is hashed and dual-written into `source_files`),
plus the core-grade trio `total_luminous_flux_lm`, `input_power_w`, and
`primary_category`. Because the photometric anchors default to `value_type:
measured` and auto-link to your LM-79 attestation, the smallest path also needs
one `attestations` row with an `lm_79*` program; for an attestation-free draft
instead, set `total_luminous_flux_lm__value_type=rated` and
`input_power_w__value_type=rated`. Everything beyond that climbs the record
toward standard and full. Nothing you add is capped: the converter ingests every
documented field you supply and the grade follows the data. (Columns and sheets
it does not recognize are ignored, not an error, so a typoed column name is
silently skipped: check the column names against the templates if a value you
expected does not appear.)

## The sheets

| Sheet | What it carries | When you need it |
|---|---|---|
| `records` | One row per record: identity, taxonomy, mechanical, electrical, photometry, colorimetry, the applicability header, and the sustainability scalars. | Always |
| `source_files` | IES / LDT / ULD / supplementary files. The cutsheet is injected automatically from `records.cutsheet_file`. | An IES row for measured photometry (the converter default); rated-only records rely on the auto-injected cutsheet |
| `attestations` | Per-record program attestations. The LM-79 row is the measurement anchor. | Measured photometry needs the LM-79 anchor even at core; standard and up otherwise as applicable |
| `shared_attestations` | Family-wide listings (UL, IEC, RoHS). | As applicable |
| `covered_axes` | One row per (axis, covered value) with rationale and derivation. | Patterns B and D |
| `cct_multipliers` | The CCT lumen-multiplier table. | Pattern B |
| `declared_by_length` | A verbatim per-length table. Omit it to have the per-foot rates generate it. | Pattern D |
| `excluded_combinations` | SKUs orderable elsewhere but out of scope for this record. | Patterns B and D |
| `alpha_opic` | Alpha-opic / melanopic per-photoreceptor efficacy. | Full enrichment |
| `flicker_metrics` | TLA metrics (SVM, Pst_LM, percent flicker). | Full enrichment |
| `lumen_maintenance_package` | LM-80 / TM-21 method-backed projection. | Full enrichment |
| `zonal_lumens` | Angle-band zonal lumens. | Full enrichment |
| `lcs_zonal_lumens` | TM-15 LCS secondary solid-angle zones. | Outdoor, full enrichment |
| `ingredient_list` | Declare / Living Building Challenge material roster. | Full enrichment |
| `cie97_lmf` | CIE-97 LMF grid (one row per interval and cleanliness; a full cutsheet has 12). | Full enrichment |
| `cie97_llmf` | CIE-97 LLMF by operating hours. | Full enrichment |

The four authoring patterns are detected for you from which sheets carry rows: a
populated `catalog_number` with no applicability sheets is a single-SKU pin
(Pattern A or, with derived photometry provenance, C); a `cct_multipliers` table
is Pattern B; a `declared_by_length` sheet or a `per_foot_linear_scaling`
derivation on the length axis is Pattern D.

## Notes for `.xlsx` authors

The `.xlsx` reader is faithful to the cell text, not to Excel's display formatting,
so author with that in mind:

- **Dates as text.** Type dates as ISO strings (`2026-02-01`), not as Excel date
  cells. An Excel date cell stores a serial number (for example `46054`), and the
  reader passes the stored value through verbatim. ISO text reads identically from
  a CSV and an `.xlsx`.
- **Plain numbers.** Author numerics as plain numbers without cell-level rounding.
  Excel stores the full precision of a value, so a displayed `0.33` that is really
  `0.333333` is read as `0.333333`.
- **Zonal provenance.** Zonal lumens (`zonal_lumens`, `lcs_zonal_lumens`) default
  to `source = ies` because they are normally extracted from the IES file. When a
  band is instead a reconstructed sum of LCS components rather than a verbatim IES
  field, set `lumens__prov_source = article_text` and add a `conflict_notes` cell
  to record how the band was derived.

## See also

- `tools/validator/internal/sheet/DESIGN.md` for the full column-to-field
  contract and the resolved implementer decisions.
- `examples/` for five complete `.ulc` records across the patterns.
- The filled fixtures under `tools/validator/internal/sheet/testdata/` for a
  working bundle per pattern.
