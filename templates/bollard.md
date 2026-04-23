# Authoring guide: bollard

Starter for an exterior ground-mounted pathway luminaire. Category defaults in `bollard.ulc.json`: `primary_category: "bollard"`, `mounting_types: ["surface_floor"]`, `shape: "round"`, `indoor_outdoor: "outdoor"`, `environment_rating: "wet"`, `ip_rating: "IP65"`, `ik_rating: "IK08"`.

## Workflow

1. Copy `bollard.ulc.json` into `examples/` (or your own directory), rename to `<manufacturer-slug>-<catalog-slug>-<scenario-slug>.ulc.json`.
2. Replace every `TODO` and every `0` placeholder with real values from the cutsheet, IES, and lab reports.
3. Compute SHA-256 of each source file and paste into the matching `reference.sha256` fields.
4. Run `ulc build-index <file>` to regenerate the index, then `ulc validate <file>`.

## Category-specific notes

### `product_family`

- **`primary_category: "bollard"`** — the canonical category for short ground-mounted pathway luminaires. For taller pole-mounted fixtures, use `flood_area_site` with `mounting_types: ["pole_top"]`.
- **`mounting_types: ["surface_floor"]`** — typical for anchor-mounted bollards. Change to `["in_ground"]` for direct-burial or concrete-anchor variants; some products support both.
- **`shape`** — `round` is most common; `square` for cube-style bollards; `custom` for sculpted decorative variants.
- **`environment_rating: "wet"`** — non-negotiable. Bollards sit in rain, snow, sprinkler spray, and puddles.
- **`shared_mechanical.ip_rating: "IP65"`** — minimum for bollards; `IP66` or `IP67` for coastal, fountain-edge, or flood-prone installations.
- **`shared_mechanical.ik_rating: "IK08"`** — minimum; `IK10` for vandal-resistant.
- **`shared_mechanical.housing_material: "cast_aluminum"`** — typical. `stainless_steel` for marine and coastal; `corrosive` environment rating should pair with stainless or powder-coated aluminum.
- **`shared_mechanical.lens_material: "tempered_glass"`** — typical. `polycarbonate` on vandal-resistant lines.

### `photometry`

- **`distribution_type: "asymmetric"`** — bollards typically project sideways and downward (pedestrian pathway pattern), not symmetrically.
- **`symmetry_type: "symm_bi_0"`** — bilateral about C0-C180 for pathway-optic bollards. Change to `symm_full` for symmetric full-cutoff bollards.
- **`beam_family: "asymmetric"`** — correct for pathway-optic. Use `wide_flood` or `flood` only for symmetric distributions.
- **`luminous_opening_shape: "vertical_cylinder"`** — typical for round bollards (cylinder sides emit). `horizontal_cylinder_perpendicular_to_photometric_horizontal` for rectangular bollards. For top-only emission, use `circular`.
- **`emission_face: "c0"`** — for pathway bollards projecting out the C0 direction (front). For top-emission, use `top`.

### `outdoor_classification`

Bollards should populate the `outdoor_classification` block (added after core validates):

- **`outdoor_distribution_type: "type_vs"`** — IESNA Type V Short, typical for symmetric bollards. Other values are `type_i` through `type_v` (see `taxonomy.schema.json#/$defs/OutdoorDistributionType`).
- **`longitudinal_distribution_range`** — `short`, `medium`, or `long`, matching how far the optic throws along the pathway axis.
- **`bug_rating`** — low values (for example `{b: 0, u: 0, g: 0}` or `{b: 1, u: 0, g: 1}`) for spec-grade path bollards designed to minimize light pollution. The index will project this as a short string like `"B1-U0-G1"`.

### `electrical`

- **`driver_protocol: "0-10v"`** — default. Change to `non_dimming` for commodity bollards that are switched but not dimmed.
- **`input_voltage_class`** — `120v` is the template default; `120-277v` for multi-volt commercial. Very few bollards run on 347V.
- **`dimming_range_percent`** — `{min: 10, max: 100}` if dimmed; set both to 100 or omit the block if non-dimming.

### `attestations`

Bollards typically carry:

- **`lm_79_08`** — mandatory.
- **`c_ul_listed`** with `standard_revision: "UL 1598 wet location"` (template stubs this).
- **IP and IK ratings** — pending separate attestation-program slots (Tier 3 schema gap noted in `docs/authoring-patterns.md`).
- **`dlc_standard`** for outdoor-DLC-qualified products.

### Common later additions

- **`outdoor_classification`** with TM-15 BUG rating.
- **`thermal_derating`** — LM-82 curve matters for bollards in full sun or snowbank-adjacent installations.
- **`corrosion_protection`** — finish attestation for coastal/salt environments (ASTM B117 salt spray hours).

## See also

- `examples/selux-aya-pole-sr-ho-3000k.ulc` — a pole-top outdoor example with full `outdoor_classification` populated (different mounting, same outdoor patterns apply).
