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

`--out` is where the finished `<record_id>.ulc` files are written. `--assets` is the
directory your referenced files (cutsheet PDF, warranty conditions PDF, IES,
attestation documents) live in; it defaults to the workbook directory.

## The join key

Every sheet is keyed by `record_id`. It is unique on `records` (one row per ULC
record) and a repeatable foreign key on every other sheet. To attach four CCT
rows or twelve CIE-97 rows to a record, repeat its `record_id` on each row.

## What you never author

Three things are computed, never typed into the workbook:

- The entire `index` block (a deterministic projection, including the graded
  `conformance_level`).
- Every computed companion leaf on a dual-unit field. Each dual-unit field
  on the records sheet has two entry columns, an SI column (`*_mm`, `*_kg`,
  `*_kg_per_m`) and an Imperial companion column (`*_in`, `*_lb`,
  `*_lb_per_ft`); the `declared_by_length` sheet's `length_mm` stays
  SI-only. Author exactly one side per field and leave the other blank; the
  converter writes both leaves, computing the companion per the published
  conversion policy (`docs/conversion-policy.md`). The authored value lands
  in the record verbatim; only the computed side is rounded. Authoring both
  sides of one field on one row is an error. Imperial entry columns are
  recognized from release 1.5.0 on, and an older `ulc` binary ignores
  unrecognized columns silently, so check `ulc version` if an
  Imperial-authored value does not appear in the output. (The schema also
  defines dual-unit temperature and area families, but no records-sheet
  column authors them today, and not every field named `*_c` is dual-unit:
  the schema types the lumen-maintenance-package `test_temperature_c` as a
  scalar `ProvenancedNumber`, which is inconsistent with the dual-unit
  temperature fields, so the converter does not author it pending a schema
  reconciliation.)
- Every `sha256`. You name the file in a path column; the converter hashes it.

## Provenance defaults and overrides

Measured and rated values carry a `value_type` and a `provenance {source,
method}`. The converter fills per-column defaults. Every provenanced value on
the `records` sheet supports the six companion columns `X__value_type`,
`X__prov_source`, `X__prov_method`, `X__extension_method`,
`X__base_attestation_ref`, and `X__attestation_ref`. The same companions are
supported by `melanopic_der` and `efficacy` on `alpha_opic`, `value` on
`flicker_metrics`, the three numeric values on `lumen_maintenance_package`, and
`lumens` on both zonal sheets. The authored rows on `declared_by_length` retain
their fixed provenance defaults and accept no companions in this release.

Measured values auto-link only within their evidence family: records-sheet
photometry and zonal lumens use LM-79; alpha-opic values use RP-46; flicker
values use LM-90-20, IEEE 1789-2015, or NEMA 77-2017; and package-maintenance
values use LM-80 or TM-21. Supply `X__attestation_ref` when a family has more
than one candidate; it must name exactly one attestation in that family, and a
confirmation-required attestation cannot anchor measured evidence. For derived photometry, `X__extension_method` names the
scaling rule and `X__base_attestation_ref` names the base measurement. Leave a
legal companion blank to take the default.

The family restriction above applies to the declared supplementary fields and
to records-sheet photometry. The records-sheet `lm_claimed_hours` field declares
the maintenance family, so its explicit `attestation_ref` can name only LM-80
or TM-21 evidence, and the manufacturer-rated claim refuses a measured
override. Every records-sheet `base_attestation_ref` follows its field's family.

`tm_21_projection_hours` is an extrapolated projection and cannot be marked
`measured`; use `rated`. Actual test quantities such as `test_hours` may use a
measured override when their maintenance evidence resolves.
The derived methods `scaled`, `optical_simulation`, and `extended_photometry`
also require `value_type=rated` and cannot be paired with `measured` or
`nominal`.

File-reference revision metadata uses a separate companion family:
`X__revision_label` and `X__revision_date` are legal for `cutsheet_file` and
`warranty_conditions_file` on `records`, `filename` on `source_files`, and
`source_document_file` on `attestations`. A double-underscore header outside
these declared base and suffix combinations is an error even when every cell
below it is blank. A plain unrecognized header remains ignored.

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
documented field you supply and the grade follows the data. Plain columns and
sheets it does not recognize are ignored, so check plain column names against
the templates if a value does not appear. An unrecognized double-underscore
companion header is refused. Imperial dual-unit columns are recognized from
release 1.5.0 on.

## The sheets

| Sheet | What it carries | When you need it |
|---|---|---|
| `records` | One row per record: identity, taxonomy, lifecycle (supersession pointer and discontinuation date), mechanical, warranty, electrical, photometry, colorimetry, the applicability header, and the sustainability scalars. | Always |
| `source_files` | IES / LDT / ULD / supplementary files. The cutsheet is injected automatically from `records.cutsheet_file`. | An IES row for measured photometry (the converter default); rated-only records rely on the auto-injected cutsheet |
| `attestations` | Per-record program attestations. The LM-79 row is the measurement anchor. | Measured photometry needs the LM-79 anchor even at core; standard and up otherwise as applicable |
| `shared_attestations` | Family-wide listings (UL, IEC, RoHS). | As applicable |
| `covered_axes` | One row per (axis, covered value) with rationale and derivation. | Patterns B and D |
| `cct_multipliers` | The CCT lumen-multiplier table. | Pattern B |
| `declared_by_length` | A verbatim per-length table with fixed provenance defaults in this release. Omit it to have the per-foot rates generate it. | Pattern D |
| `excluded_combinations` | SKUs orderable elsewhere but out of scope for this record. | Patterns B and D |
| `alpha_opic` | Alpha-opic / melanopic per-photoreceptor efficacy with per-value provenance companions. | Full enrichment |
| `flicker_metrics` | TLA metrics (SVM, Pst_LM, percent flicker) with per-value provenance companions and a metric-specific unit rule. | Full enrichment |
| `lumen_maintenance_package` | LM-80 / TM-21 method-backed projection with companions on its three numeric values. | Full enrichment |
| `zonal_lumens` | Angle-band zonal lumens with per-value provenance companions. | Full enrichment |
| `lcs_zonal_lumens` | TM-15 LCS secondary solid-angle zones with per-value provenance companions. | Outdoor, full enrichment |
| `ingredient_list` | Declare / Living Building Challenge material roster. | Full enrichment |
| `cie97_lmf` | CIE-97 LMF grid (one row per interval and cleanliness; a full cutsheet has 12). | Full enrichment |
| `cie97_llmf` | CIE-97 LLMF by operating hours. | Full enrichment |

The four authoring patterns are detected for you from which sheets carry rows: a
populated `catalog_number` with no applicability sheets is a single-SKU pin
(Pattern A or, with derived photometry provenance, C); a `cct_multipliers` table
is Pattern B; a `declared_by_length` sheet or a `per_foot_linear_scaling`
derivation on the length axis is Pattern D.

When you author the supersession columns (`superseded_by_record_id`,
`superseded_by_catalog_number`, `superseded_by_catalog_model`), also set
`record_status` to `superseded`: the converter's blank-cell default is
`active`.

Leave the `ulc_version` cell blank to stamp the converter's current released
specification version from its compiled `SpecVersion` constant. You may author
an older three-part version for an older consumer, but the converter does not
check the record against that older release: it embeds the current schema and
validates and builds the index against that schema alone. A version newer than
the converter's compiled specification version is refused. A delivery path must
also require records produced by its build to declare exactly the release of
the `ulc` engine it pins.

## Voltage columns

Use `input_voltage_v` for one numeric input-voltage value. It is a provenanced
number and accepts the same provenance companion columns as other records-sheet
measurements. Use `input_voltage_class` for the supply class or published range
the product supports, and `input_voltage_at_test` for the supply class or
published range used during the test. Those two columns are open strings: the
values shown in the schema are examples, not a closed vocabulary.

## Notes for `.xlsx` authors

The `.xlsx` reader is faithful to the cell text, not to Excel's display formatting,
so author with that in mind:

- **Format date and identifier columns as Text before typing.** In a spreadsheet
  application, format the date columns and identifier columns such as
  `catalog_number` as Text before entering any data. Spreadsheets re-type as you
  enter: an ISO date typed into a General cell is stored as its serial number (for
  example `46054`), and an identifier such as `1200.40` is stored as the number
  `1200.4`, with the trailing zero gone from the file itself. The reader passes the
  stored value through verbatim, and no tool can restore a character the file no
  longer contains. A leading apostrophe (`'2026-02-01`) forces text entry cell by
  cell; formatting the column as Text first does it wholesale.
- **Dates as ISO text.** Type dates as ISO strings (`2026-02-01`). ISO text reads
  identically from a CSV and an `.xlsx`, and schema validation rejects any date
  that is not `YYYY-MM-DD`, so a serial-number slip is caught at conversion instead
  of landing in a record.
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
- `examples/` for eight complete `.ulc` records across the four authoring patterns and the exit-sign/emergency product class.
- The filled fixtures under `tools/validator/internal/sheet/testdata/` for a
  working bundle per pattern.
