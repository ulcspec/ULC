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

## 0.1.0 (unreleased)

Initial public scaffolding of the ULC specification.

- Established repository structure, governance, and contribution guidelines
- Set foundational project documentation (README, CONTRIBUTING, GOVERNANCE, CODE_OF_CONDUCT)
- Selected MIT License
- Reserved namespace `https://ulcspec.org/schema/` for schema identifiers
- Specification, schema, examples, mappings, and reference tools to follow in subsequent batches of this release
