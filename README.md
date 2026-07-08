<p align="center">
  <img src="assets/logo.png" alt="ULC - Universal Luminaire Cutsheet" width="400" />
</p>

# ULC

**Universal Luminaire Cutsheet**

> "Cutsheet" in the name is the North American term for what is globally called a **datasheet**, the manufacturer's published technical summary of a luminaire. This document uses "datasheet" as the primary term in body text; both refer to the same document type.

**Open machine-readable standard for luminaire cutsheets.** A ULC file is a single JSON document that normalizes the information currently spread across manufacturer datasheets, IES photometric files, and EULUMDAT (LDT) files into one canonical record. ULC does not replace those source files. It provides a normalized representation of their combined content that AI systems, design tools, specification software, and data pipelines can consume directly, with verifiable references back to the originals.

**A single ULC record represents one attested photometric scenario for a luminaire.** A product family with multiple distributions, output tiers, or color-rendition variants produces multiple ULC records, one per scenario, each carrying its own measurements and an applicability block that declares which orderable SKU configurations the record covers.

ULC is stewarded by lighting professionals, for everyone who reads a luminaire datasheet during their work.

## Why ULC exists

The way designers search for product information has fundamentally shifted in the past few years in the wake of AI. Like every other industry, lighting has to adapt to benefit from what AI tools now make possible. ULC exists to close the gap between how designers used to specify luminaires and how they need to specify them today using AI tools.

Luminaire product data today is fragmented across thousands of PDF datasheets, each formatted differently, alongside decades-old text-based photometric files with inconsistent conventions.

For a lighting designer, that fragmentation shows up as:

- Six hours manually cross-referencing CRI / CCT / wattage / accessory pairings across twelve PDFs to compare fixtures for a single luminaire-schedule line
- Reviewing the same VE package for the third time, line by line, against the original design intent
- Building a personal or firm-level reference library that drifts the moment manufacturers update their products
- Leaning on manufacturer rep relationships as the human bottleneck for data accuracy

For a manufacturer, that fragmentation shows up as data discoverability eroding to whatever an AI's PDF-extraction layer guesses. As specifier discovery shifts from "open the PDF" to "ask an AI which luminaire fits," the manufacturers whose data is machine-readable will be the ones AI systems surface and design tools consume natively. The bottleneck is the data layer, not the AI layer.

ULC closes the gap. The datasheet, IES, and LDT remain where they are, published as they always have been. The ULC file provides a normalized, machine-readable representation of their combined content, ready for any AI agent, design tool, or software system to consume without preprocessing.

A universal schema also surfaces data quality issues that are invisible today. When every manufacturer uses their own taxonomy and their own notion of what belongs on a datasheet, cross-manufacturer comparison is unreliable by default. ULC defines a common structure where missing fields are explicit rather than hidden, which benefits every consumer of the data.

## What a ULC record contains

A ULC record is a single JSON document that conforms to the ULC schema. It carries:

- Product identity, family, and taxonomy
- Physical dimensions in both SI and Imperial units
- Electrical, optical, photometric, and performance data
- Environmental ratings, compliance markings, and accessories
- Provenance for every extracted value, so the source of each field is always traceable
- References to the original source files (datasheet PDF, IES, LDT) including filename, optional URL, and a SHA-256 content hash for integrity verification

ULC does not embed source files. It identifies them. A consumer who obtains a source file through any channel can verify it matches the ULC record by comparing hashes.

Every record also carries a computed **conformance grade** (`core`, `standard`, or `full`, above an `incomplete` floor), graded by the reference builder from the fields the record populates and stamped into the generated index, never declared by the author. The grading rubric applies its requirements conditionally through a layer of applicability predicates, so a fixture is only ever asked for the data its form actually has: a pure color-mixing fixture is not graded on CRI, and an indoor downlight is not graded on a BUG rating. The tiers are not arbitrary: they mirror the way a construction specification escalates what it asks of a manufacturer, from the mandatory product data and safety listing every fixture must provide, through the selection-grade performance specifications used to compare products, to the independently-certified test reports demanded of the most rigorous fixtures. Alongside the grade, the validator emits a two-part roadmap: the tier gaps a record must close to climb, and a non-gating enrichment roadmap of optional dimensions it could disclose to deepen the datasheet. The three grades above an incomplete floor, the per-grade requirement tables, the predicate layer, and the rationale behind the grade cut points are documented in [the conformance rubric](docs/methodology.md#the-conformance-rubric).

## Source inputs

ULC is designed around the three source types realistically available across the industry today:

- PDF datasheets
- IES photometric files (ANSI/IES LM-63)
- EULUMDAT photometric files (LDT)

Additional source types and fields may be supported in future versions.

## Who uses ULC

**Lighting designers, interior designers, and architects who specify lighting fixtures** consume ULC data through tools that read it. Apples-to-apples cutsheet comparison, efficient fixture search, VE package review in seconds rather than hours, automatic luminaire schedule generation from a list of ULC files; all of these become available as soon as the manufacturers a designer specs most often publish ULC. The activation mechanic is direct: as designers ask their preferred manufacturer reps for ULC files, manufacturers publish them.

**Manufacturers** publish ULC files alongside their datasheet PDFs, IES, and LDT files on their product pages. Their products surface in AI-mediated specifier discovery with accurate attributes, not whatever an extraction layer guessed off the PDF. They own the data relationship at the data layer; existing commercial relationships (reps, distributors, agencies, aggregators) stay where they are.

**Software vendors and AI systems** implement readers, writers, and validators against the ULC schema. The reference CLI validator (`ulc`, at `tools/validator/`) packages JSON Schema validation, index-builder parity, and source-file hash verification into one command, shipped as a single-file binary. Any JSON Schema Draft 2020-12 library can also validate ULC records against the schemas directly. General-purpose AI assistants (ChatGPT, Claude, Gemini) parse ULC files natively as JSON; domain-specific agents (LightingAgent.AI) consume ULC with lighting-domain-tuned depth.

## How to consume ULC today

ULC files work with the tools designers already use, today, without waiting for ecosystem maturity:

- **Generic LLMs (ChatGPT, Claude, Gemini)** parse a ULC file natively. Drag a `.ulc` record from `examples/` into the chat and ask "what is this product?" or "compare this to [another ULC file]." The LLM produces a useful spec-sheet rendering, cross-product comparison, and attribute lookup. No setup required.
- **Lighting-domain specialty consumption tools** like LightingAgent.AI (under development today) add on-demand fetching of the linked IES / LDT photometric files and built-in resolution for industry-shorthand glossary (L90, TM-21, CCR, SVM, melanopic DER, IES-C, CIE 97 maintenance-factor tables).

Both tiers work. The difference is depth of lighting-domain support, not whether ULC is consumable. ULC is the substrate that both read.

This repository defines the standard. It does not ship an application.

## Repository structure

| Path | Contents |
|---|---|
| `schema/` | Two JSON Schema files (Draft 2020-12): `ulc.schema.json` defines the record structure; `taxonomy.schema.json` defines the closed-enum vocabulary. They are split so the taxonomy can be loaded independently by search and classification tools. Cross-file references are validated in CI. |
| `docs/` | Narrative documentation: `how-it-works.md` (end-to-end overview), `methodology.md` (design rationale), and `authoring-patterns.md` (the four manufacturer authoring patterns and architectural primitives). |
| `examples/` | Canonical reference ULC records, one per manufacturer authoring pattern (A/B/C/D), drafted from real spec sheets and IES files. Source files are referenced by required SHA-256 hash and optional URL, not committed. Reserved for vetted canonical records; authors writing new records should keep their in-progress files out of this directory. |
| `templates/` | The fill-in workbook template (`workbook/`: a `records.csv` plus related sheets) for the deterministic `ulc from-sheet` converter. See [How records are authored](docs/how-it-works.md#how-records-are-authored). |
| `mappings/` | Two kinds. Adjacent standards: planned crosswalks to GLDF and ETIM plus guidance for parsing IES and LDT sources. PIM platforms: how to emit ULC records at catalog scale from Salsify, Akeneo, SAP, or an in-house PIM (`mappings/pim/`). |
| `tools/` | Reference utilities: the schema drift guard (`schema-drift-guard.py`, Python) and the `ulc` reference command-line validator at `tools/validator/` (Go, ships as a single-file binary via GoReleaser). |
| `.github/` | Issue templates, pull request template, and continuous integration (including the schema drift guard workflow). |

## Getting started

The current working state ships the schema, taxonomy, drift-guard tooling, the authoring-patterns document, five canonical reference records covering the four manufacturer authoring patterns, the reference command-line validator and compiler (`ulc`) at `tools/validator/` with schema validation, builder parity, source-file hash verification, and the deterministic `from-sheet` converter, the fill-in workbook template under `templates/workbook/`, and PIM platform mapping guides under `mappings/pim/`. The [ulcspec.org](https://ulcspec.org) narrative docs site is the public-facing companion to this repository.

- To understand the data model, read `docs/authoring-patterns.md`. It describes the four manufacturer authoring patterns ULC supports and the architectural primitives (product family, configuration, applicability, generated index, provenance classes, conditional attestations).
- To see those patterns in real data, read the records in `examples/`, which exercise the four authoring patterns against real manufacturer spec sheets.
- **To try ULC right now**, drag any `.ulc` record from `examples/` into ChatGPT, Claude, or Gemini and ask it to render the spec sheet, compare two records, or pull out a specific attribute. No setup required.
- To explore the schema directly, read `schema/ulc.schema.json` for the record structure and `schema/taxonomy.schema.json` for the closed vocabularies.
- To implement ULC in your own software, reference those two schema files by URL and use any JSON Schema Draft 2020-12 validator. The `tools/schema-drift-guard.py` script shows how `$ref`s resolve across the split.
- To validate a record end-to-end (schema, index-builder parity, source-file hashes), run `ulc validate <record>` using the reference CLI. `<record>` is the path to any `.ulc` or `.ulc.json` file; both extensions are accepted. Download a release binary from the GitHub Releases page, or build from source with `cd tools/validator && go build -o bin/ulc ./cmd/ulc`.
- To regenerate a record's `index` block, run `ulc build-index <record>`. The index is always generated, never hand-authored.

## Relationship to adjacent standards

ULC is designed to cooperate with, not replace, existing work in the lighting data ecosystem. ULC has no direct competitor; it is the first open, lightweight, machine-readable cutsheet standard for the lighting industry. The standards below are companion layers ULC is designed to interoperate with.

- **GLDF** (Global Lighting Data Format) is the primary interchange container for the DIALux and RELUX planning ecosystems. ULC and GLDF address different problems at different layers: GLDF is a rich XML-based container optimized for photometric planning software; ULC is a lightweight JSON specification optimized for structured datasheet data and AI consumption. A manufacturer can publish both. A field-level mapping between ULC and GLDF is planned at `mappings/gldf-crosswalk.md`.
- **ETIM** (ElectroTechnical Information Model) provides a widely adopted classification vocabulary for product attributes in electrotechnical wholesale. The ULC taxonomy's enum descriptions cite the relevant ETIM feature identifiers inline where a direct correspondence exists (for example `EF001596` on housing material); a compiled crosswalk at `mappings/etim-crosswalk.md` is planned.
- **IES LM-63** and **EULUMDAT** remain the photometric data formats that feed ULC. ULC does not duplicate or replace their content. A guide for extracting ULC field values from IES and LDT files is planned at `mappings/photometric-source-parsing.md`.

ULC does not redistribute the text of any paid or restricted standards. It references them by identifier.

## Adoption status

Adoption is in its early days. Five canonical reference records are published in `examples/`, each derived from a real manufacturer spec sheet:

- ERCO Quintessence: recessed downlight
- Vode Nexa: suspended linear pendant
- Selux Aya: exterior pole
- Lumenpulse Lumenfacade: RGB and RGBW inground façade (two records)

Each exercises a distinct manufacturer authoring pattern (A/B/C/D).

To check the current state of manufacturer adoption (which manufacturers have published ULC files for which products), visit **[ulcspec.org](https://ulcspec.org)**, the public adoption registry and documentation surface.

## Project status

The current release is `0.10.0`. Conformance is computed rather than declared: the reference builder grades each record against three grades (`core`, `standard`, `full`) above an `incomplete` floor and stamps the result into the generated index. The toolchain ships the split schema and taxonomy, the drift-guard tooling, the reference command-line validator and compiler (`ulc`) with schema validation, builder parity, and source-file hash verification, the deterministic `from-sheet` workbook-to-record converter, the fill-in workbook template under `templates/workbook/`, and the PIM platform mapping guides under `mappings/pim/`. See `CHANGELOG.md` for the full release history.

The specification will continue to evolve based on real-world use, industry feedback, and alignment with adjacent standards. See `CHANGELOG.md` for release notes.

## Contributing

Contributions are welcome from anyone working with luminaire product data: lighting designers, interior designers, architects, manufacturers, software vendors, trade bodies, and researchers. The Schema Change Proposal (SCP) process is documented in `CONTRIBUTING.md`. Anyone can propose schema changes from a real workflow gap on GitHub, and the community decides. Read `CONTRIBUTING.md` for how to propose changes, open issues, and submit pull requests. All participation is governed by `CODE_OF_CONDUCT.md`.

## Governance

ULC is governed openly: schema decisions flow through the Schema Change Proposal process on GitHub; the community contributes through pull requests, issues, and SCPs; engagement with adjacent industry bodies (DIAL, IES, IALD, LIA) and companion-format communities (GLDF, ETIM) is ongoing. See `GOVERNANCE.md` for origin, decision-making process, and guiding principles. The intent is to grow into a broadly supported open standard.

## Steward

The ULC framework was introduced by Foad Shafighi, MIES, IALD, CLD, at IALD Enlighten Europe 2025 in Valencia, and has since been developed through dialogue with the industry, including following the National Lighting Bureau AI Think Tank in New York in 2026.

## License

ULC is published under the MIT License. See `LICENSE`.
