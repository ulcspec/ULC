# Tools

This directory contains reference implementations for working with ULC.

These tools are open-source reference implementations intended to verify the specification's correctness and to give manufacturers and implementers a baseline they can use, extend, or replace. Production tooling and ingestion pipelines are expected to live in consumers' own codebases.

## Contents

- `schema-drift-guard.py` is a Python script that loads both schema files and verifies every `$ref` pointer resolves across `taxonomy.schema.json` and `ulc.schema.json`. It also confirms the embedded-schema mirrors under `tools/validator/schema/` stay byte-identical with the canonical `schema/` files, so the `ulc` binary always ships the matching spec version. Run locally from the repo root as `python3 tools/schema-drift-guard.py`. The same script runs in CI on every pull request that touches `schema/**` (see `.github/workflows/schema-drift-guard.yml`). Exits non-zero on any dangling reference or embedded-mirror drift.
- `hooks/pre-commit` is a shipped git pre-commit hook that runs the schema `$ref` drift guard and the record-index parity check before every commit, plus a local research-folder guard. Install it with `cp tools/hooks/pre-commit .git/hooks/pre-commit && chmod +x .git/hooks/pre-commit`. Optional — CI enforces the schema and index checks on every pull request. The research-folder guard is intentionally local-only because the `research/` path is gitignored and should never reach a commit in the first place.
- `validator/` is the reference command-line validator and index builder (ship name: `ulc`). Written in Go and built around [`santhosh-tekuri/jsonschema/v6`](https://github.com/santhosh-tekuri/jsonschema) for JSON Schema Draft 2020-12 validation. The `ulc` binary exposes two subcommands: `ulc build-index <record.ulc>` (canonical index generator, with `--check` and `--stdout` modes) and `ulc validate <record.ulc>` (schema validation, builder parity, source-file SHA-256 hash verification, and a conformance-grading stub; `--json` for machine-readable output). Built-in releases ship platform binaries via GoReleaser. See `tools/validator/README.md` for build instructions and shipped-feature status.
- `package-builder/` is a forthcoming command-line utility that assembles a ULC record alongside its source files (datasheet PDF, IES, LDT) into a canonical folder layout, computing content hashes and validating the resulting record. Not yet implemented.

## Language and dependencies

The schema drift guard is a Python 3 script with no external dependencies, so it can run in CI and on any developer machine without a setup step. The reference validator (and the forthcoming package builder) are written in Go and target Go 1.22 or later; [`santhosh-tekuri/jsonschema/v6`](https://github.com/santhosh-tekuri/jsonschema) is used as the JSON Schema Draft 2020-12 validator. Single-file binaries are shipped via GoReleaser for Linux, macOS, and Windows on x64 and arm64, so the Go tools run without requiring a local Go installation.

The former Python `build-index.py` and `builder-parity-guard.py` scripts retired in v0.4.0 when the Go CLI became authoritative; `ulc build-index` is their replacement, and the builder-schema parity check now runs as a Go unit test (`TestBuilderSchemaParity`). The schema `$ref` drift guard stays in Python indefinitely because it is internal CI tooling and is never shipped externally.
