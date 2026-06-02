## How ULC works

ULC is an open, machine-readable standard for luminaire cutsheets. A ULC record is a single JSON document that normalizes the information currently spread across a manufacturer's datasheet PDF, IES photometric file, and EULUMDAT (LDT) file into one canonical, verifiable representation. This page walks through how that record is produced, verified, and consumed end to end.

## What a ULC record is

A ULC record is one JSON document that conforms to the ULC schema. It carries product identity, family, and taxonomy; physical dimensions in both SI and Imperial units; electrical, optical, photometric, and performance data; environmental ratings, compliance markings, and accessories; provenance for every extracted value; and references to the original source files.

The record sits alongside the manufacturer's existing files, it does not replace them. The datasheet, IES, and LDT remain published exactly as they always have been. ULC provides a normalized, machine-readable representation of their combined content, so an AI agent, design tool, or software system can consume the data directly without preprocessing a PDF or parsing a decades-old text format. The originals stay authoritative; the ULC record makes their combined content legible to software.

A single ULC record represents one attested photometric scenario for a luminaire. A product family with multiple distributions, output tiers, or color-rendition variants produces multiple records, one per scenario, each carrying its own measurements and an applicability block that declares which orderable SKU configurations it covers. The reasoning behind that unit is in [methodology.md](methodology.md), and the patterns for mapping real manufacturer documentation into records are in [authoring-patterns.md](authoring-patterns.md).

## Provenance and integrity

Every value in a ULC record carries provenance: where it came from, how it was obtained, and whether it is measured, rated, or nominal. That makes the source of any field traceable rather than asserted.

ULC does not embed source files. It identifies them. Each referenced source file (datasheet PDF, IES, LDT) is recorded with its filename, an optional URL, and a SHA-256 content hash. A consumer who obtains a source file through any channel can verify it matches the record by recomputing the hash and comparing. If a single byte of the file differs, the hashes diverge. This is tamper-evident by construction, and it is a load-bearing differentiator: the record is not just data, it is data with a verifiable link back to the evidence behind it.

## How records are consumed today

ULC files work with tools people already use, in two tiers that read the same underlying substrate.

Generic large language models parse a `.ulc.json` natively as JSON. Drag a record from [examples/](../examples/) into ChatGPT, Claude, or Gemini and ask "what is this product?" or "compare this to this other record." The model produces a spec-sheet rendering, a cross-product comparison, or an attribute lookup. No setup is required.

Lighting-domain specialty tools add depth on top of the same record. They fetch the linked IES or LDT photometric files on demand and resolve industry shorthand (terms like L90, TM-21, CCR, melanopic DER, and CIE 97 maintenance-factor tables) without the reader having to know what they mean. One such tool is under development in the ecosystem.

Both tiers read the same ULC record. The difference is depth of lighting-domain support, not whether the data is consumable. ULC is the substrate; the tooling layer is a choice.

## Validation and the computed conformance level

The reference command-line validator (`ulc`, in `tools/validator/`) checks a record end to end in one command: structure against the JSON Schema, builder parity (that the generated index matches what the builder would produce from the deep blocks), and source-file hashes.

As part of that pass, the validator computes the record's conformance level from the data the record actually carries, and that level is stamped into the generated index as `index.conformance_level`. It is never hand-declared. The three levels are:

- **core**: the minimum identifying and photometric dataset.
- **standard**: what a typical LM-79 test report produces.
- **full**: adds the comprehensive set (TM-30 hue bins, lumen-maintenance projections, BUG for outdoor products, measurement uncertainty, operating-point qualifiers, and instrumentation metadata) where applicable to the product.

`ulc validate` reports the level a record achieves and, below `full`, the specific fields that would raise it to the next level. The level is verified from the data, not asserted by the author, and it is never a pass/fail gate. Because the index (including the conformance level) is a deterministic function of the deep blocks, a hand-edited value fails the builder-parity check like any other index field.

## How to try it today

Two paths, both available right now:

- **Read a record with an LLM.** Drag any `.ulc.json` from [examples/](../examples/) into ChatGPT, Claude, or Gemini and ask it to render the spec sheet, compare two records, or pull out a specific attribute. The four example records each exercise a distinct manufacturer authoring pattern.
- **Validate a record with the CLI.** Run `ulc validate <record>` to check structure, builder parity, and source-file hashes and to see the computed conformance level. Download a release binary from the GitHub Releases page, or build from source with `cd tools/validator && go build -o bin/ulc ./cmd/ulc`. To regenerate a record's `index` block, run `ulc build-index <record>`; the index is always generated, never hand-authored.

For the design rationale behind the schema, see [methodology.md](methodology.md). For the detailed authoring patterns and architectural primitives, see [authoring-patterns.md](authoring-patterns.md).
