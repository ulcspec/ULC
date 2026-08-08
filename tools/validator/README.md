# `ulc`: reference validator for ULC

The `ulc` command-line tool is the reference implementation of the ULC specification's validation and index-building logic. It is the authoritative check that a ULC record (files with a `.ulc` or `.ulc.json` extension, both accepted by all subcommands) is well-formed, conforms to the schema, and has a correctly-computed `index` block.

## Shipped features

The `ulc` CLI provides:

- [x] `ulc build-index <record>`: deterministic index projection (`<record>` is any `.ulc` or `.ulc.json` file)
- [x] `ulc build-index <record> --check`: verify stored index matches the builder; exit 1 on drift
- [x] `ulc build-index <record> --stdout`: print computed index without modifying the record
- [x] `ulc validate <record>`: JSON Schema Draft 2020-12 structural validation via [`santhosh-tekuri/jsonschema/v6`](https://github.com/santhosh-tekuri/jsonschema) with cross-file `$ref` resolution
- [x] `ulc from-sheet <bundle-dir|workbook.xlsx>`: deterministic, offline converter from a manufacturer workbook (CSV bundle or native `.xlsx`) to validated ULC records (`--out`, `--assets`, `--allow-missing-files`). Workbook archives are bounded: entry count, per-part and total inflated size, and compression ratio all carry limits, and a workbook exceeding one is rejected with an error naming the limit. The CSV bundle path is unchanged. A reference that resolves outside the assets root, including by way of a symbolic link, is rejected. A reference to a file that is not present behaves as before: an error unless `--allow-missing-files` is set, which stamps the zero sentinel and marks the record a draft.
- [x] Builder parity is included in `ulc validate` (stored `index` vs. computed)
- [x] File-reference SHA-256 hash verification when referenced files are reachable on the local filesystem: `source_files[].reference`, `product_family.cutsheet`, and `emergency.photometry_reference` are checked on every run, and the attestation evidence references under `--verify-evidence`. A locally absent file is an INFO, an unreadable file is a WARNING, and a file whose SHA-256 does not match its declared hash is an ERROR that fails validation.
- [x] Structured `ERROR` / `WARNING` / `INFO` findings, each with a JSON Pointer into the record
- [x] Conformance grading and Product Achievements: the builder computes both the conformance level (`incomplete` / `core` / `standard` / `full`, stamped into `index.conformance_level`) and the Product Achievements axis (per theme `none` / `claimed` / `documented`, stamped into `index.achievements`) from the record's populated fields, so both are authoritative rather than self-claimed (a hand-tampered value fails the builder-parity check like any other index field). Grading is class-aware: exit signs grade against the sign dataset (legend, illumination mode, battery, UL 924) instead of architectural photometry, and dedicated emergency luminaires keep the normal profile minus luminaire efficacy plus the emergency-battery gates. `ulc validate` reports the computed level, a per-grade tier roadmap to `full` (`conformance/gap`, each field naming its source document and governing standard), a non-gating enrichment roadmap of optional datasheet depth (`conformance/enrichment`), and a one-line achievements summary (`achievements/summary`). The level, tier roadmap, and achievements summary render in text by default; the enrichment roadmap, observation notes, and per-theme achievement detail (`achievements/state`, `achievements/roadmap`) appear only under `--verbose`, and JSON always includes everything. Conformance grading emits no `WARNING` (there is no declared level to fall short of), and inapplicable fields are skipped by predicate. See `docs/methodology.md` and `docs/how-it-works.md` for the grading model.
- [x] Opt-in attestation-expiry advisory (`ulc validate --expiry`): a report-only preview of attestation `valid_until` and `sustainability_declaration.expiration_date` against a caller-chosen date (`--as-of YYYY-MM-DD`, default today) within a window (`--expiry-window N` days, `0..36500`, default 90). Emits `expiry/summary` (INFO, always), `expiry/lapsed` (WARNING per already-lapsed surface), `expiry/downgrade` (WARNING per theme a re-stamp on or after the date would drop from `documented` to `claimed`), and `expiry/upcoming` (INFO per in-window surface). Advisory: it never changes the exit code, never touches the computed `index`, and is absent from default runs, so default output and the goldens stay byte-identical. `--as-of` and `--expiry-window` require `--expiry`; a malformed date or an out-of-range window is a usage error (exit 2).
- [x] Opt-in attestation-evidence verification (`ulc validate --verify-evidence`): byte-verifies the evidence documents attestations reference (`attestations[].source_document_ref` and `product_family.shared_attestations[].source_document_ref`) wherever the files are present locally. A locally absent document stays INFO, so a record that publishes the reference while withholding the document still validates; a document whose SHA-256 does not match is an ERROR and fails validation. Absent from default runs, so default output and the goldens are untouched by the flag.
- [x] `ulc scope <record>`: the grading-scope manifest, the rubric's applicability determination in result form. Prints one JSON document naming, for that record, which top-level blocks and which graded items the rubric holds in scope, so a tool built on the CLI can present, collect, or check the right fields without re-deriving the rubric. The envelope carries `scope_version`, `cli_version`, and `record_id` and `ulc_version` echoed from the record when it holds a non-empty string there; `blocks` is a derived rollup, the sorted union of the blocks the in-scope items' evidence lives in, and `items` is authoritative. Each item carries `tier` (`core` / `standard` / `full`), `kind`, `path`, `source_document`, and `standard`, with the rubric's own path strings, the same ones `conformance/gap` findings emit. `kind` describes the form of the path string only: `pointer` (a JSON Pointer naming the graded location, which may be a leaf, an object, or an array), `choice` (a pointer with an alternative, where either location satisfies), or `requirement` (a prose label for a requirement no single pointer names). Items carry no presence state, so scope minus the gaps `ulc validate` reports gives the satisfied set. The manifest covers the gating tiers only; the non-gating enrichment and observation guidance stays in `ulc validate`, and `items` will never carry non-gating rows (if that guidance is ever exported it arrives in a separate array). Report-time only: it runs no schema validation, changes no existing exit code, touches no `index`, and leaves default `ulc validate` output and every golden byte-identical. This is not the record's `applicability` block, which declares the range of orderable SKU configurations.
  - **Item identity is `(tier, path)`, and it is stable.** The `path`, `source_document`, and `standard` **values** are stable identifiers, not incidental prose: they are shared verbatim with `conformance/gap` findings, so a consumer may key on them and join the two surfaces by string equality. `path` alone is not unique across the whole rubric (a few paths are graded at two tiers, which is why identity is the pair), but it IS unique within any single manifest, so a per-record join on `path` is sound. New items may appear at a ULC minor; an existing item's identity does not change.
  - **The document is additive-only.** `scope_version` (`1.0.0` today) bumps its minor when fields, `kind` values, or arrays are added. Consumers must ignore any `tier` or `kind` value they do not recognize. Existing fields, key strings, item identities, and semantics never change for the lifetime of `scope_version` 1.x. `scope_version` is the manifest contract's own version, independent of both the CLI version and the index builder version, and this stability statement is scoped to it: the ULC version line governs the schema surface (see `ROADMAP.md`), which the manifest is not part of.
  - **Encoding.** The manifest is emitted as two-space-indented JSON with a trailing newline, and `<`, `>` and `&` are escaped as `\u003c`, `\u003e` and `\u0026`. (`ulc validate --json` escapes them the same way; `ulc build-index --stdout` deliberately does not, because its output must preserve the record's own byte shape.)
  - **`cli_version` identifies the binary**, not the rubric: it equals the ULC release for a release binary and is a development placeholder for a build from source. The manifest carries no separate rubric identifier today.
  - **`blocks` is a rollup, not a judgement.** A named block means at least one in-scope item's evidence lives there, never that the whole block is graded. `attestations` appears in every manifest because the safety-listing row is universal, and `product_family` is graded in part, not in full. Read `items` for anything precise.
  - **`record_id` and `ulc_version` are unvalidated record input.** `ulc scope` runs no schema validation, so both are attacker-controlled strings inside an otherwise CLI-authored document. They are echoed rather than dropped so that a consumer fanning out over a corpus and concatenating manifests keeps the record pairing, which matters most for the non-conforming records it is triaging. Treat both as untrusted: never a filesystem path, a cache key, or a trust anchor, and escape them for whatever output context they land in. Every other field is a static rubric or CLI string.
- [x] An A1 column reference beyond `XFD`, the format's maximum column, is treated as malformed by `ulc from-sheet` and its cell is skipped, the same handling a reference with no column letters already gets.
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
