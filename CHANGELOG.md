# Changelog

All notable changes to the ULC specification are recorded here.

ULC uses semantic versioning. Major versions indicate breaking changes to record structure or required behavior. Minor versions indicate backward-compatible additions or clarifications. Patch versions indicate corrections and non-structural edits.

Each ULC record declares the specification version it conforms to via the `ulc_version` field.

## Release process

A version is **unreleased** until it is tagged in git. Being visible on `main` is not the same as being released; consumers who want a stable version pin to a git tag.

When a version is ready to release:

1. Replace the `(unreleased)` marker next to the version heading below with the release date in ISO 8601 format, e.g. `## 0.1.0 (2026-06-15)`.
2. Commit that change to `main`.
3. Create an annotated git tag matching the version: `git tag -a v0.1.0 -m "ULC v0.1.0"`.
4. Push the tag: `git push origin v0.1.0`.
5. Optionally create a GitHub Release pointing at the tag, copying the version's changelog entry into the release notes.

## 0.2.0 (2026-04-22)

Canonical reference records for the four manufacturer authoring patterns, plus minor schema and tooling cleanups.

- Added four reference records in `examples/`, each drafted from a real manufacturer spec sheet and IES file and validated against the schema:
  - Pattern A (single SKU per cutsheet): Erco Quintessence 30416.023 recessed indoor downlight
  - Pattern B (per-photometric-scenario with applicability): Selux AYA Pole SR-HO-3000K with CCT multiplier table covering 2200 K through 5000 K
  - Pattern C (per-IES with provenance classes): Lumenpulse Lumenfacade LOI color-changing inground luminaire at the 12 in RGB 30x60 scenario, demonstrating `extended_photometry` provenance with `base_attestation_ref` pointing at the original LM-79 test
  - Pattern D (per-foot linear scaling with conditional attestations): Vode Nexa Suspended 807 at the Standard Output, 3500 K, 90 CRI, Honeycomb Louver Black Anodized, 48 in scenario, exercising option-conditional and case-by-case attestation patterns
- Removed `AttestationVerificationType.requires_project_documentation` from the taxonomy: the value introduced project-context semantics that crossed the fixture-relevance rubric boundary
- Corrected a path reference in the `ProvenanceMethod` description from `source.base_attestation_ref` to `provenance.base_attestation_ref` to match the schema
- Expanded the automated pre-merge review workflow's file-match pattern to include `templates/**/*.ulc` and `tools/hooks/**`
- Removed a dead `has_fragment` variable in `tools/schema-drift-guard.py`

## 0.1.0 (2026-04-22)

Foundation of the ULC specification: schemas, authoring patterns, drift-guard tooling.

- Established repository structure, governance, and contribution guidelines
- Set foundational project documentation (README, CONTRIBUTING, GOVERNANCE, CODE_OF_CONDUCT)
- Selected MIT License
- Reserved namespace `https://ulcspec.org/schema/` for schema identifiers
- Shipped the split schema foundation: `schema/taxonomy.schema.json` (78 closed enums grounded in study of IES LM-79, LM-80, TM-21, LM-84, TM-28, TM-30, TM-35, RP-8, RP-46, TM-15, LM-75, LM-82, LM-90, and related standards, with `AttestationProgram` carrying 102 values across trade-body, product-safety, energy-code, domestic-procurement, sustainability, and test-method programs) and `schema/ulc.schema.json` (record structure with product family, configuration, applicability, photometry, colorimetry, alpha-opic, flicker, outdoor classification, lumen maintenance, chromaticity shift, sustainability declaration, attestations, and a generated index block)
- Shipped `docs/authoring-patterns.md` describing the four manufacturer authoring patterns observed in real cutsheet evaluation and the architectural primitives the schema provides to support them
- Shipped `tools/schema-drift-guard.py` to validate every `$ref` resolves across the split, `tools/build-index.py` as the canonical index deriver (the index is generated, never hand-authored), and `tools/builder-parity-guard.py` to confirm builder-schema alignment
- Shipped `tools/hooks/pre-commit` as a tracked sample hook that mirrors the CI guards locally
- Added CI workflow at `.github/workflows/schema-drift-guard.yml` running drift, parity, and record-index checks on every pull request touching schema, the builder, or example records
