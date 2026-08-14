## ULC unit-conversion policy

How the two sides of a dual-unit value in a ULC record relate: which leaf is authoritative, the conversion rule each family uses, and the rounding each computed companion receives. This is the policy the reference converter (`ulc from-sheet`) implements; an emitter that computes companions itself (see `mappings/pim/`) follows the same rules so identical inputs produce identical records. A copy ships in every release archive as `CONVERSION-POLICY.md`; the paths in this document are repository paths in the ULC repository.

### The dual-unit rule

Five families of unit-sensitive field carry both an SI and an Imperial representation, and the schema requires both leaves on each. SI is authoritative: the specification fixes SI as the governing side (see `docs/methodology.md`), most dual-unit schema descriptions restate it, and the reference toolchain's graded reads take the SI leaf (the conformance rubric reads `mm`, never `in`). The record does not say which side was authored; it carries both values, and this policy fixes how one is computed from the other.

### Families, conversion rules, and rounding

Five dual-unit families exist. Four convert by a single constant each; temperature converts by a fixed formula. The authored value is always written verbatim on its own leaf; only the computed companion is rounded.

| Family | SI leaf | Imperial leaf | Conversion | Computed Imperial (SI authored) | Computed SI (Imperial authored) |
|---|---|---|---|---|---|
| Length | `mm` | `in` | 25.4 mm per in | `in = mm / 25.4`, at most 4 decimal places | `mm = in * 25.4`, at most 4 decimal places |
| Mass | `kg` | `lb` | 2.2046226 lb per kg | `lb = kg * 2.2046226`, 1 decimal place | `kg = lb / 2.2046226`, at most 4 decimal places |
| Temperature | `c` | `f` | `f = c * 9/5 + 32` | not rounded (mirrors the source data) | `c = (f - 32) * 5/9`, at most 4 decimal places |
| Area | `m2` | `ft2` | 10.7639 ft2 per m2 | `ft2 = m2 * 10.7639`, at most 4 decimal places | `m2 = ft2 / 10.7639`, at most 4 decimal places |
| Mass per length | `kg_per_m` | `lb_per_ft` | 0.671969 lb/ft per kg/m | `lb_per_ft = kg_per_m * 0.671969`, at most 4 decimal places | `kg_per_m = lb_per_ft / 0.671969`, at most 4 decimal places |

Rounding is round half away from zero. "At most" means trailing zeros are not padded: `113 mm` computes to `4.4488 in`, and `114.3 mm` computes to `4.5 in`, not `4.5000`. Whole numbers render as integers (`113`, not `113.0`).

Each of the four factor families converts by one constant, applied by multiplication in one direction and division in the other, and temperature uses the same formula in both directions, inverted, so the two sides of a value never disagree about which rule governs. Because the computed side is rounded, converting a value out and then back does not in general return the original number, which is exactly why the authored side is written verbatim and never recomputed. On the Imperial-authored direction the computed SI leaf is rounded to remove binary floating-point artifacts (in IEEE 754 arithmetic `96 * 25.4` evaluates to `2438.3999999999996`; the record carries `2438.4`), not to coarsen authored precision.

### Why the authored side is verbatim

A manufacturer's published number appears in the record exactly as printed. Rounding only the computed side means a cutsheet value can always be found byte for byte in the record, whichever unit system it was authored in, and a computed companion never feeds back into the authored leaf.

### Exceptions

Some unit-bearing fields are deliberately single-unit, documented in their schema descriptions: LM-82 temperature sample arrays mirror their source data in Celsius only, exit-sign face luminance is recorded in cd/m2 with no footlambert companion, illuminance values are carried in lux only, and embodied-carbon values are SI-native (kg CO2e) because life-cycle-assessment practice publishes no Imperial figure. The schema descriptions are authoritative for these exceptions.

### Where this policy is implemented

The reference implementation is the `ulc from-sheet` converter in `tools/validator/`. The workbook's `records` sheet accepts either side of a dual-unit field: author the SI column (for example `overall_diameter_mm`) or its Imperial companion column (`overall_diameter_in`), never both for one field on one row. The `declared_by_length` sheet's `length_mm` column is SI-only. See `templates/workbook/README.md` for the column reference and `docs/methodology.md` for why SI is the authoritative side.
