# Tools

This directory contains reference implementations for working with ULC.

These tools are open-source reference implementations intended to verify the specification's correctness and to give manufacturers and implementers a baseline they can use, extend, or replace. Production tooling and ingestion pipelines are expected to live in consumers' own codebases.

## Contents

- `schema-drift-guard.py` is a Python script that loads both schema files and verifies every `$ref` pointer resolves across `taxonomy.schema.json` and `ulc.schema.json`. Run locally from the repo root as `python3 tools/schema-drift-guard.py`. The same script runs in CI on every pull request that touches `schema/**` (see `.github/workflows/schema-drift-guard.yml`). Exits non-zero on any dangling reference.
- `builder-parity-guard.py` verifies that `build-index.py` and `ulc.schema.json` agree on the shape of the `index` block. Catches two classes of drift: the builder emitting a key the schema does not declare, or the schema requiring a key the builder does not produce. Runs in CI alongside the schema drift guard.
- `hooks/pre-commit` is a shipped git pre-commit hook that runs the schema `$ref` drift guard, the builder-schema parity guard, and the record-index parity check before every commit, plus a local research-folder guard. Install it with `cp tools/hooks/pre-commit .git/hooks/pre-commit && chmod +x .git/hooks/pre-commit`. Optional — CI enforces the three schema and index checks on every pull request. The research-folder guard is intentionally local-only because the `research/` path is gitignored and should never reach a commit in the first place.
- `build-index.py` is the canonical builder for a ULC record's `index` block. The index is a denormalized scan surface (manufacturer, catalog, category, nominal CCT, total lumens, input power, BUG, attestation programs, search keywords) that ULC spec forbids hand-authoring. The builder reads a record's deep blocks and deterministically produces the index, stamping it with `x-ulc-generated: true` and the current builder version. Three modes: default (write in place), `--stdout` (print without writing), and `--check` (exit 1 if the record's existing index does not match the builder output). Manufacturer PIMs and authoring tools are expected to call `build-index.py` as the final step before emitting a ULC record.
- `validator/` is a forthcoming command-line validator that checks a ULC record against the JSON Schema, reports structural errors, and verifies that referenced source files match their declared content hashes. Not yet implemented.
- `package-builder/` is a forthcoming command-line utility that assembles a ULC record alongside its source files (datasheet PDF, IES, LDT) into a canonical folder layout, computing content hashes and validating the resulting record. Not yet implemented.

## Language and dependencies

The drift guard and index builder are Python 3 scripts with no external dependencies, so they can run in CI and on any developer machine without a setup step. The forthcoming reference validator and package builder are written in TypeScript and target Node.js 20 or later; AJV is used as the JSON Schema Draft 2020-12 validator. Single-binary distribution is provided where practical so the Node tools can run without a local Node installation.
