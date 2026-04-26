# ULC Roadmap

This roadmap describes near-term, next-major, and explicit out-of-scope work.
It is updated when scope decisions land. Concrete dates are not promised. ULC
ships when batches are ready, not on a calendar.

## Versioning model

ULC uses [SemVer](https://semver.org/). The version a record conforms to is
declared in its `ulc_version` field.

- **Major**: breaking schema changes (removed fields, narrowed types, removed
  enum values, changed `required[]`).
- **Minor**: backward-compatible additions and clarifications.
- **Patch**: corrections and non-structural edits.

v1.0.0 marks the first formal backward-compatibility commitment for the
schema surface. Pre-1.0 minor and patch versions follow the same rules
above and have been kept additive in practice.

## Active version: v0.5.x

- Patch releases for clarifications, doc fixes, mapping refinements
- No new normative fields without a v0.6.0 minor bump

## Next minor: v0.6.0

- Continued expansion of reference records, real cutsheets only (see
  `CONTRIBUTING.md` for sourcing rules)
- Additional PIM platform mapping guides as patterns mature
- Validator improvements informed by reference-record validation findings

## Toward v1.0.0

The criteria for declaring v1.0.0, schema-stable and ecosystem-mature:

- Reference records covering each declared category
- Validator hardened against malformed real-world cutsheets
- Stable PIM mapping guidance for the documented platforms
- At least one selected-manufacturer pilot completed end-to-end
- Adjacent-standard mappings (GLDF, ETIM, IES LM-63, EULUMDAT) drafted

## Explicitly NOT in v1.0.0

These items are deferred or out of scope. Listed explicitly so contributors
do not propose them for v1.0.0:

- **Site, design, or installation context**. Lighting design data belongs in
  design tools, not in fixture metadata. This is the scope rule from
  `CONTRIBUTING.md`. Pavement reflectance, pole spacing, mounting height,
  pedestrian activity: none of these belong in ULC.
- **IANA media-type registration** (`application/vnd.ulc+json`). Deferred
  until adoption signals support the filing.
- **Vendor-specific extensions in the core schema**. Handled via the
  `extensions` field, not normative.
- **Project-level lighting-design metadata**. Belongs in design tools or
  separate standards.
- **Schema URL versioning**. The `$id` stays versionless by design. Records
  declare conformance via `ulc_version`, not via a versioned schema URL.

## Branch model

- `main` is the integration branch and the release tag source
- `release/vX.Y.Z` for release PRs (auto-tagged on merge)
- `feat/*`, `fix/*`, `docs/*`, `chore/*` for non-release working branches
- All changes land via PR (ruleset enforced)

## Cadence

Batch-driven, not calendar-driven. Each batch is its own minor or patch
release. No fixed cadence is promised.
