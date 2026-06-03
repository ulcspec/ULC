## Methodology

This document explains why ULC is shaped the way it is. It sits above [authoring-patterns.md](authoring-patterns.md), which describes the concrete patterns and primitives, and complements [how-it-works.md](how-it-works.md), which walks through ULC end to end for a consumer. Here the concern is the reasoning: the core unit of the standard, the principles that shape the schema, and how the standard evolves.

## The core unit: one attested photometric scenario

A ULC record represents one attested photometric scenario for a luminaire. That definition is deliberate, and it does three things at once.

First, it pins each record to measurement evidence: an LM-79 test, or a declared derivative of one. A record is not a marketing summary; it is a normalized view of measured (or explicitly rated) performance with the provenance to prove it.

Second, it decouples records from orderable SKUs. A manufacturer may produce thousands of SKUs from one cutsheet through a configurator. Tying records to scenarios rather than SKUs means a single record can cover a wide range of orderable configurations through its applicability block, so a cutsheet that spans tens of thousands of theoretical SKUs does not force tens of thousands of ULC files.

Third, it decouples records from IES files. A manufacturer that ships many IES files derived from a smaller base-test set does not need a separate record per file unless the manufacturer chooses to materialize that 1:1 mapping. The scenario, not the file, is the unit.

Two links carry the weight of this decoupling. A structured applicability block connects a record to the orderable SKUs it describes. Provenance metadata on each value connects that value to the measurement evidence behind it. The four authoring patterns (A, B, C, D) show how different manufacturer data models map onto this single unit; see [authoring-patterns.md](authoring-patterns.md) for each pattern in detail rather than a summary here.

## Design principles that shape the schema

Each principle below is a deliberate constraint, with a short rationale for why it earns its place.

**Provenance-first.** Every value traces to its source, and carries a `value_type` of measured, rated, or nominal. This is what lets a consumer distinguish a physically tested figure from a manufacturer-rated projection from a nominal claim awaiting verification. Without it, cross-manufacturer comparison silently mixes evidence of different strengths.

**SI-authoritative dual-unit representation.** Physical quantities are carried in both SI and Imperial units, with SI authoritative. A single fixed policy for which unit governs avoids per-record ambiguity and makes derived scalars deterministic, so two readers extracting the same field always get the same number.

**Closed taxonomy with extensions for the long tail.** Vocabularies live in a closed enum set so that search, classification, and comparison are interoperable across manufacturers. The long tail of manufacturer-specific or experimental data has a defined home in the extensions mechanism, so the core stays comparable without forcing every edge case into a shared enum.

**Generated, not hand-authored, derived data.** The `index` block is a flat, denormalized summary of the most commonly queried values, and it is a deterministic projection of the deep blocks beneath it, produced by the reference builder rather than typed by a manufacturer. The computed conformance level is written into that same index. Because the index is a pure function of the deep blocks, drift is prevented by construction, and a hand-edited index value fails the builder-parity check. This mirrors precedent across the industry, where scan surfaces are generated from tooling rather than hand-authored.

**Metadata-only.** ULC references external standards and source files by identifier and content hash. It does not embed or redistribute the text of paid or restricted standards, and it does not bundle the source files themselves. The record points to evidence; it does not republish it.

**Explicit completeness.** Missing fields are explicit rather than hidden, and the computed conformance level (core, standard, full) makes a record's completeness legible at a glance. When every manufacturer used their own notion of what belongs on a datasheet, gaps were invisible. A common structure where absence is stated, and where the level is computed from the data actually present, surfaces data-quality issues that no single proprietary format ever exposed.

Taken together, these principles serve one goal: records that are comparable across manufacturers, with evidence a consumer can verify and completeness a consumer can read.

## How the standard evolves

ULC changes through the Schema Change Proposal (SCP) process described in `CONTRIBUTING.md` and `GOVERNANCE.md`. A proposal is opened as an issue that states what the change is, why it is needed, which records and tools it affects, and whether it is backward-compatible. After discussion and maintainer feedback, an accepted proposal lands as a pull request that updates the schema, at least one example, the relevant documentation, and the changelog together.

Two practices keep that evolution honest. Changes are grounded in real manufacturer cutsheets: example records are sourced from real spec sheets and real IES files, not invented data, so every field that enters the schema is motivated by a real product or a real workflow gap. And the schema is additive by default. New fields and enums extend the standard without breaking existing records; when a constraint genuinely needs tightening, the change is documented explicitly and chosen so that existing valid records remain valid wherever possible.

The result is a standard that grows from the evidence rather than ahead of it. For the consumer-facing walkthrough, see [how-it-works.md](how-it-works.md); for the patterns and primitives, see [authoring-patterns.md](authoring-patterns.md); for governance and the guiding principles, see `GOVERNANCE.md`.
