# `ulc` — reference validator for ULC

The `ulc` command-line tool is the reference implementation of the ULC specification's validation and index-building logic. It is the authoritative check that a `.ulc` file is well-formed, conforms to the schema, and has a correctly-computed `index` block.

## Status

Batch 4 is in progress. Current state:

- [x] `ulc build-index <record.ulc>` (port of `tools/build-index.py`)
- [x] `ulc build-index <record.ulc> --check`
- [x] `ulc build-index <record.ulc> --stdout`
- [ ] `ulc validate <record.ulc>` — stub now, wires in full JSON Schema Draft 2020-12 validation later in Batch 4 using [`santhosh-tekuri/jsonschema/v6`](https://github.com/santhosh-tekuri/jsonschema)
- [ ] Conformance grading (`ERROR` / `WARNING` / `INFO` classes per declared `conformance_level`)
- [ ] Source-file hash verification against declared `sha256` values
- [ ] `--json` machine-readable output mode
- [ ] Single-file binaries via goreleaser for Linux / macOS / Windows × x64 / arm64

## Language

Go 1.22+. Rationale for the language choice (and why it is Go, not TypeScript or Rust) lives in `project_language_decisions.md` in the maintainer's memory system; the short version is that Go's static-binary distribution story fits manufacturer-CI adoption, `santhosh-tekuri/jsonschema/v6` has one of the strongest Draft 2020-12 compliance records in any language, and the maintainer ramp from Python is small.

## Build

```bash
cd tools/validator
go build -o bin/ulc ./cmd/ulc
./bin/ulc version
```

## Test

```bash
cd tools/validator
go test ./...
```

The load-bearing test is `internal/index.TestBuilderMatchesStoredIndex`, which runs `Build()` over every `.ulc` file in `examples/` and asserts the computed index block matches the stored one byte-for-byte. This keeps the Go and Python builders in lockstep during the transition; once `tools/build-index.py` retires, the stored index itself becomes the Go builder's authority.

## Relationship to the Python tooling

During Batch 4 migration, both builders coexist:

| Tool | Status | Role |
|---|---|---|
| `tools/build-index.py` | Kept | Legacy canonical builder. Will retire once the Go CLI is authoritative. |
| `tools/builder-parity-guard.py` | Kept | Retires alongside `build-index.py`. |
| `tools/schema-drift-guard.py` | Kept indefinitely | Internal `$ref` walker, Python is fine, no user-facing shipping. |
| `tools/validator/` (this dir) | New | The authoritative future tool. |

The end state is: one shipped Go binary (`ulc`), one internal Python guard (`schema-drift-guard.py`), zero drift surfaces.
