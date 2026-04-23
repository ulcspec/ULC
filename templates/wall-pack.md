# Authoring guide: wall-pack (bulkhead)

Starter for an exterior wall-mounted area luminaire. Category defaults in `wall-pack.ulc.json`: `primary_category: "bulkhead_wall_pack"`, `mounting_types: ["surface_wall"]`, `shape: "rectangular"`, `indoor_outdoor: "outdoor"`, `environment_rating: "wet"`, `ip_rating: "IP65"`, `ik_rating: "IK08"`.

## Workflow

1. Copy `wall-pack.ulc.json` into your working directory (any location outside `examples/`, which is reserved for curated canonical reference records), rename to `<manufacturer-slug>-<catalog-slug>-<scenario-slug>.ulc.json`.
2. Replace every `TODO` and every `0` placeholder with real values from the cutsheet, IES, and lab reports.
3. Compute SHA-256 of each source file and paste into every `reference.sha256` field — both the `source_files[].reference.sha256` entries and the family-level `product_family.cutsheet.sha256`.
4. Run `ulc build-index <file>` to regenerate the index, then `ulc validate <file>`.

## Category-specific notes

### `product_family`

- **`primary_category: "bulkhead_wall_pack"`** — use this for bulkhead-form and wall-pack-form fixtures: full-cutoff perimeter packs, round/drum bulkheads, and utility-style building-mounted area lights. This is a **form** category, not an environment split: a decorative sconce that happens to be rated for outdoor use stays under `sconce` (see `wall-sconce.md`), and a bulkhead used indoors (rare but possible, for example in covered parking) stays under `bulkhead_wall_pack`. Indoor/outdoor exposure is carried separately by `indoor_outdoor` and `environment_rating`.
- **`secondary_function: ["asymmetric", "flood"]`** — wall packs typically project light away from the wall asymmetrically. Add `wall_wash` if the optic is designed to graze downward along the wall.
- **`shape`** — `rectangular` is most common; `round` for drum-style bulkheads; `square` for cube fixtures.
- **`environment_rating`** — `wet` is the typical default. A damp-only exterior bulkhead (covered entry porch, protected soffit) still belongs to `bulkhead_wall_pack`; downgrade `environment_rating` to `damp` rather than re-categorizing.
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

- **`outdoor_distribution_type`** — IESNA Type I through V per TM-15 (`type_i`, `type_ii`, `type_iii`, `type_iv`, `type_v`, `type_vs`, and the four-way variants). See `taxonomy.schema.json#/$defs/OutdoorDistributionType`.
- **`longitudinal_distribution_range`** — `short`, `medium`, or `long`.
- **`bug_rating`** — BackLight, UpLight, Glare ratings as `{b, u, g}` integers 0-5.
- Optional **`legacy_cutoff`** — legacy cutoff classification (`full_cutoff`, `cutoff`, `semi_cutoff`, `non_cutoff`) when the cutsheet still publishes one.

The index will automatically project `bug_rating` as a short string (for example `"B1-U0-G2"`).

### `attestations`

Exterior wall packs typically carry more program attestations than indoor fixtures:

- **`c_ul_listed`** with `standard_revision: "UL 1598 wet location"` (template stubs this).
- **`iec_60598`** with `standard_revision: "IEC 60598-2-1 fixed general purpose luminaires"` for non-US markets (this is the correct part for building-perimeter wall packs; the road/street-lighting part `60598-2-3` applies only to roadway-class fixtures).
- **`dlc_standard`** or **`dlc_premium`** for DLC-qualified utility-rebate products.
- **IP and IK ratings** are carried on `product_family.shared_mechanical.ip_rating` and `shared_mechanical.ik_rating` directly, not as entries in the `attestations[]` array. First-class `AttestationProgram` values for these are a Tier 3 schema gap (see `docs/authoring-patterns.md`).

### `electrical`

- **`input_voltage_class`** — `universal_120_277` multi-volt is common for wall packs; change the rated value and class accordingly.
- **`driver_protocol`** — `0-10v` is typical; `non_dimming` for commodity wall packs.

### Common later additions

- **`outdoor_classification`** (TM-15 BUG rating).
- **`thermal_derating`** — ambient-to-flux curve per LM-82 matters for outdoor fixtures mounted in full sun.
- **`chromaticity_shift_projection`** — TM-35 if the manufacturer reports it.

## See also

- `docs/authoring-patterns.md` — mapping cutsheets into ULC.
- `examples/selux-aya-pole-sr-ho-3000k.ulc` — a pole-top outdoor example with full `outdoor_classification` populated (not a wall pack, but the outdoor-section pattern transfers).
