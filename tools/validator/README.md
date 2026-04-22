# `ulc` — reference validator for ULC

The `ulc` command-line tool is the reference implementation of the ULC specification's validation and index-building logic. It is the authoritative check that a `.ulc` file is well-formed, conforms to the schema, and has a correctly-computed `index` block.

## Shipped features

As of v0.4.0:

- [x] `ulc build-index <record.ulc>` — deterministic index projection
- [x] `ulc build-index <record.ulc> --check` — verify stored index matches the builder; exit 1 on drift
- [x] `ulc build-index <record.ulc> --stdout` — print computed index without modifying the record
- [x] `ulc validate <record.ulc>` — JSON Schema Draft 2020-12 structural validation via [`santhosh-tekuri/jsonschema/v6`](https://github.com/santhosh-tekuri/jsonschema) with cross-file `$ref` resolution
- [x] Builder parity is included in `ulc validate` (stored `index` vs. computed)
- [x] Source-file SHA-256 hash verification when referenced files are reachable on the local filesystem
- [x] Structured `ERROR` / `WARNING` / `INFO` findings, each with a JSON Pointer into the record
- [x] `--json` machine-readable output
- [x] Single-file binaries via GoReleaser for Linux / macOS / Windows × x64 / arm64, cut on tag push
- [x] Embedded schemas via `go:embed` so the binary runs outside the source repository
- [x] `--schema-dir PATH` override for pointing the binary at an alternate schema copy

Deferred to a follow-up CLI release (scope for post-pilot feedback):

- [ ] **Conformance-grading rubric** — currently emits a single `INFO` marker per record acknowledging the declared `conformance_level`. The rubric that grades records against the `standard` and `full` completeness targets is intentionally left unimplemented until manufacturer pilot feedback indicates what those levels should require.

## Language

Go 1.22+. Selected after an independent language re-evaluation on 2026-04-22 that overturned an earlier tentative TypeScript + AJV choice. Rationale: `santhosh-tekuri/jsonschema/v6` has a stronger Draft 2020-12 compliance pedigree than AJV (non-trivial for a reference validator), Go's static-binary distribution fits manufacturer-CI adoption better than Node's options, and the maintainer ramp from Python is small. Rust was the close second.

## Build

```bash
cd tools/validator
go build -o bin/ulc ./cmd/ulc
./bin/ulc version
```

## Test

```bash
cd tools/validator
go test -race ./...
```

The load-bearing tests are:

- `internal/index.TestBuilderMatchesStoredIndex` — runs `Build()` over every `.ulc` file in `examples/` and asserts the computed index block matches the stored one byte-for-byte. Pins the builder against the committed canonical records.
- `internal/index.TestBuilderSchemaParity` — loads `Index.properties` and `Index.required` from `schema/ulc.schema.json`, runs `Build()` over a maximal synthetic fixture that triggers every emit path, and confirms no unknown keys are emitted and every required key is produced. Replaces the retired `tools/builder-parity-guard.py`.
- `internal/validate.TestValidatorAcceptsExampleRecords` / `TestValidatorRejectsBrokenRecord` — schema validation accept + reject cases.
- `internal/validate.TestVerifyHashesAllOutcomes` — hash verification on valid, mismatching, and missing-local files against the real schema-shaped `source_files[].reference` wrapper.

## Relationship to the Python tooling

The former Python builder and builder-parity guard retired in v0.4.0 when the Go CLI became authoritative:

| Tool | Status | Role |
|---|---|---|
| `tools/build-index.py` | Retired in v0.4.0 | Replaced by `ulc build-index`. |
| `tools/builder-parity-guard.py` | Retired in v0.4.0 | Replaced by `TestBuilderSchemaParity`. |
| `tools/schema-drift-guard.py` | Kept indefinitely | Internal `$ref` walker plus embedded-mirror drift guard. Not shipped externally. |
| `tools/validator/` (this dir) | Authoritative | The reference tool. |

End state: one shipped Go binary (`ulc`), one internal Python guard (`schema-drift-guard.py`), zero drift surfaces.
