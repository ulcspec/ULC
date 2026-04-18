# Templates

This directory contains starter templates for authoring ULC records.

## Files

- `ulc.template.json` is an empty but structurally valid ULC record skeleton. Copy this file, rename it following the naming convention below, and fill in the fields for your product.

## Naming convention

New ULC files should follow the pattern:

```
<manufacturer-slug>-<catalog-slug>.ulc.json
```

For example, `selux-elx-48.ulc.json`. Manufacturer and catalog slugs are lowercase ASCII with hyphens replacing spaces and separators.

## See also

- `docs/creation-guide.md` for a full walkthrough of authoring a ULC record from a datasheet, IES file, and LDT file
- `schema/ulc.schema.json` for the formal schema the template conforms to
