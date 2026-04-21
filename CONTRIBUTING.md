# Contributing to ULC

Thank you for your interest in contributing. ULC is an open specification, and its value grows with input from across the lighting industry: manufacturers, software vendors, specifiers, trade bodies, and anyone working with luminaire product data.

This document describes how to participate.

## Ways to contribute

You do not need to be a developer to contribute. Valuable contributions include:

- Opening issues to report ambiguities, gaps, or unclear language in the specification
- Suggesting new fields, enumerations, or taxonomy entries grounded in real-world products
- Sharing use cases and workflows that ULC should support
- Proposing mappings to adjacent standards (GLDF, ETIM, IES, LDT)
- Submitting pull requests for documentation improvements, example records, or tooling
- Reviewing open pull requests and issues

## Before opening an issue

1. Search existing issues to see if the topic is already under discussion.
2. If the topic is new, open an issue describing the problem or proposal clearly. For specification changes, use the "Schema Change Proposal" template.
3. For questions or open-ended discussion, prefer Discussions over Issues if enabled.

## Submitting a pull request

1. Keep the scope of each pull request focused. One logical change per PR is easier to review and merge.
2. Explain the purpose of the change in the PR description, including the motivation and any tradeoffs considered.
3. Update related documentation, examples, and mappings when you change the schema.
4. Ensure that examples still validate against the schema. The CI workflow will check this automatically.
5. Avoid unrelated edits, formatting-only changes mixed with substantive changes, or large refactors without prior discussion.
6. The `index` block on every ULC record is **generated, not hand-authored**. Do not edit index values directly. If a deep-block field changes, re-run `python3 tools/build-index.py <record>.ulc` to regenerate the index before committing. The pre-commit hook and CI will reject records whose index does not match the builder output.

## What belongs in ULC

ULC is a fixture metadata specification. Every field in the schema describes the luminaire itself, not the road, site, building, or lighting design project where the luminaire is installed. Before proposing a new field, please confirm that it falls into at least one of these five categories:

1. **Intrinsic property of the fixture** — physical construction, materials, optics geometry, dimensions, emission characteristics.
2. **Declared capability of the fixture** — dimming protocols, network interfaces, adaptive control modes, operating conditions the fixture supports.
3. **Attestation earned through testing** — LM-79 results, BUG rating, DLC listing, UL or CE compliance, along with the test context (stabilization method, ambient condition flags, temperature monitoring point) that makes the attestation meaningful.
4. **Derivable from fixture data** — zonal lumens from the IES file, Rf and Rg from an SPD, beam angle from the candela distribution, symmetry type from the photometric grid.
5. **Record-level metadata about the ULC document itself** — conformance level, unit system declaration, source-file format, provenance pointers, record lifecycle status.

If a proposed field requires knowing something a manufacturer cannot authoritatively publish — the pavement reflectance class of the road, the pedestrian activity level of the site, the mounting height and pole spacing of the installation, the specific design criteria for a jurisdiction — then it belongs in a lighting-design tool, a project specification, or a separate standard, not in ULC.

This filter exists because ULC's value depends on records being comparable across manufacturers. Site and design context varies installation-by-installation; fixture properties do not. Pulling design context into ULC would dilute that comparability and duplicate work that already belongs to other specifications (ANSI/IES RP-8, DIALux, AGi32, ETIM attribute groups for application context, and so on).

Field proposals that pass the rubric should still come with a citation to the standards or real-world products that motivate them.

## Schema change proposals

Changes to the schema, taxonomy, or package specification have downstream effects on every ULC implementer. For any material change:

1. Open a Schema Change Proposal issue using the provided template.
2. Describe what the change is, why it is needed, what records or tools it affects, and whether it is backward-compatible.
3. Wait for discussion and maintainer feedback before submitting code.
4. If accepted in principle, submit a PR that updates the schema, at least one example, the relevant documentation, and the changelog.

## Contribution principles

Contributions should be:

- Clear, with enough context that a future reader understands the reasoning
- Specific, addressing one concern rather than many
- Technically grounded, referencing real products, real standards, or real workflows
- Aligned with the scope of ULC as an open luminaire datasheet specification
- Respectful of licensing. ULC does not redistribute the text of paid or restricted standards. Reference and link to them instead.

## Licensing of contributions

By submitting a contribution to this repository, you agree that your contribution is licensed under the same terms as the project (see `LICENSE`).

## Code of conduct

All participation is governed by the `CODE_OF_CONDUCT.md`. Please read it before engaging.

## Governance

Project direction, maintainership, and decision-making are described in `GOVERNANCE.md`.
