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
