# Authoring guide: downlight

Walks through each block in `downlight.ulc.json` for a recessed downlight product. Values listed here are the category-typical defaults already present in the template; change them only if your product differs.

## Workflow

1. Copy `downlight.ulc.json` into `examples/` (or your own directory), rename to `<manufacturer-slug>-<catalog-slug>-<scenario-slug>.ulc.json`.
2. Replace every `TODO` and every `0` placeholder with real values from your cutsheet, IES file, and lab reports.
3. Compute the SHA-256 of each source file (`shasum -a 256 file.pdf`) and paste it into the matching `source_files[].reference.sha256` and `product_family.cutsheet.sha256`.
4. Run `ulc build-index --stdout <file>` to preview the projected index, then `ulc build-index <file>` to write it in place.
5. Run `ulc validate <file>` and resolve any errors or warnings.

## Field-by-field notes

### Top level

- **`ulc_version`** — the ULC spec version the record conforms to. Bump only when a new schema revision ships.
- **`record_id`** — lowercase ASCII with hyphens; must match the filename stem minus `.ulc.json`.
- **`record_status`** — `active` for shipping products, `preliminary` during pre-release review, `superseded` after a replacement record exists, `withdrawn` when the product is discontinued.
- **`conformance_level`** — start at `core` while data is incomplete; upgrade to `standard` once measured values are in, and `full` once accessories, sustainability, and thermal derating are populated.

### `product_family`

- **`primary_category: "downlight"`** — do not change this for a downlight template.
- **`mounting_types: ["recessed_ceiling"]`** — change to `["surface_ceiling"]` for a surface-mount downlight; keep both if the SKU supports either with different accessories.
- **`environment_rating: "dry"`** — change to `damp` for bathroom-rated or `wet` for shower/outdoor-rated downlights.
- **`shape: "round"`** — change to `square` for trimless square apertures.
- **`shared_mechanical.housing_material`** — common downlight values: `cast_aluminum`, `die_cast_aluminum`, `extruded_aluminum`, `steel`.
- **`shared_mechanical.lens_material: "cone_only"`** — used when the optic has a reflector cone and no separate lens. Change to `glass_tempered`, `acrylic`, or `polycarbonate` if a lens is present.
- **`physical_dimensions`** — downlights typically populate `overall_diameter`, `ceiling_aperture`, `recess_depth`, and `luminaire_mass`. Omit `overall_length`/`overall_width`/`overall_height` (those are for linear, pendant, or outdoor fixtures).

### `configuration`

- **`photometric_scenario_id`** — unique per tested scenario. For a single-SKU single-IES product, one scenario; for multi-CCT families with per-CCT IES files, one scenario per CCT.
- **`tested_axes.cri_tier`** — `cri_80`, `cri_90`, or `cri_95` depending on which color bin your IES was measured at.
- **`tested_conditions.nominal_cct_at_test`** — the CCT in Kelvin as a string (`"2700"`, `"3000"`, `"3500"`, `"4000"`).
- **`source_ies_ref`** — filename of the IES file that anchors this scenario's measurements, matching an entry in `source_files[]`.

### `applicability`

- **`applicable_catalog_pattern`** — the order-code prefix or pattern this record covers. For Pattern A (one SKU), this equals the full catalog number.
- **`fixed_axes`** — the order-code axes that this specific scenario holds constant (catalog number, CCT, distribution, finish, voltage). Authors of multi-SKU records should read `docs/authoring-patterns.md` for how to express variable axes via `varying_axes[].derivation_rule`.

### `electrical`

- **`input_power_w`** — from the IES file `[INPUT POWER]` keyword or the cutsheet's rated input. Use `value_type: "measured"` with `provenance.source: "ies"` when the IES is the source.
- **`driver_protocol`** — `0-10v`, `dali`, `phase_cut`, `dmx`, `none_non_dim`, etc. Downlights are most commonly `0-10v` in North America and `dali` in Europe.
- **`dimming_method`** — `ccr` (constant-current reduction), `pwm` (pulse-width modulation), or `hybrid`. `ccr` is the default for spec-grade architectural downlights.
- **`dimming_range_percent`** — the declared dimming range as a pair. `{min: 1, max: 100}` is typical for deep-dim drivers; `{min: 10, max: 100}` for entry-level.

### `photometry`

- **`total_luminous_flux_lm`** — from the IES total output. Not the LED package lumens; not the lamp lumens. The luminaire output.
- **`luminaire_efficacy_lm_per_w`** — `total_luminous_flux_lm / input_power_w`, derived in the IES. Use `provenance.method: "computed"`.
- **`beam_family`** — coarse beam category matching the distribution label on the cutsheet: `narrow_spot`, `spot`, `narrow_flood`, `medium_flood`, `flood`, `wide_flood`, or `very_wide_flood`.
- **`symmetry_type`** — `symm_full` for axially-symmetric round downlights (the default); `asymmetric` for wall-wash downlights.
- **`luminous_opening_shape`** — `circular` for round apertures; `square` for square trimless; `rectangular` if the optic is directional.

### `colorimetry`

- **`nominal_cct_k`** — same value as `configuration.tested_conditions.nominal_cct_at_test`, as a string. Must match.
- **`cri_ra`** — general color rendering index from the IES spectral data (`[CRI]` keyword or equivalent).

### `operating_point`

- For North American 120V/60Hz fixtures, the defaults in the template are correct.
- For European 230V/50Hz, change to `{value: 230, unit: "V"}` and `{value: 50, unit: "Hz"}`.

### `source_files`

- Always include the cutsheet PDF (`datasheet_pdf`) and at least one IES file (`ies`).
- Add an LDT file (`ldt`) when the manufacturer publishes one for DIALux/Relux users.
- Add a ULD file (`uld`) when the manufacturer publishes one (emerging format).
- Each entry carries a SHA-256 hash. Compute with `shasum -a 256 <filename>` and paste the 64-char hex string.

### `attestations`

- The template stubs a single LM-79-08 attestation. Add LM-80 (`lm_80_08`) for the LED package, UL listings (`c_ul_listed`, `ul_1598`), IEC listings (`iec_60598`, `iec_62031`), and RoHS/REACH as your product has them.
- Set `status: "verified"` once you have the lab report in hand; `claimed` while the manufacturer asserts conformance without a lab test ID.

### `index`

- **Do not hand-edit.** Always run `ulc build-index <file>` to regenerate after any change to the deep blocks. The index is a projection, not an authoring surface.

## Downlight-specific sections commonly added later

Once your core record validates, expand by adding:

- **`alpha_opic_metrics`** — melanopic DER and per-channel efficacies if you have spectral data per IES RP-46.
- **`flicker_measurements`** — SVM and PstLM per LM-90.
- **`lumen_maintenance_package`** and **`lumen_maintenance_luminaire`** — LM-80 + TM-21 projections and the L70/L80/L90 claim.
- **`compatible_accessories`** — junction boxes, plaster frames, trims, mounting rings (common for architectural downlights).

## See also

- `docs/authoring-patterns.md` — the four manufacturer authoring patterns (A/B/C/D) and which to use.
- `examples/erco-quintessence-30416-023.ulc` — a full real-world downlight example (Pattern A).
- `schema/ulc.schema.json` — the formal schema.
