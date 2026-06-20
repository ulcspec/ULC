# ULC Spreadsheet Workbook Schema: Deterministic (NO-LLM) Converter Design

Design for `ulc from-sheet`: a deterministic converter that turns a manufacturer-authored
workbook into valid `.ulc` records, covering all four authoring patterns (A single-SKU,
B multiplier-table, C per-IES-with-provenance, D per-foot-linear). No LLM is involved; a
spreadsheet is structured data, so column to field mapping is mechanical.

## 0. Design premise

A manufacturer authors **one workbook** (a set of sheets / CSVs). Every sheet is keyed by
`record_id`. The converter:

1. reads sheets, joins on `record_id`;
2. assembles ULC deep blocks;
3. computes dual-unit companions (`in`/`lb`/`f`/`ft2`/`lb_per_ft`) from the SI side;
4. computes `sha256` of every referenced file;
5. fills per-column default provenance;
6. runs the existing index builder (stamps `index.*` + `conformance_level`) and the validator.

The workbook carries **only authored values**. Three things are NEVER columns: the entire
`index.*` block, every Imperial companion leaf, every `sha256`.

**Universal conventions used in every sheet:**

- `record_id` is the join key on every sheet. Unique on `records` (primary key); a repeatable
  foreign key on related sheets.
- **ProvenancedNumber columns** appear as a value column plus, where load-bearing, companion
  `*__value_type`, `*__prov_source`, `*__prov_method`, `*__attestation_ref` columns. Where a
  column has a sensible per-pattern default (see section 3.3) the provenance columns may be left
  blank and the converter fills them.
- **DualUnit columns** are authored as the **SI side only** (`*.mm`, `*.kg`, `*.c`, `*.m2`,
  `*.kg_per_m`). The header names the SI leaf explicitly (e.g. `overall_diameter_mm`). The
  converter writes both `mm` and the computed `in`.
- **Enum-list / string-list scalars** with a single value are authored inline on `records` as a
  delimited cell (`;`-joined). When more than one value needs per-value structure they move to a
  dedicated long sheet.
- Blank cell = field absent (converter omits the key). Empty is never the same as `0`.

## 1. Sheet list (summary)

| Sheet | Purpose | Key | Patterns |
|---|---|---|---|
| `records` | one row per ULC record; the master row | `record_id` (unique) | A B C D |
| `source_files` | IES/LDT/ULD/supplementary files (cutsheet auto-injected) | `record_id` | A B C D |
| `attestations` | per-record program attestations (LM-79 row is load-bearing) | `record_id` | A B C D |
| `shared_attestations` | family-wide listings (UL/IEC/RoHS) | `record_id` | A B C D |
| `covered_axes` | one row per (axis, covered value) + rationale + derivation | `(record_id,axis_key,value)` | B D |
| `cct_multipliers` | the Pattern B CCT lumen-multiplier table | `(record_id,axis_value)` | B |
| `declared_by_length` | verbatim per-length table (else generated) | `(record_id,length_mm)` | D |
| `excluded_combinations` | SKUs orderable elsewhere but not covered here | `(record_id,row#)` | B D |
| `compatible_accessories` | separately-ordered accessories (planned; declared here but not yet consumed by the converter) | `(record_id,accessory)` | A B C |
| `ingredient_list` | Declare / LBC material roster | `(record_id,material)` | B D |
| `cie97_lmf` / `cie97_llmf` | CIE-97 LMF grid + LLMF-by-hours | `(record_id,...)` | A (full) |
| `lumen_maintenance_package` | LM-80 / TM-21 method-backed rows | `(record_id,pkg)` | full |
| `alpha_opic` | per-photoreceptor efficacy (assembled into `alpha_opic_metrics`) | `(record_id,channel)` | full |
| `flicker_metrics` | TLA metrics (SVM, Pst_LM) | `(record_id,metric)` | full |
| `zonal_lumens` / `lcs_zonal_lumens` | angle-band + TM-15 LCS zones | `(record_id,zone)` | B / outdoor |

The full column list per sheet (header -> dotted ULC path -> type -> required-at-level) is the
implementation contract; see the per-sheet tables maintained alongside the converter code.

### `records` minimum (core-level Pattern A, smallest happy path)

The smallest valid workbook is `records` (1 row), plus a `source_files` IES row for the default
measured photometry path (a rated-only record relies on the auto-injected cutsheet `datasheet_pdf`
source and needs no IES row). Core grading needs only `total_luminous_flux_lm.value`,
`input_power_w.value`, and `primary_category`; schema structure adds the identity/cutsheet/scenario
fields:

```
record_id, ulc_version(=0.6.0 default), record_status(=active),
family_id, manufacturer_slug, manufacturer_display_name, catalog_model,
cutsheet_file        (-> sha256 + cutsheet/source_files dual-write),
primary_category     (CORE grade gate),
photometric_scenario_id,
catalog_number       (Pattern-A signal),
input_power_w        (CORE grade gate),
total_luminous_flux_lm (CORE grade gate)
```

Plus, for measured photometry, a `source_files` row `{record_id, file_type=ies, filename}`. The converter supplies the
`ulc_version` default, dual-unit companions, both `sha256` values, the cutsheet dual-write,
default provenance, and the whole `index`. Because the two photometric anchors default to
`value_type=measured`, the schema then wants an `attestation_ref`, satisfied by one `attestations`
row with an `lm_79*` program (or, to stay attestation-free, set `input_power_w__value_type=rated`
+ `total_luminous_flux_lm__value_type=rated`).

## 2. Pattern detection (deterministic, no LLM)

Classified per `record_id` purely on sheet/column presence:

| Signal | A | B | C | D |
|---|---|---|---|---|
| `records.catalog_number` populated | yes | no | yes | usually no |
| any `covered_axes` rows | no | yes | no | yes |
| any `cct_multipliers` rows (`manufacturer_lumen_multiplier`) | no | yes | no | no |
| any `declared_by_length` rows / `per_foot_linear_scaling` derivation | no | no | no | yes |
| headline PN `prov_method` in {`extended_photometry`,`optical_simulation`} + `base_attestation_ref` | no | no | yes | no |

Precedence (resolves the Pattern D `catalog_number` collision, open question 10): the
derivation/length/multiplier signals win over the `catalog_number` signal. Classify as B or D
whenever the covered-axes / length / multiplier sheets carry rows for the record, regardless of
whether `catalog_number` is also present. A vs C is not an applicability fork (both are
fixed-axes pins); the only difference is provenance defaults, driven by the per-column provenance
values the manufacturer supplies.

## 3. Dual-unit, SHA-256, provenance

### 3.1 Dual-unit (single SI input -> DualUnit)
Author the SI/authoritative leaf only; the converter computes the companion and writes both keys
plus the schema-required `value_type`:

| Authored suffix | SI leaf | Computed companion |
|---|---|---|
| `*_mm` | `.mm` | `.in` = mm/25.4 |
| `*_kg` | `.kg` | `.lb` = kg*2.2046226 |
| `*_c` | `.c` | `.f` = c*9/5+32 |
| `*_m2` | `.m2` | `.ft2` = m2*10.7639 |
| `*_kg_per_m` | `.kg_per_m` | `.lb_per_ft` = kg_per_m*0.671969 |

The dual-unit companion applies only to fields the schema types as a `DualUnit*`
object (lengths, masses, the `ambient_temperature` / `case_temperature`
temperatures, areas). NOTE: the schema currently types the lumen-maintenance
package `test_temperature_c` as a scalar `ProvenancedNumber` (unit `C`), not a
dual-unit temperature. That is a schema inconsistency; until it is reconciled at
the schema level, the converter does NOT author `test_temperature_c` (rather than
emit a value that conflicts with the dual-unit temperature policy).

### 3.2 SHA-256 + cutsheet dual-write
Any path-input column (`records.cutsheet_file`, `source_files.filename`,
`attestations.source_document_file`) is a local path the converter resolves, hashes (lowercase
hex `^[a-f0-9]{64}$`), and stamps into the matching `reference.sha256`. `records.cutsheet_file`
is dual-written to both `product_family.cutsheet` (the FileReference directly) AND a synthesized
`source_files[] {file_type: datasheet_pdf}` entry (de-duplicated on filename). The manufacturer
lists the cutsheet once.

### 3.3 Provenance (per-column defaults, overridable)
Every ProvenancedNumber/DualUnit needs `provenance {source, method}` + `value_type`. The converter
applies per-column defaults, overridable by optional `*__value_type` / `*__prov_source` /
`*__prov_method` / `*__attestation_ref` / `*__extension_method` / `*__base_attestation_ref`
columns. Load-bearing rule: any column whose effective `value_type=measured` MUST carry an
`attestation_ref`; the converter auto-links it to the record's single `lm_79*` attestation and
hard-errors if there are zero or more-than-one (the manufacturer then disambiguates explicitly).

## 4. Resolved implementer decisions (the design's 10 open questions)

1. **KV-map cells** (`fixed_axes`, attestation `required_constraints`, `excluded.axes`): JSON-in-cell, converter-validated.
2. **`covered_axes` rationale**: axis-level; error on two non-identical non-blank rationales for one `(record_id, axis_key)`.
3. **`declared_by_length` author-vs-generate**: authored sheet wins; WARN when a row diverges from `rate*length` by > 2%.
4. **`attestation_ref` with multiple LM-79 rows**: require the explicit `*__attestation_ref` column; hard-error rather than guess. Attestations are emitted as unconditional (the default), case-by-case (`verification_type=requires_manufacturer_confirmation`, which the converter forbids from also being `value_type=measured`), or family-wide (shared). Option-conditional attestation applicability is supported via the `required_order_code_options` (`;`-list) and `required_constraints_json` (JSON object) columns, emitted as `attestation.applicability`.
5. **Pattern C provenance**: supported via the per-column companion columns (`*__value_type`, `*__prov_source`, `*__prov_method`, `*__extension_method`, `*__base_attestation_ref`, `*__attestation_ref`). A `provenance_overrides` long sheet keyed by `(record_id, ulc_field_path)` was considered but is NOT implemented in v1.
6. **Enum validation**: enum cell values pass through as authored, and the schema validator (run after assembly) rejects unknown tokens. A converter-side enum-domain preload from `ulc.schema.json` with offending cell coordinates was considered but is NOT implemented in v1. Programs with no `AttestationProgram` enum (IP/IK/etc.) route to `extensions_json`, not `attestations`.
7. **List delimiter**: inline `;`-joined for short scalar lists; `finish_color_options` is authoritative for `shared_mechanical`; the B `finish` covered-axis is authored independently and not cross-populated.
8. **`luminaire_efficacy`**: authored via the `luminaire_efficacy_lm_per_w` column; the Pattern D per-foot path computes efficacy for generated length rows (`method=computed`). Blank-fill from `flux/power` and a >1 lm/W divergence WARN were considered but are NOT implemented in v1.
9. **IES autofill**: NOT in v1; measured/zonal values are authored. Phase-2 IES-autofill fills blanks only, never overrides authored cells.
10. **Pattern D `catalog_number` collision**: derivation/length/multiplier signals take precedence over the `catalog_number` signal (see section 2).

## 5. Implementation notes

- Subcommand `ulc from-sheet <input>` in the existing binary; calls `index.Build` + the validator
  internally (same binary, no subprocess).
- Input formats (both shipped, both stdlib-only and offline): a CSV bundle (a directory of
  `<sheet>.csv` files) OR a native `.xlsx` workbook (one tab per sheet, named the same). The
  `.xlsx` reader (`xlsx.go`) is written on `archive/zip` + `encoding/xml` with no third-party
  dependency vendored; it produces the same format-agnostic `Workbook` model the CSV reader does,
  so an `.xlsx` and an equivalent CSV bundle convert identically (locked by a parity test).
  `Convert` dispatches on the input shape (a directory is a bundle; a `.xlsx` file is a workbook).
- Level scope: all four patterns across every conformance tier. The converter never targets a
  level. It ingests every authored field (core, standard, AND full), assembles the record, and
  the index builder grades the achieved `conformance_level` from what is present. The full-level
  related sheets are implemented (`fulllevel.go`): `alpha_opic` -> alpha_opic_metrics,
  `flicker_metrics` -> flicker_measurements, `lumen_maintenance_package` (top-level array),
  `zonal_lumens` -> photometry.zonal_lumens, `lcs_zonal_lumens` -> outdoor_classification, the
  Declare roster `ingredient_list` -> sustainability_declaration (whose block scalars ride on the
  records sheet), and `cie97_lmf` / `cie97_llmf` -> lumen_maintenance_luminaire.cie_97_lmf_table.
  Each attaches only when its sheet carries rows. Under the redesigned rubric two of these feed
  full-tier rules (`zonal_lumens` gates full; a `lumen_maintenance_package` with a
  tm_21_projection_hours is the method-backed maintenance projection); the rest are non-gating
  enrichment. No converted record reaches full on these sheets alone, because the full tier also
  requires test-report depth (uncertainty, corrections, instrumentation, TM-30) the converter does
  not synthesize, and an analog or phase-cut fixture that does not author its dimming method and
  range caps at core (a digital-protocol fixture is exempt and reaches standard without them). A
  manufacturer who has the data adds those sheets and the record carries more depth; one
  who does not grades
  core/standard. Nothing the manufacturer supplies is dropped or capped.
- A published, fill-in workbook template ships at `templates/workbook/` (header-only CSVs for
  every sheet plus a README): the manufacturer-facing starting point.
