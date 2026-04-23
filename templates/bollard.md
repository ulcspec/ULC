# Authoring guide: bollard

Starter for an exterior ground-mounted pathway luminaire. Category defaults in `bollard.ulc.json`: `primary_category: "bollard"`, `mounting_types: ["surface_floor"]`, `shape: "round"`, `indoor_outdoor: "outdoor"`, `environment_rating: "wet"`, `ip_rating: "IP65"`, `ik_rating: "IK08"`.

## Workflow

1. Copy `bollard.ulc.json` into your working directory (any location outside `examples/`, which is reserved for curated canonical reference records), rename to `<manufacturer-slug>-<catalog-slug>-<scenario-slug>.ulc.json`.
2. Replace every `TODO` and every `0` placeholder with real values from the cutsheet, IES, and lab reports.
3. Compute SHA-256 of each source file and paste into every `reference.sha256` field — both the `source_files[].reference.sha256` entries and the family-level `product_family.cutsheet.sha256`.
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

The template defaults describe the most common bollard type: a symmetric full-cutoff bollard that emits downward from the top of the cylinder. For pathway-optic bollards (asymmetric side-emission along the C0-C180 plane), switch the five fields below to the pathway values in parentheses.

- **`distribution_type: "symmetric"`** (pathway-optic: `"asymmetric"`) — symmetric full-cutoff is the template default; pathway bollards project sideways through the C0-C180 plane.
- **`symmetry_type: "symm_full"`** (pathway-optic: `"symm_bi_0"`) — full rotational symmetry for top-emission bollards; bilateral about C0-C180 for pathway optics.
- **`beam_family: "wide_flood"`** (pathway-optic: `"asymmetric"`) — wide flood is typical for top-emission ground coverage; use `asymmetric` only when the optic is actually asymmetric.
- **`luminous_opening_shape: "circular"`** (pathway-optic: `"vertical_cylinder"` for round bodies, `"horizontal_cylinder_perpendicular_to_photometric_horizontal"` for rectangular bodies) — circular top opening is the default; the cylinder shapes describe side-emission cases.
- **`emission_face: "top"`** (pathway-optic: `"c0"`) — light exits the top face by default; pathway optics emit through a C-plane.

### `outdoor_classification`

Bollards should populate the `outdoor_classification` block (added after core validates):

- **`outdoor_distribution_type`** — `type_v` for symmetric bollards with a circular footprint; `type_vs` for symmetric bollards with a square footprint. (These are footprint variants, not throw-length classifications.) Other values `type_i` through `type_iv` apply to asymmetric optics, with `type_i_four_way` and `type_ii_four_way` available for the four-lobed variants of Types I and II (four-way variants exist only for those two). See `taxonomy.schema.json#/$defs/OutdoorDistributionType`.
- **`longitudinal_distribution_range`** — `short`, `medium`, or `long`, a separate dimension describing how far the optic throws along the pathway axis. Typical pathway bollards are `short` or `medium`.
- **`bug_rating`** — low values (for example `{b: 0, u: 0, g: 0}` or `{b: 1, u: 0, g: 1}`) for spec-grade path bollards designed to minimize light pollution. The index will project this as a short string like `"B1-U0-G1"`.

### `electrical`

- **`driver_protocol: "0-10v"`** — default. Change to `non_dimming` for commodity bollards that are switched but not dimmed.
- **`input_voltage_class`** — `120v` is the template default; `universal_120_277` for multi-volt commercial. Very few bollards run on 347V.
- **`dimming_range_percent`** — `{min: 10, max: 100}` if dimmed; set both to 100 or omit the block if non-dimming.

### `attestations`

Bollards typically carry:

- **`lm_79_08`** — mandatory.
- **`c_ul_listed`** with `standard_revision: "UL 1598 wet location"` (template stubs this).
- **IP and IK ratings** — pending separate `AttestationProgram` slots, listed under "Open items deferred to future schema work" in `docs/authoring-patterns.md`. IP and IK values are currently carried on `shared_mechanical.ip_rating` / `ik_rating`.
- **`dlc_standard`** for outdoor-DLC-qualified products.

### Common later additions

- **`outdoor_classification`** with TM-15 BUG rating.
- **`thermal_derating`** — LM-82 curve matters for bollards in full sun or snowbank-adjacent installations.
- **Coastal / salt-environment finish attestations** (ASTM B117 salt spray hours) — no first-class schema slot yet; park in `extensions.manufacturer_specific.<slug>` with the test hours, apparatus, and finish system. A future revision may promote this into a native attestation pattern.

## See also

- `examples/selux-aya-pole-sr-ho-3000k.ulc` — a pole-top outdoor example with full `outdoor_classification` populated (different mounting, same outdoor patterns apply).
