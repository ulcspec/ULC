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
`lumens` on both zonal sheets, `value` on `spectral_power_distribution`, and
`claimed_hours` and `failure_percent` on `additional_rated_claims`.
The spectrum companions describe its whole block, with one resolved provenance
for all samples. The authored rows on `declared_by_length` retain
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

`tm_21_projection_hours` is an extrapolated projection and requires `rated`;
both `measured` and `nominal` are refused. Actual test quantities such as
`test_hours` may use a measured override when their maintenance evidence
resolves.
The derived methods `scaled`, `optical_simulation`, and `extended_photometry`
also require `value_type=rated` and cannot be paired with `measured` or
`nominal`.

### Measured-colour records columns

Release 1.14.0 adds these optional `records` columns for measured colour data:

| Header | Record destination | Shape and unit | Defaults or allowed values |
|---|---|---|---|
| `measured_cct_k` | `colorimetry.measured_cct_k` | Provenanced number, `K` | `measured`, `test_report`, `transcribed` |
| `chromaticity_x` | `colorimetry.chromaticity_x` | Provenanced number, no unit | `measured`, `test_report`, `transcribed` |
| `chromaticity_y` | `colorimetry.chromaticity_y` | Provenanced number, no unit | `measured`, `test_report`, `transcribed` |
| `cri_r9` | `colorimetry.cri_r9` | Provenanced number, no unit | `measured`, `test_report`, `transcribed` |
| `tm_30_rf` | `colorimetry.tm_30.rf` | Provenanced number, no unit | `measured`, `test_report`, `transcribed` |
| `tm_30_rg` | `colorimetry.tm_30.rg` | Provenanced number, no unit | `measured`, `test_report`, `transcribed` |
| `tm_30_reference_illuminant_type` | `colorimetry.tm_30.reference_illuminant_type` | Plain enum | `planckian`, `blended_planckian_daylight`, or `cie_d_series` |
| `tm_30_pvf_code` | `colorimetry.tm_30.pvf_code` | Plain string | `P1` to `P3`, `V1` to `V3`, or `F1` to `F3` |

Each of the six provenanced-number columns accepts all six standard companions:
`__value_type`, `__prov_source`, `__prov_method`, `__extension_method`,
`__base_attestation_ref`, and `__attestation_ref`. The two plain columns accept
no companions. A measured value requires eligible LM-79 evidence. When more
than one eligible LM-79 attestation exists, author the value's explicit
`__attestation_ref`. Report evidence must actually support the authored value;
the converter does not turn unrelated evidence into a measurement.

TM-30 Rf and Rg are independent values. Each has its own value type,
provenance, and evidence reference, so either can be authored without the other
and their companions need not match. The reference illuminant and PVF code may
also be authored independently. Blank cells are omitted, and four blank TM-30
cells create no `tm_30` object. The converter never derives Rf, Rg, reference
illuminant, or PVF from CCT or another colour metric.

Use an `ulc` converter at release 1.14.0 or newer for these columns. Older
converters ignore the two unknown plain headers and reject the new companion
headers, even when those companion cells are blank.

File-reference revision metadata uses a separate companion family:
`X__revision_label` and `X__revision_date` are legal for `cutsheet_file` and
`warranty_conditions_file` on `records`, `filename` on `source_files`, and
`source_document_file` on `attestations`. A double-underscore header outside
these declared base and suffix combinations is an error even when every cell
below it is blank. A plain unrecognized header remains ignored.

The `domestic_content` payload on `attestations` and `shared_attestations`
accepts only `domestic_content__prov_source` and
`domestic_content__prov_method`. It has no `value_type` or attestation reference
companions because it inherits the containing attestation's evidence.

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
| `customization_openness` | Manufacturer-stated axes open to requests beyond the published order-code menu. | When the family publishes or states an opening |
| `covered_axes` | One row per (axis, covered value) with rationale and derivation. | Patterns B and D |
| `cct_multipliers` | The CCT lumen-multiplier table. | Pattern B |
| `declared_by_length` | A verbatim per-length table with fixed provenance defaults in this release. Omit it to have the per-foot rates generate it. | Pattern D |
| `excluded_combinations` | SKUs orderable elsewhere but out of scope for this record. | Patterns B and D |
| `alpha_opic` | Alpha-opic / melanopic per-photoreceptor efficacy with per-value provenance companions. Every filled `efficacy` requires an authored `efficacy_unit` of `W/lm` or `mW/lm`. | Full enrichment |
| `spectral_power_distribution` | One row per wavelength sample for a uniformly spaced spectrum, with provenance for the whole spectrum. | Colorimetry enrichment |
| `flicker_metrics` | TLA metrics (SVM, Pst_LM, percent flicker) with per-value provenance companions and a metric-specific unit rule. | Full enrichment |
| `lumen_maintenance_package` | LM-80 / TM-21 method-backed projection with companions on its three numeric values. | Full enrichment |
| `additional_rated_claims` | Further lumen-maintenance thresholds beside the records-sheet headline, with companions on hours and failure percent. | When several claims are published |
| `zonal_lumens` | Angle-band zonal lumens with per-value provenance companions. | Full enrichment |
| `lcs_zonal_lumens` | TM-15 LCS secondary solid-angle zones with per-value provenance companions. | Outdoor, full enrichment |
| `ingredient_list` | Declare / Living Building Challenge material roster. | Full enrichment |
| `cie97_lmf` | CIE-97 LMF grid (one row per interval and cleanliness; a full cutsheet has 12). | Full enrichment |
| `cie97_llmf` | CIE-97 LLMF by operating hours. | Full enrichment |

When a cutsheet prints several lumen-maintenance claims, put the first claim it
gives, or the one you judge most representative, in `records` using
`lm_claim_type`, `lm_claimed_hours`, and `lm_claim_basis`. Put the other claims
on `additional_rated_claims`, one row per claim. Each row needs `claim_type`
and `claimed_hours`; `basis` is required by the schema. Claims without a
records-sheet headline are refused.
The additional-claims sheet authors rated hours; use `lumen_maintenance_package`
for an L50 threshold crossed experimentally in an extended LM-80 test.

## Openness sheet

Use `customization_openness` for one row per open axis, repeated identically
for every record in the family. The `statement` names the request opportunity
without enumerating values, prices, lead times, or quantities. If the source
prints such a list, cite the document and elide the list from the statement.
Set `published_in_ref` to a filename in the vocabulary of the record's
`source_files` entries and the family cutsheet. Set `contact_reference` to a
public role or department, never a named person. At least one of these two
references is required for each row.

## Spectrum sheet

Use `spectral_power_distribution` for a sampled spectrum. Each row needs
`record_id`, numeric `wavelength_nm`, and numeric `value`. Supply at least two
rows per record in strictly ascending order at one uniform wavelength step. The
converter derives the start and step from those rows and refuses duplicate,
descending, or off-grid wavelengths. For a chart or exchange file with irregular
samples, resample the curve to one step before authoring it.

Set `unit` to `mW/nm`, `W/nm`, or `relative`, and set `source_kind` to
`laboratory_table`, `digitized_chart`, or `exchange_file`. For a luminaire-level
measurement through its optics, set `measured_through_optics` to `TRUE`. These
cells and `conflict_notes` may be filled on the first row or repeated; two
different non-blank values for the same record are refused. The six `value__*`
companions follow the same rule and describe the whole spectrum. The default
is measured, from a test report, transcribed, and linked to the record's single
LM-79 attestation. For a digitized cutsheet chart, set
`value__value_type=rated` and `value__prov_source=datasheet_pdf`. For a TM-27
exchange file, add it to `source_files`, set `source_kind=exchange_file`, and
set `value__prov_source=tm27`.
If you set `measured_through_optics=FALSE`, set `value__value_type=rated`;
a measured package spectrum cannot use a luminaire LM-79 attestation.

## Attestation evidence columns

On `attestations` and `shared_attestations`, use `valid_until` (an ISO date in
`YYYY-MM-DD` form), `listing_number`, and `test_laboratory` for the three
attestation evidence members. The `verification_contact_reference` and
`verification_notes` columns become members of the attestation's verification
block. Its `type` defaults to `unconditional` when `verification_type` is blank,
even if either of those two columns is filled. For a case-by-case claim, put
`verification_contact_reference` beside
`verification_type=requires_manufacturer_confirmation`.

## Domestic-content columns

On `attestations` or `shared_attestations`, fill
`domestic_content_us_cost_share_percent`,
`domestic_content_foreign_cost_share_percent`,
`domestic_content_threshold_percent`,
`domestic_content_threshold_effective_date`, and `domestic_content_basis` to
record the cost shares and the threshold behind a domestic-content claim.
The foreign share and effective date are optional. When any domestic-content
cell is filled, the schema requires the US share, threshold, and basis, and
checks percentages against 0 to 100. Set `basis` to `manufacturing_cost`,
`component_cost`, or `other`. The payload defaults to provenance source
`manufacturer_direct` and method `transcribed`. For a delivered manufacturer
spreadsheet, set `domestic_content__prov_source=manufacturer_data_export`;
`domestic_content__prov_method` can override the method independently. The
payload belongs to its attestation and carries no separate `value_type`.
Derived methods (`scaled`, `optical_simulation`, and `extended_photometry`) are
refused because this payload cannot name a base attestation.

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

## Market columns

Use `markets` to declare the sales markets into which the product family is
sold. For more than one market, separate tokens with semicolons, for example
`north_america;united_kingdom`. The allowed tokens are `north_america`,
`united_kingdom`, `european_union`, `japan`, `australia_new_zealand`, and
`other`. This field controls market-specific evidence applicability and is
distinct from `technical_region`, which describes the fixture's electrical
configuration.

## Voltage columns

Use `input_voltage_v` for one numeric input-voltage value. It is a provenanced
number and accepts the same provenance companion columns as other records-sheet
measurements. Use `input_voltage_class` for the supply class or published range
the product supports, and `input_voltage_at_test` for the supply class or
published range used during the test. Those two columns are open strings: the
values shown in the schema are examples, not a closed vocabulary.

## Bound columns

Use `power_factor` with `power_factor_bound_operator` for a declared power
factor bound, such as `0.9` with `gt` for above 0.9. Use `thd_percent` with
`thd_percent_bound_operator` for a THD bound, such as `20` with `lt` for below
20 percent. Use `max_surface_luminance_cd_per_m2` with
`max_surface_luminance_bound_operator` for a luminous-surface limit, such as
`1600` with `lt` for below 1600 cd/m2. The operator is optional when the
number is a point estimate; an operator without its number fails schema
validation. These three operators accept `eq`, `lte`, `lt`, `gte`, and `gt`.
The existing `ugr_4h_8h_bound_operator` still accepts only `eq`, `lte`, and
`lt`.

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
