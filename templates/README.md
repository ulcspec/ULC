# Templates

Starter templates for authoring ULC records by luminaire category. Each template is a structurally valid ULC record with category-typical defaults pre-filled and obvious placeholders (`"TODO ..."` strings, `0` numerics, 64-zero sentinel SHA-256 hashes) for author-supplied values.

Templates exist to bootstrap a first record. They are not a production authoring surface — manufacturers with catalog-scale output should emit ULC programmatically from their PIM (see `mappings/` for PIM export patterns).

## Categories

| Template | Category | Typical product |
|---|---|---|
| [`downlight.ulc.json`](downlight.ulc.json) / [guide](downlight.md) | `downlight` | Recessed ceiling downlight, architectural spec-grade |
| [`linear-pendant.ulc.json`](linear-pendant.ulc.json) / [guide](linear-pendant.md) | `linear` | Suspended linear pendant, office or retail |
| [`wall-pack.ulc.json`](wall-pack.ulc.json) / [guide](wall-pack.md) | `bulkhead_wall_pack` | Exterior wall-mount area luminaire |
| [`high-bay.ulc.json`](high-bay.ulc.json) / [guide](high-bay.md) | `high_bay` | Warehouse or industrial high-ceiling luminaire |
| [`bollard.ulc.json`](bollard.ulc.json) / [guide](bollard.md) | `bollard` | Exterior ground-mount pathway luminaire |
| [`wall-sconce.ulc.json`](wall-sconce.ulc.json) / [guide](wall-sconce.md) | `sconce` | Interior wall-mount decorative or architectural |

Each template pairs a `.ulc.json` skeleton with a sibling `.md` guide walking through category-specific field conventions.

## Workflow

1. **Pick the category closest to your product** and copy both the `.ulc.json` template and the sibling `.md` guide.
2. **Rename the `.ulc.json`** to `<manufacturer-slug>-<catalog-slug>-<scenario-slug>.ulc.json`. Slugs are lowercase ASCII with hyphens replacing spaces and separators. For example: `selux-elx-48-3500k-90cri.ulc.json`.
3. **Fill in every `TODO` and every `0` placeholder** using data from the cutsheet PDF, IES file, and lab attestation reports. The sibling guide explains what each field means for your category.
4. **Compute SHA-256 of each source file** with `shasum -a 256 <file>` and paste the 64-char hex string into the matching `reference.sha256` entries (including `product_family.cutsheet.sha256`).
5. **Regenerate the index** with `ulc build-index <file>`. Do not hand-edit the `index` block.
6. **Validate** with `ulc validate <file>` and resolve any errors or warnings before committing.

## Conformance level

Templates declare `conformance_level: "core"`. Upgrade to `"standard"` once measured values and provenance are in; upgrade to `"full"` once accessories, sustainability declarations, thermal derating, and lumen-maintenance projections are populated.

The reference validator currently emits a single `INFO` marker per record for the declared `conformance_level`. Structural, parity, and hash checks always run; content-completeness grading at `standard` and `full` will land in a later validator release.

## Placeholder conventions

Grep for any of these to find unfilled placeholders in your in-progress record:

- `TODO` — string placeholders that must be replaced
- `0000000000000000000000000000000000000000000000000000000000000000` — sentinel SHA-256; every occurrence must be replaced with a real hash
- `"2026-01-01"` — sentinel date; replace with actual cutsheet revision date
- Numeric fields with `0` — replace with real rated or measured values

## See also

- `docs/authoring-patterns.md` — the four manufacturer authoring patterns (A: one SKU / one IES; B: one SKU / multiple IES; C: many SKUs / one IES with derivation rules; D: length-scaled) and how to pick the one that matches your cutsheet.
- `schema/ulc.schema.json` — the formal schema each template conforms to.
- `mappings/` — how to emit ULC from Salsify, Akeneo, SAP, and in-house PIMs at scale.
- `examples/` — four real-world reference records, one per authoring pattern.
