# Templates

Starter templates for authoring ULC records by luminaire category. Each template is a structurally valid ULC record with category-typical defaults pre-filled and obvious placeholders (`"TODO ..."` strings, `0` numerics, 64-zero sentinel SHA-256 hashes) for author-supplied values.

Templates exist to bootstrap a first record. They are not a production authoring surface — manufacturers with catalog-scale output should emit ULC programmatically from their PIM (see `mappings/` for PIM export patterns).

## Note on the committed `index` block

Every template file ships with an `index` block. The ULC specification forbids hand-authoring the index — it must always be a deterministic projection from the deep blocks, produced by `ulc build-index`. The index blocks committed in these template files are not an exception to that rule: each was produced by running `ulc build-index` against the template's own placeholder deep-block values at template-creation time, and the CI `schema-drift-guard` job runs `ulc build-index --check` against every template on every commit to ensure the committed index stays in sync with the deep blocks.

The committed index exists so templates validate at rest against `schema/ulc.schema.json` (which marks `index` as a required top-level field) rather than requiring a build step before validation. Authors who edit a template's deep blocks must regenerate the index with `ulc build-index <file>` before committing; the drift guard will flag any uncomitted drift in CI.

This is an explicit template-only affordance. Canonical reference records in `examples/` follow the same rule but it is less visible there because their deep-block values are settled.

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

1. **Pick the category closest to your product** and copy both the `.ulc.json` template and the sibling `.md` guide **into your own working directory** — anywhere outside the `examples/` directory, which is reserved for the curated canonical reference records. Your working directory can be a separate folder in this repository (not checked in), the manufacturer's own repository, a scratch directory, or anywhere the author normally edits files.
2. **Rename the `.ulc.json`** to `<manufacturer-slug>-<catalog-slug>-<scenario-slug>.ulc.json`. Slugs are lowercase ASCII with hyphens replacing spaces and separators. For example: `selux-elx-48-3500k-90cri.ulc.json`.
3. **Update `record_id` to match the new filename stem.** Every template ships with `record_id: "todo-manufacturer-catalog-scenario"` as a placeholder; replace it with the same slug you used when renaming the file. The validator does not enforce the filename-stem match (it is a repository naming convention, not a schema rule), but every canonical reference record in `examples/` follows it and downstream tooling assumes it.
4. **Fill in every `TODO` and every `0` placeholder** using data from the cutsheet PDF, IES file, and lab attestation reports. The sibling guide explains what each field means for your category.
5. **Compute SHA-256 of each source file** with `shasum -a 256 <file>` and paste the 64-char hex string into the matching `reference.sha256` entries (including `product_family.cutsheet.sha256`).
6. **Regenerate the index** with `ulc build-index <file>`. Do not hand-edit the `index` block.
7. **Validate** with `ulc validate <file>` and resolve any errors or warnings before committing.

## Conformance level

Templates declare `conformance_level: "core"`. Upgrade to `"standard"` once measured values and provenance are in; upgrade to `"full"` once accessories, sustainability declarations, thermal derating, and lumen-maintenance projections are populated.

The reference validator currently emits a single `INFO` marker per record for the declared `conformance_level`. Structural, parity, and hash checks always run; content-completeness grading at `standard` and `full` will land in a later validator release.

## Placeholder conventions

Grep for any of these to find unfilled placeholders in your in-progress record:

- `TODO` — string placeholders that must be replaced
- `todo-manufacturer-catalog-scenario` — placeholder `record_id` that must be replaced with the product's real slug, matching the renamed filename stem
- `0000000000000000000000000000000000000000000000000000000000000000` — sentinel SHA-256; every occurrence must be replaced with a real hash
- `"1970-01-01"` — sentinel date used in both `record_status_as_of` and `product_family.cutsheet.revision_date`. Replace `record_status_as_of` with today's date (the date the author last verified the product's commercial status) and replace `cutsheet.revision_date` with the cutsheet's actual revision date.
- Numeric fields with `0` — replace with real rated or measured values

## Filename extension

Templates use `.ulc.json`; canonical example records use `.ulc`. Both extensions are accepted by `ulc validate` and `ulc build-index` (file-path CLIs, extension-agnostic) and both are covered by the validator CI globs. The two conventions exist only to make it visually obvious at a glance whether a file is an authoring scaffold (`.ulc.json`) or a real record (`.ulc`). `record_id` matches the filename stem in both cases.

## See also

- `docs/authoring-patterns.md` — the four manufacturer authoring patterns (A: one record per SKU; B: one record per photometric scenario covers many SKUs via multiplier tables; C: one record per IES preserves 1:1 file correspondence with provenance classes; D: per-foot linear scaling across fixture lengths) and how to pick the one that matches your cutsheet.
- `schema/ulc.schema.json` — the formal schema each template conforms to.
- `mappings/` — how to emit ULC from Salsify, Akeneo, SAP, and in-house PIMs at scale.
- `examples/` — four real-world reference records, one per authoring pattern. **Do not place new in-progress records here**; this directory is the curated canonical corpus consumers rely on.
