## How ULC works

ULC is an open, machine-readable standard for luminaire cutsheets. A ULC record is a single JSON document that normalizes the information currently spread across a manufacturer's datasheet PDF, IES photometric file, and EULUMDAT (LDT) file into one canonical, verifiable representation. This page walks through how that record is produced, verified, and consumed end to end.

## What a ULC record is

A ULC record is one JSON document that conforms to the ULC schema, carrying a fixture's identity, physical, electrical, optical, photometric, and performance data, its environmental and compliance markings, provenance for every extracted value, and references to the original source files; [README.md](../README.md#what-a-ulc-record-contains) lists the full contents. It sits alongside the manufacturer's existing datasheet, IES, and LDT, which stay published and authoritative, and makes their combined content legible to software without preprocessing a PDF or parsing a decades-old text format.

A single ULC record represents one attested photometric scenario for a luminaire. A product family with multiple distributions, output tiers, or color-rendition variants produces multiple records, one per scenario, each carrying its own measurements and an applicability block that declares which orderable SKU configurations it covers. The reasoning behind that unit is in [methodology.md](methodology.md), and the patterns for mapping real manufacturer documentation into records are in [authoring-patterns.md](authoring-patterns.md).

## How records are authored

A manufacturer does not hand-write ULC JSON. A record is compiled from the documents the manufacturer already publishes by a deterministic tool, so the same inputs always produce the same record. There are two authoring paths, and both are free and offline.

**Gather the source documents first.** Whichever path you use, a ULC record is the union of data across a manufacturer's existing documents. Collect the ones that apply: the marketing cutsheet, the LED-driver cutsheet when the driver ships on its own sheet, the IES (LM-63) photometric file, the EULUMDAT (LDT) file, the installation instructions, the accredited test reports (LM-79, LM-80 with TM-21, LM-82, TM-30, and similar) that lift a record to the higher conformance tiers, and the compliance documents.

You only need the documents your product actually has; a record is graded on what it carries and is never penalized for a document that does not exist. The full mapping of which field comes from which document is the [source-documents reference in methodology.md](methodology.md#the-source-documents).

**Path 1: the free `ulc` compiler (spreadsheet path).** For a manufacturer without a structured data pipeline, the reference `ulc` CLI turns a filled-in spreadsheet into validated records, with no LLM and no cloud step:

1. Copy the blank workbook template at [templates/workbook/](../templates/workbook/) (a `records.csv` plus the related sheets) and transcribe the gathered source data into it, one row per photometric scenario.
2. Run `ulc from-sheet <workbook> --out <dir>`. The converter classifies each row into its authoring pattern, assembles the deep blocks, computes the dual-unit companions and the SHA-256 source-file hashes, builds the index (which grades and stamps both `conformance_level` and `achievements`), and validates each record against the schema before writing it.
3. The output is a finished `.ulc` record per scenario. Because the converter is deterministic, the same workbook always produces byte-identical records.

Download the CLI from the GitHub Releases page, or build it from source with `cd tools/validator && go build -o bin/ulc ./cmd/ulc`.

**Path 2: emit from a PIM (programmatic path).** A manufacturer with catalog-scale output emits ULC directly from its product-information-management system. The [mappings/pim/](../mappings/pim/) guides show how to map Salsify, Akeneo, SAP, and in-house PIMs onto the ULC schema, so records are generated as part of the normal product-data pipeline. The same `ulc validate` and `ulc build-index` commands then verify each emitted record and stamp its index.

**Why the output is trustworthy.** Neither path guesses. The spreadsheet converter and the PIM emitter both produce structured fields directly from the manufacturer's own data, and every record then passes three deterministic checks before it is published: JSON Schema validation, builder parity (the stored index must equal the deterministic projection of the deep blocks), and source-file hash verification. The conformance level and the Product Achievements axis are both computed by the builder, never declared. A published ULC record is therefore accurate by construction: it carries only what the source documents state, with a verifiable hash link back to each one.

## Provenance and integrity

Every value in a ULC record carries provenance: where it came from, how it was obtained, and whether it is measured, rated, or nominal. That makes the source of any field traceable rather than asserted.

ULC does not embed source files. It identifies them. Each referenced source file (datasheet PDF, IES, LDT) is recorded with its filename, an optional URL, and a SHA-256 content hash. A consumer who obtains a source file through any channel can verify it matches the record by recomputing the hash and comparing. If a single byte of the file differs, the hashes diverge. This is tamper-evident by construction, and it is a load-bearing differentiator: the record is not just data, it is data with a verifiable link back to the evidence behind it.

## How records are consumed today

ULC files work with tools people already use, in two tiers that read the same underlying substrate.

Generic large language models parse a ULC record (`.ulc`) natively as JSON. Drag a record from [examples/](../examples/) into ChatGPT, Claude, or Gemini and ask "what is this product?" or "compare this to this other record." The model produces a spec-sheet rendering, a cross-product comparison, or an attribute lookup. No setup is required.

Lighting-domain specialty tools add depth on top of the same record. They fetch the linked IES or LDT photometric files on demand and resolve industry shorthand (terms like L90, TM-21, CCR, SVM, melanopic DER, IES-C, and CIE 97 maintenance-factor tables) without the reader having to know what they mean. One such tool is under development in the ecosystem.

Both tiers read the same ULC record. The difference is depth of lighting-domain support, not whether the data is consumable. ULC is the substrate; the tooling layer is a choice.

## Validation and the two computed views

The reference command-line validator (`ulc`, in `tools/validator/`) checks a record end to end in one command: structure against the JSON Schema, builder parity (that the generated index matches what the builder would produce from the deep blocks), and source-file hashes.

The conformance level is computed and stamped into the generated index as `index.conformance_level` by the builder (`ulc build-index`), never hand-declared; the same builder stamps the second grading axis, Product Achievements, as `index.achievements` (with the `index.restricted_substances_declared` sibling flag). As part of its end-to-end pass, `ulc validate` recomputes the level and checks it against the stored value (builder parity), then reports it. Grading is a cumulative gate: a record is the highest grade all of whose hard requirements it meets. There are three grades above an `incomplete` floor:

- **incomplete**: the floor, a record that has not yet met a core requirement. It grades incomplete, indexes, and carries a roadmap to core; a record missing identity (the schema-required `product_family.manufacturer.slug`, `product_family.manufacturer.display_name`, `product_family.family_id`, and `product_family.catalog_model`) is malformed rather than incomplete: `ulc validate` rejects it against the JSON Schema, and the builder additionally cannot derive the required index keys it projects from identity (`manufacturer_slug`, `catalog_model`), reported through `MissingRequiredKeys`. Either rejection is distinct from incomplete.
- **core**: a complete, identifiable, legally-sellable luminaire with headline numbers and a market safety listing.
- **standard**: core plus the fuller specification a typical LM-79 report produces.
- **full**: standard plus exhaustive accredited characterization, mostly from an accredited test report.

The exact required-field list for each grade is in [the conformance rubric in methodology.md](methodology.md#the-conformance-rubric).

Exit signs and emergency luminaires grade against their own dataset (legend, illumination mode, battery, and UL 924 listing) rather than the architectural-photometry profile, so an exit-sign-only or emergency-only product reaches its honest grade instead of being stranded for lacking a report it never produces; see the [product-class profiles in methodology.md](methodology.md#product-class-profiles-exit-signs-and-emergency-luminaires).

The safety-listing core gate checks for a self-asserted listing claim, not third-party verification of it: a conformance level is a data-completeness grade, never a safety certification (see [methodology.md](methodology.md#the-conformance-rubric)). Compliance beyond the core safety listing never gates the conformance level; instead the third-party program qualifications among the attestations are computed into the second, orthogonal axis, Product Achievements. `ulc validate` reports the grade a record achieves and a per-grade roadmap to full: the grades it already satisfies and, for each grade not yet reached, only that grade's own remaining fields, each naming its source document and governing standard. Alongside that tier roadmap it also emits an enrichment roadmap of optional dimensions a record could disclose to deepen the datasheet without ever changing the grade (hidden from text output unless `--verbose`, always present in JSON). The level is verified from the data, not asserted by the author, and it is never a pass/fail gate. Because the index (including the conformance level and the achievements axis) is a deterministic function of the deep blocks, a hand-edited value fails the builder-parity check like any other index field.

`ulc validate` also reports the Product Achievements axis. Where the conformance level answers "how complete is the datasheet?", achievements answer "what is the product documented for?": per theme (embodied carbon, circularity, material health, energy, dark sky, emergency) the record is `none`, `claimed`, or `documented`, the last only when a qualifying attestation carries an attached, unexpired evidence document. The validator prints a one-line achievements headline by default and the per-theme detail under `--verbose` or in JSON. See [the two grading axes in methodology.md](methodology.md#the-two-grading-axes-completeness-and-achievements).

## How to try it today

Two paths, both available right now:

- **Read a record with an LLM.** Drag any `.ulc` record from [examples/](../examples/) into ChatGPT, Claude, or Gemini and ask it to render the spec sheet, compare two records, or pull out a specific attribute. The eight example records exercise the four manufacturer authoring patterns and the exit-sign/emergency product class.
- **Validate a record with the CLI.** Run `ulc validate <record>` to check structure, builder parity, and source-file hashes and to see the computed conformance level and the default-visible Product Achievements summary. Download a release binary from the GitHub Releases page, or build from source with `cd tools/validator && go build -o bin/ulc ./cmd/ulc`. To regenerate a record's `index` block, run `ulc build-index <record>`; the index is always generated, never hand-authored.

For the design rationale behind the schema, see [methodology.md](methodology.md). For the detailed authoring patterns and architectural primitives, see [authoring-patterns.md](authoring-patterns.md).
