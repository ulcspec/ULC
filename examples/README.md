# Examples

This directory contains canonical reference ULC records covering the four manufacturer authoring patterns and the exit-sign/emergency product class, drafted from real manufacturer spec sheets and IES files. Each record is a single `.ulc` file that validates against `schema/ulc.schema.json` and round-trips cleanly through the reference `ulc` CLI.

Source files (the original PDF datasheets, IES photometric files, and related documents) are NOT committed to this repository. The ULC record references them by filename, optional URL, and SHA-256 content hash. Any consumer who obtains a source file through the manufacturer's own distribution channel can verify it matches the record by comparing hashes.

## Records in this directory

| File | Pattern | What it exercises |
|---|---|---|
| `erco-quintessence-30416-023.ulc` | A: single SKU per cutsheet | Narrow applicability, classic indoor recessed downlight, full measured photometric baseline from LM-79 test, TM-21 lumen-maintenance time series, CIE 97 LMF table, alpha-opic metrics, flicker bounds via the `lte` comparison operator pattern |
| `vode-nexa-suspended-807-so-3500k-90cri-hl-black-48in.ulc` | D: per-foot linear scaling with conditional attestations | Per-foot lm/ft and W/ft scaling across length variants, option-conditional attestation (Chicago Plenum requires the CPP order code with remote power), case-by-case attestation pattern (BAA and BABA require per-project manufacturer confirmation), full Declare sustainability block with 21-material ingredient list |
| `selux-aya-pole-sr-ho-3000k.ulc` | B: per-photometric-scenario with applicability | CCT multiplier table applied to a 3000 K tested baseline (2200 K × 0.86 through 5000 K × 1.07), wide applicability covering thousands of orderable SKUs from one photometric scenario, outdoor classification block with BUG rating and Type V distribution, LCS zonal lumens breakdown, DarkSky compliance, Declare block with LBC Temp Exception for a Small Electrical Components entry |
| `lumenpulse-lumenfacade-loi-12-rgb-30x60-ts0.ulc` | C: per-IES with provenance classes | The `extended_photometry` provenance class: every photometric value carries `value_type: rated` and points back to a base LM-79 attestation (Spectralux test ID S1503051-R1, 2015-03-05) via `base_attestation_ref`, preserving the 1:1 IES-to-record mapping specifiers expect while distinguishing scaled derivatives from direct measurements |
| `lumenpulse-lumenfacade-loi-12-rgbw30k-10x60-ts2-5.ulc` | C: per-IES with provenance classes | The RGBW counterpart to the RGB facade record: a four-channel fixture whose white channel carries a 3000 K nominal CCT, exercising the white-point split, the CCT is graded while CRI, SDCM, and TM-30 are waived for the color-mixing architecture |
| `cooper-sure-lites-lpx7sd.ulc` | Exit-sign class: internally illuminated, self-powered | The sign profile authored from a spec sheet (no IES): illumination mode, legend color, a UL 924 listing, and an integral-battery `emergency` block with the standard battery trio (NiCd, 90-minute, self-diagnostic). Held at core by two real disclosure gaps the LPX sheet does not publish (letter height and self-powered input wattage) |
| `cooper-sure-lites-es61src.ulc` | Exit-sign class: internally illuminated, AC only | An edge-lit AC-only sign at standard: 6-inch letters, single face, no chevron, and 3.8 W published input power. The battery trio correctly skips because `power_source` is `ac_only` (the hasIntegralBattery negative in a real record) |
| `cooper-atlite-auxswhsd.ulc` | Exit-sign class: internally illuminated, self-powered | A self-powered NYC-compliant edge-lit sign at standard with 8-inch letters and the battery trio met; its `ul_924` attestation is backed by two attached UL Certificates of Compliance, carried as `compliance_documents` source files |

## Conformance grades and roadmap

Each record is graded by the reference validator, which computes a conformance grade from the fields actually present and emits a per-grade roadmap to full: the grades the record already satisfies and, for each grade not yet reached, only that grade's own remaining fields. The grade is stamped into `index.conformance_level` and is never hand-declared. The table below is the machine output of `ulc validate --verbose <record>`; every gap names the field, the source document it comes from, and the governing standard, grouped per grade to full. Beyond completeness, every record also carries the second grading axis, Product Achievements (`index.achievements`), computed from its attestations and reported alongside the grade; see [the two grading axes in docs/methodology.md](../docs/methodology.md#the-two-grading-axes-completeness-and-achievements).

| File | Grade | Remaining grades to full |
|---|---|---|
| `erco-quintessence-30416-023.ulc` | standard | **full** needs test-report depth: `zonal_lumens` (IES, LM-79), `corrections_applied` (test report, LM-79), `uncertainty` (test report, LM-79 / GUM), `tm_30.rf` and `tm_30.rf_h_per_bin` (test report, TM-30), and instrumentation depth (test report, LM-79 / LM-75) |
| `selux-aya-pole-sr-ho-3000k.ulc` | standard | **full** needs `corrections_applied`, `uncertainty` (test report, LM-79 / GUM), method-backed lumen maintenance (test report, LM-80 / TM-21 / TM-28), and `tm_30.rf` and `tm_30.rf_h_per_bin` (test report, TM-30) |
| `lumenpulse-lumenfacade-loi-12-rgb-30x60-ts0.ulc` | standard | **full** needs `zonal_lumens` (IES, LM-79), `corrections_applied`, `uncertainty` (test report, LM-79 / GUM), and method-backed lumen maintenance (test report, LM-80 / TM-21 / TM-28) |
| `lumenpulse-lumenfacade-loi-12-rgbw30k-10x60-ts2-5.ulc` | standard | **full** needs `zonal_lumens` (IES, LM-79), `corrections_applied`, `uncertainty` (test report, LM-79 / GUM), and method-backed lumen maintenance (test report, LM-80 / TM-21 / TM-28) |
| `vode-nexa-suspended-807-so-3500k-90cri-hl-black-48in.ulc` | core | **standard** needs `sdcm_step`, the MacAdam ellipse step (datasheet, ANSI C78.377); the Vode cutsheet publishes no MacAdam step. **full** additionally needs `zonal_lumens` (IES, LM-79), `corrections_applied`, `uncertainty` (test report, LM-79 / GUM), method-backed lumen maintenance (test report, LM-80 / TM-21 / TM-28), and `tm_30.rf` and `tm_30.rf_h_per_bin` (test report, TM-30) |
| `cooper-sure-lites-es61src.ulc` | standard | **full** needs test-report-backed sign-face luminance (test report, UL 924), the UL 924 test-report depth that anchors the sign full tier |
| `cooper-atlite-auxswhsd.ulc` | standard | **full** needs test-report-backed sign-face luminance (test report, UL 924) |
| `cooper-sure-lites-lpx7sd.ulc` | core | **standard** needs `legend_height` (datasheet, NFPA 101 / IBC) and `input_power_w` (datasheet, UL 924), the two figures the LPX sheet does not publish. **full** additionally needs test-report-backed sign-face luminance (test report, UL 924) |

The architectural standard records sit one tier below full for the same structural reason: full certifies the complete accredited-report depth, measurement uncertainty, applied corrections, TM-30 fidelity, zonal lumens, and a method-backed maintenance projection, none of which a published cutsheet carries. A record may already satisfy one or two of those gates (Selux carries zonal lumens; Erco a TM-21 maintenance projection), but none carries the whole set. Two records are held at core, each by its own honest gap: the Vode pendant publishes no MacAdam SDCM step, which standard requires of a white-light fixture, and the Cooper LPX exit sign publishes neither its letter height nor a self-powered input wattage. The two Cooper edge-lit signs that reach standard (ES and AUX) are one tier below full for the sign-specific reason: full asks for a test-report-backed face luminance, the UL 924 lab depth a spec sheet does not carry.

The two Lumenpulse facade records reach standard despite publishing no dimming method or dimming range because their driver protocol is DMX/RDM, a digital control protocol. The dimming-method and dimming-range gates apply only to analog and phase-cut drivers (0-10V, 1-10V, phase, PWM-input), whose dim floor and electrical method are published driver specs a designer selects on. Digital (DALI, DMX), wireless, and non-dimming protocols are exempt, because that detail is commanded externally or not conventionally printed on the cutsheet. See `docs/methodology.md` for the predicate.

## How the records were produced

These reference records were authored by hand from each manufacturer's publicly-available spec sheet and photometric files, to serve as canonical, real-data fixtures. Values flow from the IES file header and the spec sheet into the record's deep blocks, and the `index` block is generated by `ulc build-index` from those blocks. Manufacturers producing records at scale do not hand-write JSON: they fill the `templates/workbook/` spreadsheet for the deterministic `ulc from-sheet` converter, or emit from a PIM (see `docs/how-it-works.md`). Every `value_type: measured` field in the record points to an attestation record via `provenance.attestation_ref`; every scaled or simulated value points to a base attestation via `provenance.base_attestation_ref`.

SHA-256 hashes in the `source_files[]` array reference the specific file versions used during authoring. When a manufacturer reissues a cutsheet or regenerates an IES, the hash will differ from what is stored in the record.

## Using these records

To confirm a record parses as valid JSON and see its identifier:

```
python3 -c "import json; print(json.load(open('examples/erco-quintessence-30416-023.ulc'))['record_id'])"
```

This is a parse sanity check, not schema validation. Formal Draft 2020-12 validation against `schema/ulc.schema.json` requires a JSON Schema library that handles cross-file `$ref` (for example Python's `jsonschema` with `referencing`, or any equivalent in another language). The reference `ulc` CLI (at `tools/validator/`) packages schema validation, index-builder parity, and source-file hash verification into one command:

```
ulc validate examples/erco-quintessence-30416-023.ulc
```

To regenerate the index block in place:

```
ulc build-index examples/erco-quintessence-30416-023.ulc
```

To confirm the stored index matches what the builder would emit:

```
ulc build-index --check examples/erco-quintessence-30416-023.ulc
```

## Scope notes

These records represent the four IES-derived authoring patterns plus the exit-sign class, and several distinct product categories (indoor downlight, linear pendant, pole-top outdoor, inground color-changing, and internally illuminated exit signs). They do not exhaustively cover every luminaire category; additional examples targeting track, wall sconce, recessed troffer, cove, landscape, dedicated emergency luminaires, and non-North-American markets are anticipated in future batches.

Each record uses real-world data to stress-test the schema against real manufacturer practice. Some real-world data does not cleanly fit the current schema and is parked under `extensions.manufacturer_specific.<slug>` with a descriptive key. Those parked items are the primary input into the next schema refinement pass.
