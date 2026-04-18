# Examples

This directory contains worked examples of conforming ULC records, each accompanied by the source files (datasheet PDF, IES, LDT) that the record represents.

## Structure

Each example lives in its own folder, named after the fixture it represents. The folder contains:

- The canonical `<manufacturer-slug>-<catalog-slug>.ulc.json` record
- The original datasheet PDF
- The original IES photometric file
- The original EULUMDAT (LDT) photometric file

The source files are included alongside the ULC record as reference, showing exactly what data was available when the record was authored.

## Coverage

Examples span a representative range of luminaire types, from indoor architectural to outdoor infrastructure.

## Data sourcing

Examples use either real manufacturer data contributed with explicit permission, or synthetic data representative of a plausible product. Each example's source and permission status is declared in its folder.

## See also

- `docs/creation-guide.md` for step-by-step guidance on authoring your own ULC records
- `templates/ulc.template.json` for a starter skeleton
