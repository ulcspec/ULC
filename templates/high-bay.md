# Authoring guide: high-bay

Starter for an industrial or commercial high-ceiling luminaire. Category defaults in `high-bay.ulc.json`: `primary_category: "high_bay"`, `mounting_types: ["pendant", "surface_ceiling"]`, `shape: "round"`, `environment_rating: "damp"`, `ip_rating: "IP65"`, `tested at 5000K`.

## Workflow

1. Copy `high-bay.ulc.json` into `examples/` (or your own directory), rename to `<manufacturer-slug>-<catalog-slug>-<scenario-slug>.ulc.json`.
2. Replace every `TODO` and every `0` placeholder with real values from the cutsheet, IES, and lab reports.
3. Compute SHA-256 of each source file and paste into the matching `reference.sha256` fields.
4. Run `ulc build-index <file>` to regenerate the index, then `ulc validate <file>`.

## Category-specific notes

### `product_family`

- **`primary_category: "high_bay"`** — for ceiling mounts above approximately 20 ft (6 m). For lower ceilings, use `low_bay` instead.
- **`secondary_function: ["wide", "direct"]`** — high-bays flood wide to cover floor area from height. Add `narrow` for aisle-specific optics.
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

- **`input_voltage_class`** — `120-277v` multi-volt is extremely common for high-bays; change rated value and class accordingly. Change to `347v` for Canadian 347V-only markets.
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
- **`occupancy_sensing`** and **`daylight_harvesting`** fields if integrated sensors are included.

## See also

- `docs/authoring-patterns.md` — typically Pattern A (one SKU / one IES per wattage tier) or Pattern C (wattage-tier scaling with `derivation_rule`).
