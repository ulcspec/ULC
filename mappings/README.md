# Mappings

How ULC relates to the other systems it has to live alongside. Two categories of mapping live here:

1. **Adjacent data standards** — GLDF, ETIM, IES LM-63, EULUMDAT. ULC is designed to cooperate with, not replace, existing work.
2. **Product Information Management (PIM) platforms** — Salsify, Akeneo, SAP, and in-house product databases. How to emit ULC records at catalog scale from the systems that already hold a manufacturer's product data.

## Adjacent data standards

- `gldf-crosswalk.md` (planned) — field-level mapping between ULC records and GLDF (Global Lighting Data Format).
- `etim-crosswalk.md` (planned) — ULC fields to ETIM (ElectroTechnical Information Model) feature identifiers.
- `photometric-source-parsing.md` (planned) — extracting ULC field values from IES LM-63 and EULUMDAT (LDT) files.

How ULC relates to each:

- **GLDF** is the primary interchange container for the DIALux and RELUX planning ecosystems. GLDF and ULC address different problems: GLDF is an XML-based container optimized for photometric planning software, while ULC is a lightweight JSON specification optimized for structured datasheet data and AI consumption.
- **ETIM** provides a widely adopted classification vocabulary for product attributes in electrotechnical wholesale. Individual ULC schema fields cite the corresponding ETIM feature identifiers inline in their descriptions (for example `EF001596` on `HousingMaterial`) where a direct correspondence exists, so downstream consumers can discover the link by reading the schema. The separate compiled `mappings/etim-crosswalk.md` doc that would enumerate the full set of correspondences in one place is still planned and has not yet landed.
- **IES LM-63** and **EULUMDAT (LDT)** are the established photometric data formats. Their content feeds into ULC records; the files themselves are referenced, not replaced.

ULC does not redistribute the text of any paid or restricted standards. It references them by identifier.

## PIM platforms

See [`pim/`](pim/) for platform-specific emit patterns: [Salsify](pim/salsify.md), [Akeneo](pim/akeneo.md), [SAP](pim/sap.md), and [custom / in-house](pim/custom-pim.md). Start with [`pim/README.md`](pim/README.md) for the six shared translation concerns (record-per-scenario model, dual-unit handling, provenance defaults, source-file hashing, category enum mapping, index generation).
