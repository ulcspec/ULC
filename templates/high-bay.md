# Authoring guide: high-bay

Starter for an industrial or commercial high-ceiling luminaire. Category defaults in `high-bay.ulc.json`: `primary_category: "high_bay"`, `mounting_types: ["pendant", "surface_ceiling"]`, `shape: "round"`, `environment_rating: "damp"`, `ip_rating: "IP65"`, `technical_region: "universal"`, `input_voltage_class: "universal_120_277"`, tested at 5000K at the 277V point.

## Workflow

1. Copy `high-bay.ulc.json` into your working directory (any location outside `examples/`, which is reserved for curated canonical reference records), rename to `<manufacturer-slug>-<catalog-slug>-<scenario-slug>.ulc.json`.
2. Replace every `TODO` and every `0` placeholder with real values from the cutsheet, IES, and lab reports.
3. Compute SHA-256 of each source file and paste into every `reference.sha256` field — both the `source_files[].reference.sha256` entries and the family-level `product_family.cutsheet.sha256`.
4. Run `ulc build-index <file>` to regenerate the index, then `ulc validate <file>`.

## Category-specific notes

### `product_family`

- **`primary_category: "high_bay"`** — for ceiling mounts above approximately 20 ft (6 m). For lower ceilings, use `low_bay` instead.
- **`secondary_function: ["wide", "direct"]`** — high-bays flood wide to cover floor area from height. Change to `["spot", "direct"]` for aisle-specific narrow-beam optics. See `taxonomy.schema.json#/$defs/SecondaryFunction` for the full enum.
- **`mounting_types: ["pendant", "surface_ceiling"]`** — most high-bays ship with pendant/hook mount, some with ceiling-flush. Add `track` only if the product actually supports track mounting (rare).
- **`shape`** — `round` for UFO-style; `rectangular` for panel-style; `square` for troffer-style high-bay hybrids.
- **`environment_rating: "damp"`** — high-bays are typically damp-rated for covered but humid indoor environments (warehouses, parking garages). Change to `wet` for open-structure garage applications or food-processing washdown environments.
- **`shared_mechanical.ip_rating`** — `IP65` is typical; `IP66` for washdown; `IP20` on commodity high-bays (not recommended for specs).
- **`shared_mechanical.housing_material: "die_cast_aluminum"`** — typical. Change to `extruded_aluminum` for linear high-bay.
- **`shared_mechanical.lens_material: "prismatic_polymer"`** — typical for round high-bay; `acrylic` for diffused panel-style.

### `photometry`

- **`distribution_type: "direct"`** — high-bays project straight down, full cutoff.
- **`symmetry_type: "symm_full"`** — correct for round UFO high-bays; change to `symm_bi_0` for linear or rectangular variants.
- **`beam_family: "wide_flood"`** — 120° typical. `flood` for narrow aisle optics; `very_wide_flood` for extra-wide warehouse coverage.
- **`luminous_opening_shape`** — `circular` for round; `rectangular` for linear or panel.

### `electrical`

- **`input_voltage_class`** — `universal_120_277` (the template default) is extremely common for high-bays. Change to `120v` or `277v` for single-voltage SKUs, or `347v` for Canadian 347V-only markets. Change `product_family.technical_region` from `universal` to `120v_60hz_north_america` if you constrain the template to a single-voltage North American variant.
- **`driver_protocol: "0-10v"`** — the default for DLC-qualified high-bays. `dali` is rare in North America but common in Europe.
- **`dimming_range_percent: {min: 10, max: 100}`** — commodity high-bay default. Spec-grade go down to `{min: 1, max: 100}`.

### `attestations`

High-bays often carry:

- **`lm_79_08`** — mandatory for all LED luminaires.
- **`lm_80_08`** — LED package lumen maintenance.
- **`dlc_premium`** — mandatory for utility rebates in most North American markets (the template stubs this).
- **`c_ul_listed`** — with `standard_revision: "UL 1598"`.

### Common later additions

- **`lumen_maintenance_package`** and **`lumen_maintenance_luminaire`** with L80 claim (L70 is rarely specified for high-bays; most spec at L80 or L90 at 50k-100k hours).
- **`thermal_derating`** — LM-82 curve matters for warehouses with ambient variation.
- **Integrated sensor metadata** — currently parked in `extensions.manufacturer_specific` pending first-class schema slots for occupancy and daylight-harvesting sensor capabilities.

## See also

- `docs/authoring-patterns.md` — typically Pattern A (one record per SKU, one IES per wattage tier) or Pattern B (one record covers a range of wattage tiers via `applicability.covered_axes.wattage_tier.derivation` with a `method: "wattage_tier_scaling"` rule).
