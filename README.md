<p align="center">
  <img src="assets/logo.png" alt="ULC - Universal Luminaire Cutsheet" width="400" />
</p>

# ULC

**Universal Luminaire Cutsheet**

> "Cutsheet" in the name is the North American term for what is globally called a **datasheet**, the manufacturer's published technical summary of a luminaire. This document uses "datasheet" as the primary term in body text; both refer to the same document type.

ULC is an open specification for structured, machine-readable luminaire product data. A ULC file is a single JSON document that normalizes the information currently spread across manufacturer datasheets, IES photometric files, and EULUMDAT (LDT) files into one consistent, canonical record.

**A single ULC record represents one attested photometric scenario for a luminaire.** A product family with multiple distributions, output tiers, or color-rendition variants produces multiple ULC records, one per scenario, each carrying its own measurements and an applicability block that declares which orderable SKU configurations the record covers.

ULC does not replace those source files. It provides a normalized representation of their combined content that AI systems, design tools, specification software, and data pipelines can consume directly, with verifiable references back to the originals.

## Why ULC exists

Luminaire product data today is fragmented across thousands of PDF datasheets, each formatted differently, alongside decades-old text-based photometric files with inconsistent conventions. That worked while designers read datasheets one by one. It does not work now that AI systems are increasingly expected to consume product data at scale.

Structured JSON is orders of magnitude cheaper, faster, and more reliable for AI systems to process than running extraction pipelines over unstructured PDFs. For a specifier evaluating a hundred-product value engineering package against a hundred-product basis of design, the gap between structured and unstructured source data is the gap between an instant automated comparison and hours of manual reconciliation.

ULC closes that gap. The datasheet, IES, and LDT remain where they are, published as they always have been. The ULC file provides a normalized, machine-readable representation of their combined content, ready for any AI agent or software system to consume without preprocessing.

A universal schema also surfaces data quality issues that are invisible today. When every manufacturer uses their own taxonomy and their own notion of what belongs on a datasheet, cross-manufacturer comparison is unreliable by default. ULC defines a common structure where missing fields are explicit rather than hidden, which benefits every consumer of the data.

## What ULC is

A ULC record is a single JSON document that conforms to the ULC schema. It carries:

- Product identity, family, and taxonomy
- Physical dimensions in both SI and Imperial units
- Electrical, optical, photometric, and performance data
- Environmental ratings, compliance markings, and accessories
- Provenance for every extracted value, so the source of each field is always traceable
- References to the original source files (datasheet PDF, IES, LDT) including filename, optional URL, and a SHA-256 content hash for integrity verification

ULC does not embed source files. It identifies them. A consumer who obtains a source file through any channel can verify it matches the ULC record by comparing hashes.

## Source inputs

ULC is designed around the three source types that are realistically available today across the industry:

- PDF datasheets
- IES photometric files (ANSI/IES LM-63)
- EULUMDAT photometric files (LDT)

Additional source types and fields may be supported in future versions.

## Who uses ULC

| Audience | How they use ULC |
|---|---|
| Specifiers and designers | Consume ULC data indirectly through tools that read it. Benefit from fast, accurate, AI-assisted comparisons, luminaire schedules, and value engineering review. |
| Manufacturers | Publish ULC files alongside datasheet PDFs, IES, and LDT files on their product pages. Gain machine-discoverability by AI systems and improved data fidelity in downstream workflows. |
| Software vendors | Implement readers, writers, and validators against the ULC schema. The reference CLI validator (`ulc`, at `tools/validator/`) packages JSON Schema validation, index-builder parity, and source-file hash verification into one command, shipped as a single-file binary; any JSON Schema Draft 2020-12 library can also validate ULC records against the schemas directly. |
| AI agents and assistants | Parse ULC files directly to answer product queries, compare alternates, generate documentation, and automate design tasks. Any JSON-aware system can consume the format, including general-purpose assistants such as ChatGPT and Claude and domain-specific agents such as LightingAgent.AI. |

Product discoverability is shifting. General-purpose search engines that once indexed PDF datasheets are being supplemented, and in some workflows replaced, by AI agents that retrieve and compare product data on the user's behalf. Machine-readable product data that can be parsed directly, without PDF extraction, is the input AI agents prefer. Manufacturers who publish ULC records put their products in reach of that new retrieval path.

This repository defines the standard. It does not ship an application.

## Repository structure

| Path | Contents |
|---|---|
| `schema/` | Two JSON Schema files (Draft 2020-12): `ulc.schema.json` defines the record structure; `taxonomy.schema.json` defines the closed-enum vocabulary. They are split so the taxonomy can be loaded independently by search and classification tools. Cross-file references are validated in CI. |
| `docs/` | Narrative specification, field reference, authoring guide, and `authoring-patterns.md` describing the four manufacturer authoring patterns the schema supports. |
| `examples/` | Canonical reference ULC records, one per manufacturer authoring pattern (A/B/C/D), drafted from real spec sheets and IES files. Source files are referenced by required SHA-256 hash and optional URL, not committed. |
| `templates/` | Per-category starter templates (downlight, linear-pendant, wall-pack, high-bay, bollard, wall-sconce). Each is a structurally valid `.ulc.json` skeleton with category-typical defaults and a sibling `.md` authoring guide. |
| `mappings/` | Two kinds of mappings. Adjacent data standards: planned crosswalks to GLDF and ETIM plus guidance for parsing IES and LDT sources. PIM platforms: how to emit ULC records at catalog scale from Salsify, Akeneo, SAP, or an in-house PIM (`mappings/pim/`). |
| `tools/` | Reference utilities: the schema drift guard (`schema-drift-guard.py`, Python) and the `ulc` reference command-line validator at `tools/validator/` (Go, ships as a single-file binary via GoReleaser). |
| `.github/` | Issue templates, pull request template, and continuous integration (including the schema drift guard workflow) |

## Getting started

The current working state ships the schema, taxonomy, drift-guard tooling, the authoring-patterns document, four canonical reference records covering the four manufacturer authoring patterns, and the reference command-line validator (`ulc`) at `tools/validator/` with schema validation, builder parity, and source-file hash verification. Deep narrative guides are part of later batches.

- To understand the data model, read `docs/authoring-patterns.md`. It describes the four manufacturer authoring patterns ULC supports and the architectural primitives (product family, configuration, applicability, generated index, provenance classes, conditional attestations).
- To see those patterns in real data, read the four records in `examples/`. Each one exercises a distinct pattern against a real manufacturer spec sheet.
- To explore the schema directly, read `schema/ulc.schema.json` for the record structure and `schema/taxonomy.schema.json` for the closed vocabularies.
- To implement ULC in your own software, reference those two schema files by URL and use any JSON Schema Draft 2020-12 validator. The `tools/schema-drift-guard.py` script shows how `$ref`s resolve across the split.
- To validate a record end-to-end (schema, index-builder parity, source-file hashes), run `ulc validate <record>` using the reference CLI. `<record>` is the path to any `.ulc` or `.ulc.json` file; both extensions are accepted. Download a release binary from the GitHub Releases page, or build from source with `cd tools/validator && go build -o bin/ulc ./cmd/ulc`.
- To regenerate a record's `index` block, run `ulc build-index <record>`. The index is always generated, never hand-authored.

## Relationship to adjacent standards

ULC is designed to cooperate with, not replace, existing work in the lighting data ecosystem:

- **GLDF** (Global Lighting Data Format) is the primary interchange container for the DIALux and RELUX planning ecosystems. ULC and GLDF address different problems: GLDF is a rich XML-based container optimized for photometric planning software, while ULC is a lightweight JSON specification optimized for structured datasheet data and AI consumption. A field-level mapping between ULC and GLDF is planned at `mappings/gldf-crosswalk.md` and has not yet landed. Future tooling could generate GLDF output from ULC records, and vice versa, although neither direction is implemented as part of this repository today.
- **ETIM** (ElectroTechnical Information Model) provides a widely adopted classification vocabulary for product attributes in electrotechnical wholesale. A crosswalk documenting the corresponding ETIM feature identifiers for ULC luminaire fields is planned at `mappings/etim-crosswalk.md` and has not yet landed.
- **IES LM-63** and **EULUMDAT** remain the photometric data formats that feed ULC. ULC does not duplicate or replace their content. A guide for extracting ULC field values from IES and LDT files is planned at `mappings/photometric-source-parsing.md` and has not yet landed.

ULC does not redistribute the text of any paid or restricted standards. It references them by identifier.

## Project status

Version `0.1.0` establishes the foundation of the specification: the split schema (`ulc.schema.json` plus `taxonomy.schema.json`), the authoring-patterns document, and the drift-guard tooling. Version `0.2.0` adds four canonical reference records in `examples/`, one per manufacturer authoring pattern. Version `0.3.0` refines the schema primarily through additive changes informed by those reference records, moving previously extension-parked content into native fields (physical dimensions, dimming range and method, cutoff angle, UGR bound operator, per-length photometric declarations, technical region, and others). One field (`Configuration.tested_axes.cri_tier`) was tightened from free-string to a closed enum; all v0.2 reference records use values that remain valid, so practical impact is zero. Version `0.4.0` ships the reference command-line validator as `ulc` — a Go single-file binary that validates records against the schema, verifies source-file hashes, and regenerates the `index` block. The legacy Python `build-index.py` and `builder-parity-guard.py` scripts retire in this release; the `ulc` CLI is now the authoritative builder. Version `0.5.0` ships author-facing documentation: six per-category starter templates under `templates/` and four PIM platform mapping guides under `mappings/pim/` (Salsify, Akeneo, SAP, custom / in-house). No schema or validator changes in v0.5.0. The ulcspec.org docs site lands in a subsequent batch. The specification will continue to evolve based on real-world use, industry feedback, and alignment with adjacent standards. See `CHANGELOG.md` for release notes.

## Contributing

Contributions are welcome from anyone working with luminaire product data: manufacturers, software vendors, specifiers, trade bodies, and researchers. Read `CONTRIBUTING.md` for how to propose changes, open issues, and submit pull requests. All participation is governed by `CODE_OF_CONDUCT.md`.

## Governance

ULC is stewarded by its originator with input from the industry bodies and collaborators engaged in its development, and is intended to grow into a broadly supported open standard. See `GOVERNANCE.md` for origin, stewardship, decision-making process, and guiding principles.

## License

ULC is published under the MIT License. See `LICENSE`.
