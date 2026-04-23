# Authoring guide: linear pendant

Starter for a suspended linear luminaire (cove-adjacent, office pendant, library or retail linear). Category defaults in `linear-pendant.ulc.json`: `primary_category: "linear"`, `mounting_types: ["pendant"]`, `shape: "linear"`, `housing_material: "extruded_aluminum"`, `lens_material: "acrylic"`.

## Workflow

1. Copy `linear-pendant.ulc.json` into `examples/` (or your own directory), rename to `<manufacturer-slug>-<catalog-slug>-<scenario-slug>.ulc.json`.
2. Replace every `TODO` and every `0` placeholder with real values from the cutsheet, IES, and lab reports.
3. Compute SHA-256 of each source file and paste into the matching `reference.sha256` fields.
4. Run `ulc build-index <file>` to regenerate the index, then `ulc validate <file>`.

## Category-specific notes

### Authoring pattern

Linear pendants most commonly follow **Pattern D: length-scaled photometry.** The cutsheet publishes one IES measured at a single length (often 4 ft) and scales lumens and power per foot across other lengths. See `docs/authoring-patterns.md` for how to express length scaling via `applicability.varying_axes[].derivation_rule: "linear_rate"`.

If your product publishes a separate IES per length instead, use Pattern A or B, and delete the `linear_rate` derivation rules from `applicability`.

### `product_family`

- **`mounting_types`** — `["pendant"]` is the default. If the product also supports surface-ceiling or wall-mount installation, add those values too.
- **`shape: "linear"`** — keep as is; linear covers pendants, recessed linear, and surface-mount linear.
- **`shared_mechanical.housing_material`** — `extruded_aluminum` is typical. `steel` is rare for architectural linear. Change only if the cutsheet explicitly specifies.
- **`shared_mechanical.lens_material`** — `acrylic` or `optical_polymer` for most architectural linear. `prismatic_polymer` when the lens has a batwing or asymmetric optic.
- **`physical_dimensions`** — populate `overall_length`, `overall_width`, `overall_height`, and `luminaire_mass`. For Pattern D, also populate `linear_mass_per_foot` (uses `DualUnitMassPerLength` with `kg_per_m` and `lb_per_ft`). Omit `overall_diameter`, `ceiling_aperture`, `recess_depth`.

### `photometry`

- **`symmetry_type: "symm_bi_0"`** — linear fixtures have bilateral symmetry about the C0-C180 plane. Keep as is.
- **`luminous_opening_shape: "rectangular"`** — correct for standard linear; `rectangular_with_luminous_sides` if the optic emits through the side walls too.
- **`distribution_type`** — `direct` for the default downward-emitting pendant; change to `direct_indirect` for up-down pendants; `indirect` for pure uplight.
- **`emission_face: "bottom"`** — correct for direct. For direct-indirect, the single-face model is a simplification; the TM-33 emission-areas model in the record's richer blocks is the long-term answer.
- **`beam_family`** — `medium_flood` or `flood` is typical for linear; `wide_flood` for wall-wash linear.

### `electrical`

- **`driver_protocol`** — `0-10v` is typical North American, `dali_2` for European spec-grade, `dali_dt8` for tunable-white.
- **`dimming_range_percent`** — `{min: 1, max: 100}` for deep-dim architectural drivers; `{min: 10, max: 100}` for commodity drivers.

### Common later additions

- **`lumen_maintenance_package`** and **`lumen_maintenance_luminaire`** — LM-80 package data and the L70/L80 claim.
- **`flicker_measurements`** — SVM and PstLM per LM-90 (spec-grade pendants usually report these).
- **`alpha_opic_metrics`** — circadian metrics for wellness-spec projects.

## See also

- `examples/vode-nexa-suspended-807-so-3500k-90cri-hl-black-48in.ulc` — a real-world linear pendant (Pattern D, length-scaled).
- `docs/authoring-patterns.md` — how to express length-scaling in `applicability.varying_axes`.
