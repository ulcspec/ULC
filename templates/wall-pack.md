# Authoring guide: wall-pack (bulkhead)

Starter for an exterior wall-mounted area luminaire. Category defaults in `wall-pack.ulc.json`: `primary_category: "bulkhead_wall_pack"`, `mounting_types: ["surface_wall"]`, `shape: "rectangular"`, `indoor_outdoor: "outdoor"`, `environment_rating: "wet"`, `ip_rating: "IP65"`, `ik_rating: "IK08"`.

## Workflow

1. Copy `wall-pack.ulc.json` into `examples/` (or your own directory), rename to `<manufacturer-slug>-<catalog-slug>-<scenario-slug>.ulc.json`.
2. Replace every `TODO` and every `0` placeholder with real values from the cutsheet, IES, and lab reports.
3. Compute SHA-256 of each source file and paste into the matching `reference.sha256` fields.
4. Run `ulc build-index <file>` to regenerate the index, then `ulc validate <file>`.

## Category-specific notes

### `product_family`

- **`primary_category: "bulkhead_wall_pack"`** — the canonical category for all exterior wall-mount luminaires including full-cutoff wall packs, traditional bulkheads, and decorative outdoor sconces.
- **`secondary_function: ["asymmetric", "flood"]`** — wall packs typically project light away from the wall asymmetrically. Add `wall_wash` if the optic is designed to graze downward along the wall.
- **`shape`** — `rectangular` is most common; `round` for drum-style bulkheads; `square` for cube fixtures.
- **`environment_rating: "wet"`** — non-negotiable for exterior wall mount. If the fixture is only damp-rated, the product is probably not a wall pack but an exterior sconce; use `primary_category: "sconce"` instead.
- **`shared_mechanical.ip_rating`** — `IP65` is typical; `IP66` for high-pressure washdown or coastal; `IP67` for occasional submersion (rare for wall packs).
- **`shared_mechanical.ik_rating`** — `IK08` or `IK09` is typical vandal-resistance for building-perimeter fixtures.
- **`shared_mechanical.lens_material: "tempered_glass"`** — most common for wet-location wall packs. `polycarbonate` on vandal-resistant variants.

### `photometry`

- **`distribution_type: "asymmetric"`** — wall packs project forward and down, not symmetrically. Keep as is.
- **`symmetry_type: "symm_bi_0"`** — bilateral about the C0-C180 plane (the vertical plane through the fixture).
- **`beam_family: "wide_flood"`** — typical; `flood` for narrower beam spread; `very_wide_flood` for full-cutoff wall packs.
- **`emission_face: "bottom"`** — if the fixture emits forward and down. For pure-downlight bulkheads (full cutoff), stays `bottom`. For decorative up-down wall-mount, the TM-33 emission-areas model handles it better than a single face.

### `outdoor_classification`

Outdoor wall packs should populate the `outdoor_classification` block (added to the record after the core validates) with:

- **`luminaire_classification_system`** — IESNA Type I through V per TM-15.
- **`bug_rating`** — BackLightUpLightGlare rating as `{b, u, g}` ints 0-5.

The index will automatically project `bug_rating` as a short string (for example `"B1-U0-G2"`).

### `attestations`

Exterior wall packs typically carry more program attestations than indoor fixtures:

- **`c_ul_listed`** with `standard_revision: "UL 1598 wet location"` (template stubs this).
- **`iec_60598`** with `standard_revision: "IEC 60598-2-3 road and street lighting"` for non-US markets.
- **`dlc_standard`** or **`dlc_premium`** for DLC-qualified utility-rebate products.
- **IP rating attestation** — treated as a separate entry with `program: "ip_rating_<value>"` per the Tier 3 gap noted in `docs/authoring-patterns.md`.

### `electrical`

- **`input_voltage_class`** — `120-277v` multi-volt is common for wall packs; change the rated value and class accordingly.
- **`driver_protocol`** — `0-10v` is typical; `non_dimming` for commodity wall packs.

### Common later additions

- **`outdoor_classification`** (TM-15 BUG rating).
- **`thermal_derating`** — ambient-to-flux curve per LM-82 matters for outdoor fixtures mounted in full sun.
- **`chromaticity_shift_projection`** — TM-35 if the manufacturer reports it.

## See also

- `docs/authoring-patterns.md` — mapping cutsheets into ULC.
- `examples/selux-aya-pole-sr-ho-3000k.ulc` — a pole-top outdoor example with full `outdoor_classification` populated (not a wall pack, but the outdoor-section pattern transfers).
