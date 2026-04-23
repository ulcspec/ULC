# Authoring guide: wall sconce

Starter for an interior wall-mounted decorative or architectural luminaire. Category defaults in `wall-sconce.ulc.json`: `primary_category: "sconce"`, `mounting_types: ["surface_wall"]`, `shape: "rectangular"`, `indoor_outdoor: "indoor"`, `environment_rating: "dry"`, tested at 2700K (warm residential default).

## Workflow

1. Copy `wall-sconce.ulc.json` into `examples/` (or your own directory), rename to `<manufacturer-slug>-<catalog-slug>-<scenario-slug>.ulc.json`.
2. Replace every `TODO` and every `0` placeholder with real values from the cutsheet, IES, and lab reports.
3. Compute SHA-256 of each source file and paste into the matching `reference.sha256` fields.
4. Run `ulc build-index <file>` to regenerate the index, then `ulc validate <file>`.

## Category-specific notes

### `product_family`

- **`primary_category: "sconce"`** — for interior wall-mount. For exterior wall-mount, use `bulkhead_wall_pack` instead. For a vanity light over a bathroom mirror, `sconce` is still the right category; adjust `environment_rating` to `damp`.
- **`secondary_function: ["direct_indirect"]`** — common for architectural ADA sconces that emit both up and down. Change to `["direct"]` for pure-downlight or `["indirect"]` for pure-uplight variants.
- **`shape`** — `rectangular` is typical architectural; `square` for cube sconces; `round` for drum-style; `custom` for decorative or sculptural.
- **`environment_rating`** — `dry` for typical interior living spaces. `damp` for bathroom vanity, kitchen, or unconditioned interior (stairwells in humid climates).
- **`shared_mechanical.housing_material: "die_cast_aluminum"`** — typical for architectural lines. `steel` for traditional; `brass` or `copper` for decorative luxury.
- **`shared_mechanical.lens_material: "acrylic"`** — typical. `glass` for traditional or decorative; `diffuser_polymer` for softer light quality.

### `physical_dimensions`

- Populate `overall_length`, `overall_width`, `overall_height`, and `luminaire_mass`.
- For ADA compliance, `overall_height` from the wall matters (must be under 4 inches / 100 mm of projection). Note it in `product_family.extensions.manufacturer_specific` if relevant to the customer.

### `photometry`

- **`distribution_type: "direct_indirect"`** — matches the default secondary function. Change to `direct` for downlight-only sconces.
- **`symmetry_type: "symm_bi_0"`** — bilateral about the C0-C180 plane (wall-facing and room-facing halves). Correct for most sconces.
- **`beam_family: "medium_flood"`** — typical. `flood` or `wide_flood` for wash sconces; `narrow_flood` for downward-accent.
- **`luminous_opening_shape: "rectangular"`** — for rectangular sconces. `circular` for round; `vertical_cylinder` for drum-style.
- **`emission_face: "bottom"`** — approximates a direct sconce. For direct-indirect, the TM-33 emission-areas model is more accurate; the single-face model is a simplification that the `face` field carries forward pending richer photometry.

### `electrical`

- **`driver_protocol: "phase_forward"`** — typical for residential and residential-adjacent sconces (TRIAC/forward-phase dimmer compatibility). Change to `0-10v` for commercial spec; `phase_reverse` for ELV/reverse-phase dimmers; `dali` for European architectural.
- **`dimming_range_percent: {min: 5, max: 100}`** — typical for phase-cut. Commercial 0-10V drivers go to `{min: 1, max: 100}`.
- **`input_voltage_class: "120v"`** — typical residential. `120-277v` for commercial multi-volt.

### `attestations`

Sconces typically carry:

- **`lm_79_08`** — mandatory for LED.
- **`c_ul_listed`** with `standard_revision: "UL 1598"` (dry location) or `"UL 1598 damp location"` for bathroom.
- **`energy_star`** for residential ENERGY STAR qualification (template does not stub this; add if applicable).

### Common later additions

- **`compatible_accessories`** — junction box covers, decorative trims, lamp shades (for residential styles).
- **`lumen_maintenance_package`** and **`lumen_maintenance_luminaire`** — L70 at 25k-50k hours is typical for residential sconces; commercial spec at L80 at 50k+.
- **`color_tunability_extensions`** — for dim-to-warm sconces (1800K-3000K warm-dim curves), populate the dim-to-warm block.

## See also

- `docs/authoring-patterns.md` — typically Pattern A (one SKU / one IES) for sconces since lamping and finish variations rarely change photometry meaningfully.
