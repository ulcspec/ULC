<p align="center">
  <img src="assets/logo.png" alt="ULC - Universal Luminaire Cutsheet" width="400" />
</p>

# ULC

**Universal Luminaire Cutsheet**

> "Cutsheet" in the name is the North American term for what is globally called a **datasheet**, the manufacturer's published technical summary of a luminaire. This document uses "datasheet" as the primary term in body text; both refer to the same document type.

ULC is an open specification for structured, machine-readable luminaire product data. A ULC file is a single JSON document that normalizes the information currently spread across manufacturer datasheets, IES photometric files, and EULUMDAT (LDT) files into one consistent, canonical record.

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
| Software vendors | Implement readers, writers, and validators against the ULC schema. Use the reference validator in this repository to confirm conformance. |
| AI agents and assistants | Parse ULC files directly to answer product queries, compare alternates, generate documentation, and automate design tasks. Any JSON-aware system can consume the format, including general-purpose assistants such as ChatGPT and Claude and domain-specific agents such as LightingAgent.AI. |

Product discoverability is shifting. General-purpose search engines that once indexed PDF datasheets are being supplemented, and in some workflows replaced, by AI agents that retrieve and compare product data on the user's behalf. Machine-readable product data that can be parsed directly, without PDF extraction, is the input AI agents prefer. Manufacturers who publish ULC records put their products in reach of that new retrieval path.

This repository defines the standard. It does not ship an application.

## Repository structure

| Path | Contents |
|---|---|
| `schema/` | JSON Schema files that define the ULC record and taxonomy |
| `docs/` | Narrative specification, field reference, and authoring guide |
| `examples/` | Worked examples of conforming ULC records with reference source files |
| `templates/` | Starter templates for authors |
| `mappings/` | Crosswalks to GLDF and ETIM, plus guidance for parsing IES and LDT sources |
| `tools/` | Reference validator |
| `.github/` | Issue templates, pull request template, and continuous integration |

## Getting started

Detailed guides are in `docs/`. A brief map:

- To understand ULC as a reader, start with `docs/introduction.md`.
- To author a ULC record, read `docs/creation-guide.md` and copy `templates/ulc.template.json`.
- To validate a record, run the CLI in `tools/validator/` against your JSON file.
- To implement ULC in your own software, read `docs/specification.md` and reference the schemas in `schema/`.

## Relationship to adjacent standards

ULC is designed to cooperate with, not replace, existing work in the lighting data ecosystem:

- **GLDF** (Global Lighting Data Format) is the primary interchange container for the DIALux and RELUX planning ecosystems. ULC and GLDF address different problems: GLDF is a rich XML-based container optimized for photometric planning software, while ULC is a lightweight JSON specification optimized for structured datasheet data and AI consumption. This repository provides a field-level mapping between ULC and GLDF in `mappings/gldf-crosswalk.md`. Future tooling could generate GLDF output from ULC records, and vice versa, although neither direction is implemented as part of this repository at v0.1.
- **ETIM** (ElectroTechnical Information Model) provides a widely adopted classification vocabulary for product attributes in electrotechnical wholesale. Where ETIM codes apply to luminaire fields, ULC documents the corresponding ETIM feature identifiers in `mappings/etim-crosswalk.md`.
- **IES LM-63** and **EULUMDAT** remain the photometric data formats that feed ULC. ULC does not duplicate or replace their content. Guidance for extracting ULC field values from IES and LDT files is documented in `mappings/photometric-source-parsing.md`.

ULC does not redistribute the text of any paid or restricted standards. It references them by identifier.

## Project status

Version `0.1.0` establishes the foundation of the specification: schema, narrative documentation, examples, mappings, and reference validator. The specification will continue to evolve based on real-world use, industry feedback, and alignment with adjacent standards. See `CHANGELOG.md` for release notes.

## Contributing

Contributions are welcome from anyone working with luminaire product data: manufacturers, software vendors, specifiers, trade bodies, and researchers. Read `CONTRIBUTING.md` for how to propose changes, open issues, and submit pull requests. All participation is governed by `CODE_OF_CONDUCT.md`.

## Governance

ULC is stewarded by its originator with input from the industry bodies and collaborators engaged in its development, and is intended to grow into a broadly supported open standard. See `GOVERNANCE.md` for origin, stewardship, decision-making process, and guiding principles.

## License

ULC is published under the MIT License. See `LICENSE`.
