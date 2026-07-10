# `ulc`: reference validator for ULC

The `ulc` command-line tool is the reference implementation of the ULC specification's validation and index-building logic. It is the authoritative check that a ULC record (files with a `.ulc` or `.ulc.json` extension, both accepted by all subcommands) is well-formed, conforms to the schema, and has a correctly-computed `index` block.

## Shipped features

The `ulc` CLI provides:

- [x] `ulc build-index <record>`: deterministic index projection (`<record>` is any `.ulc` or `.ulc.json` file)
- [x] `ulc build-index <record> --check`: verify stored index matches the builder; exit 1 on drift
- [x] `ulc build-index <record> --stdout`: print computed index without modifying the record
- [x] `ulc validate <record>`: JSON Schema Draft 2020-12 structural validation via [`santhosh-tekuri/jsonschema/v6`](https://github.com/santhosh-tekuri/jsonschema) with cross-file `$ref` resolution
- [x] `ulc from-sheet <bundle-dir|workbook.xlsx>`: deterministic, offline converter from a manufacturer workbook (CSV bundle or native `.xlsx`) to validated ULC records (`--out`, `--assets`, `--allow-missing-files`)
- [x] Builder parity is included in `ulc validate` (stored `index` vs. computed)
- [x] Source-file SHA-256 hash verification when referenced files are reachable on the local filesystem
- [x] Structured `ERROR` / `WARNING` / `INFO` findings, each with a JSON Pointer into the record
- [x] Conformance grading and Product Achievements: the builder computes both the conformance level (`incomplete` / `core` / `standard` / `full`, stamped into `index.conformance_level`) and the Product Achievements axis (per theme `none` / `claimed` / `documented`, stamped into `index.achievements`) from the record's populated fields, so both are authoritative rather than self-claimed (a hand-tampered value fails the builder-parity check like any other index field). Grading is class-aware: exit signs grade against the sign dataset (legend, illumination mode, battery, UL 924) instead of architectural photometry, and dedicated emergency luminaires keep the normal profile minus luminaire efficacy plus the emergency-battery gates. `ulc validate` reports the computed level, a per-grade tier roadmap to `full` (`conformance/gap`, each field naming its source document and governing standard), a non-gating enrichment roadmap of optional datasheet depth (`conformance/enrichment`), and a one-line achievements summary (`achievements/summary`). The level, tier roadmap, and achievements summary render in text by default; the enrichment roadmap, observation notes, and per-theme achievement detail (`achievements/state`, `achievements/roadmap`) appear only under `--verbose`, and JSON always includes everything. Conformance grading emits no `WARNING` (there is no declared level to fall short of), and inapplicable fields are skipped by predicate. See `docs/methodology.md` and `docs/how-it-works.md` for the grading model.
- [x] `--json` machine-readable output
- [x] Single-file binaries via GoReleaser for Linux / macOS / Windows × x64 / arm64, cut on tag push
- [x] Embedded schemas via `go:embed` so the binary runs outside the source repository
- [x] `--schema-dir PATH` override for pointing the binary at an alternate schema copy

Not yet implemented:

- [ ] Promoting selected observations to graded requirements. The `full` tier hard-gates accredited-laboratory depth: zonal lumens, measurement uncertainty, applied corrections, method-backed lumen-maintenance projections, deeper instrumentation metadata, and (for primarily-white-light fixtures) TM-30 fidelity. The remaining comprehensive items stay non-gating across two channels: the enrichment roadmap carries the optional datasheet depth (power factor, flicker, alpha-opic and circadian metrics, and similar), while a sustainability declaration and a small residual set stay plain `conformance/observation` notes.

## Language

Go 1.22+. Selected after an independent language re-evaluation on 2026-04-22 that overturned an earlier tentative TypeScript + AJV choice. Rationale: `santhosh-tekuri/jsonschema/v6` has a stronger Draft 2020-12 compliance pedigree than AJV (non-trivial for a reference validator), Go's static-binary distribution fits manufacturer-CI adoption better than Node's options, and the maintainer ramp from Python is small. Rust was the close second.

## Build

The Go module lives at the repo root (`go.mod` is `github.com/ulcspec/ULC`); the validator is a subpackage. Build from the repo root:

```bash
mkdir -p tools/validator/bin
go build -o tools/validator/bin/ulc ./tools/validator/cmd/ulc
./tools/validator/bin/ulc version
```

## Test

```bash
go test -race ./...
```

The load-bearing tests are:

- `internal/index.TestBuilderMatchesStoredIndex`: runs `Build()` over every `.ulc` file in `examples/` and asserts the computed index block matches the stored one byte-for-byte. Pins the builder against the committed canonical records.
- `internal/index.TestBuilderSchemaParity`: loads `Index.properties` and `Index.required` from `schema/ulc.schema.json`, runs `Build()` over a maximal synthetic fixture that triggers every emit path, and confirms no unknown keys are emitted and every required key is produced. Replaces the retired `tools/builder-parity-guard.py`.
- `internal/validate.TestValidatorAcceptsExampleRecords` / `TestValidatorRejectsBrokenRecord`: schema validation accept + reject cases.
- `internal/validate.TestVerifyHashesAllOutcomes`: hash verification on valid, mismatching, and missing-local files against the real schema-shaped `source_files[].reference` wrapper.

## Relationship to the canonical schemas

The schemas have exactly one canonical location: `schema/ulc.schema.json` and `schema/taxonomy.schema.json` at the repo root. The `ulc` binary embeds those same files via `//go:embed` from `schema/embed.go`, so the shipped binary carries the matching spec version without any file copies anywhere else in the tree. Editing `schema/*.json` is all you need to do when revving the spec.

## Relationship to the Python tooling

The former Python builder and builder-parity guard retired in v0.4.0 when the Go CLI became authoritative:

| Tool | Status | Role |
|---|---|---|
| `tools/build-index.py` | Retired in v0.4.0 | Replaced by `ulc build-index`. |
| `tools/builder-parity-guard.py` | Retired in v0.4.0 | Replaced by `TestBuilderSchemaParity`. |
| `tools/schema-drift-guard.py` | Kept indefinitely | Internal `$ref` resolution walker. Not shipped externally. |
| `tools/validator/` (this dir) | Authoritative | The reference tool. |

End state: one shipped Go binary (`ulc`), one internal Python guard (`schema-drift-guard.py`), zero drift surfaces.
