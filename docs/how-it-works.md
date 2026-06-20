## How ULC works

ULC is an open, machine-readable standard for luminaire cutsheets. A ULC record is a single JSON document that normalizes the information currently spread across a manufacturer's datasheet PDF, IES photometric file, and EULUMDAT (LDT) file into one canonical, verifiable representation. This page walks through how that record is produced, verified, and consumed end to end.

## What a ULC record is

A ULC record is one JSON document that conforms to the ULC schema. It carries product identity, family, and taxonomy; physical dimensions in both SI and Imperial units; electrical, optical, photometric, and performance data; environmental ratings, compliance markings, and accessories; provenance for every extracted value; and references to the original source files.

The record sits alongside the manufacturer's existing files, it does not replace them. The datasheet, IES, and LDT remain published exactly as they always have been. ULC provides a normalized, machine-readable representation of their combined content, so an AI agent, design tool, or software system can consume the data directly without preprocessing a PDF or parsing a decades-old text format. The originals stay authoritative; the ULC record makes their combined content legible to software.

A single ULC record represents one attested photometric scenario for a luminaire. A product family with multiple distributions, output tiers, or color-rendition variants produces multiple records, one per scenario, each carrying its own measurements and an applicability block that declares which orderable SKU configurations it covers. The reasoning behind that unit is in [methodology.md](methodology.md), and the patterns for mapping real manufacturer documentation into records are in [authoring-patterns.md](authoring-patterns.md).

## How records are authored

A manufacturer does not hand-write ULC JSON. A record is compiled from the documents the manufacturer already publishes by a deterministic tool, so the same inputs always produce the same record. There are two authoring paths, and both are free and offline.

**Gather the source documents first.** Whichever path you use, a ULC record is the union of data across a manufacturer's existing documents. Collect the ones that apply to the product:

- the marketing cutsheet (identity, mechanical, environmental, and the headline rated specs),
- the LED-driver cutsheet, when the driver is specified on its own sheet (the electrical detail, dimming protocol, and dimming method),
- the IES (LM-63) photometric file (the candela distribution and the absolute electrical anchors),
- the EULUMDAT (LDT) file, if the manufacturer publishes one,
- the installation instructions (the deep mounting, wiring, and recess geometry the cutsheet only summarizes),
- the accredited test reports (LM-79, LM-80 with TM-21, LM-82, TM-30, and similar), which carry the measurement uncertainty, corrections, and depth that lift a record to the higher conformance tiers,
- the compliance documents (safety listings, sustainability and origin declarations).

You only need the documents your product actually has; a record is graded on what it carries and is never penalized for a document that does not exist. The full mapping of which field comes from which document is the [source-documents reference in methodology.md](methodology.md#the-source-documents).

**Path 1: the free `ulc` compiler (spreadsheet path).** For a manufacturer without a structured data pipeline, the reference `ulc` CLI turns a filled-in spreadsheet into validated records, with no LLM and no cloud step:

1. Copy the blank workbook template at [templates/workbook/](../templates/workbook/) (a `records.csv` plus the related sheets) and transcribe the gathered source data into it, one row per photometric scenario.
2. Run `ulc from-sheet <workbook> --out <dir>`. The converter classifies each row into its authoring pattern, assembles the deep blocks, computes the dual-unit companions and the SHA-256 source-file hashes, builds the index (which grades and stamps `conformance_level`), and validates each record against the schema before writing it.
3. The output is a finished `.ulc` record per scenario. Because the converter is deterministic, the same workbook always produces byte-identical records.

Download the CLI from the GitHub Releases page, or build it from source with `cd tools/validator && go build -o bin/ulc ./cmd/ulc`.

**Path 2: emit from a PIM (programmatic path).** A manufacturer with catalog-scale output emits ULC directly from its product-information-management system. The [mappings/pim/](../mappings/pim/) guides show how to map Salsify, Akeneo, SAP, and in-house PIMs onto the ULC schema, so records are generated as part of the normal product-data pipeline. The same `ulc validate` and `ulc build-index` commands then verify each emitted record and stamp its index.

**Why the output is trustworthy.** Neither path guesses. The spreadsheet converter and the PIM emitter both produce structured fields directly from the manufacturer's own data, and every record then passes three deterministic checks before it is published: JSON Schema validation, builder parity (the stored index must equal the deterministic projection of the deep blocks), and source-file hash verification. The conformance level is computed by the builder, never declared. A published ULC record is therefore accurate by construction: it carries only what the source documents state, with a verifiable hash link back to each one.

## Provenance and integrity

Every value in a ULC record carries provenance: where it came from, how it was obtained, and whether it is measured, rated, or nominal. That makes the source of any field traceable rather than asserted.

ULC does not embed source files. It identifies them. Each referenced source file (datasheet PDF, IES, LDT) is recorded with its filename, an optional URL, and a SHA-256 content hash. A consumer who obtains a source file through any channel can verify it matches the record by recomputing the hash and comparing. If a single byte of the file differs, the hashes diverge. This is tamper-evident by construction, and it is a load-bearing differentiator: the record is not just data, it is data with a verifiable link back to the evidence behind it.

## How records are consumed today

ULC files work with tools people already use, in two tiers that read the same underlying substrate.

Generic large language models parse a ULC record (`.ulc`) natively as JSON. Drag a record from [examples/](../examples/) into ChatGPT, Claude, or Gemini and ask "what is this product?" or "compare this to this other record." The model produces a spec-sheet rendering, a cross-product comparison, or an attribute lookup. No setup is required.

Lighting-domain specialty tools add depth on top of the same record. They fetch the linked IES or LDT photometric files on demand and resolve industry shorthand (terms like L90, TM-21, CCR, melanopic DER, and CIE 97 maintenance-factor tables) without the reader having to know what they mean. One such tool is under development in the ecosystem.

Both tiers read the same ULC record. The difference is depth of lighting-domain support, not whether the data is consumable. ULC is the substrate; the tooling layer is a choice.

## Validation and the computed conformance level

The reference command-line validator (`ulc`, in `tools/validator/`) checks a record end to end in one command: structure against the JSON Schema, builder parity (that the generated index matches what the builder would produce from the deep blocks), and source-file hashes.

The conformance level is computed and stamped into the generated index as `index.conformance_level` by the builder (`ulc build-index`), never hand-declared. As part of its end-to-end pass, `ulc validate` recomputes the level and checks it against the stored value (builder parity), then reports it. Grading is a cumulative gate: a record is the highest tier all of whose hard requirements it meets. The four levels are:

- **incomplete**: a real photometric record (it carries the flux, input-power, and primary-category anchors) that has not yet met every core requirement. Those anchors make the record gradeable at this tier and earn it a roadmap naming the core fields it is missing; a fully valid index also requires the identity core-fields (`manufacturer_slug` and `catalog_model`), which the builder reports through `MissingRequiredKeys` when they are absent.
- **core**: a complete, identifiable, legally-sellable luminaire: full identity, headline photometric and electrical numbers, one-line colorimetry, and a market safety listing.
- **standard**: core plus the fuller specification a typical LM-79 report produces (full photometric geometry, materials, an LM-79 attestation, a lumen-maintenance framework, and the white-light, directional, outdoor-site, and wet-location conditionals that apply).
- **full**: standard plus exhaustive accredited characterization (zonal lumens, an operating point, measurement uncertainty, corrections, instrumentation depth, a method-backed lumen-maintenance projection, and TM-30 detail for white-light products), mostly from an accredited test report.

The safety-listing core gate checks for the presence of a self-asserted listing claim, not third-party verification of it; a conformance level is a data-completeness grade, never a safety certification. Compliance beyond the core safety listing is tracked, not gated. `ulc validate` reports the level a record achieves and the specific fields that would raise it to the next level, each naming its source document and governing standard. The level is verified from the data, not asserted by the author, and it is never a pass/fail gate. Because the index (including the conformance level) is a deterministic function of the deep blocks, a hand-edited value fails the builder-parity check like any other index field.

## How to try it today

Two paths, both available right now:

- **Read a record with an LLM.** Drag any `.ulc` record from [examples/](../examples/) into ChatGPT, Claude, or Gemini and ask it to render the spec sheet, compare two records, or pull out a specific attribute. The example records each exercise one of the four manufacturer authoring patterns.
- **Validate a record with the CLI.** Run `ulc validate <record>` to check structure, builder parity, and source-file hashes and to see the computed conformance level. Download a release binary from the GitHub Releases page, or build from source with `cd tools/validator && go build -o bin/ulc ./cmd/ulc`. To regenerate a record's `index` block, run `ulc build-index <record>`; the index is always generated, never hand-authored.

For the design rationale behind the schema, see [methodology.md](methodology.md). For the detailed authoring patterns and architectural primitives, see [authoring-patterns.md](authoring-patterns.md).
