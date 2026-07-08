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
schema surface. Pre-1.0 releases generally aim for additive changes;
compatibility-tightening changes may occur when documented in the changelog
(as with the v0.3.0 `cri_tier` enum tightening).

## Active version: v0.9.x

The current line. v0.9.0 made data completeness fully explicit: the optional
depth taxonomies are now surfaced as an actionable **enrichment roadmap**
(`conformance/enrichment` findings) rather than passive `--verbose`-only
observations, sitting alongside the existing tier roadmap. It also added five
optional fields (installed orientation, optical radiation band, adaptive
lighting modes, per-source photometric file format, and the TM-30 reference
illuminant type) and gave the grading package a structured `Compute()` entry
point. Every change is additive and non-gating: a v0.8.x record validates and
grades identically.

- Continued expansion of reference records, real cutsheets only (see
  `CONTRIBUTING.md` for sourcing rules)
- Additional PIM platform mapping guides as patterns mature
- Patch releases for clarifications, doc fixes, and mapping refinements
- No new normative fields without a minor bump

## Toward v1.0.0

v1.0.0 marks the first formal backward-compatibility commitment for the schema
surface. The direction is a second computed axis alongside data completeness: a
view of the third-party program achievements a record can demonstrate, computed
from the attestations a record already carries. This is described at the level
of direction only; the field shapes are not committed until the work lands with
real example records.

The criteria for declaring v1.0.0, schema-stable and ecosystem-mature:

- Reference records covering each declared category
- Validator hardened against malformed real-world cutsheets
- Stable PIM mapping guidance for the documented platforms
- At least one selected-manufacturer pilot completed end-to-end
- Adjacent-standard mappings (GLDF, ETIM, IES LM-63, EULUMDAT) drafted

## Deferred schema work

Patterns observed in real cutsheets that the current schema does not yet
model natively. Each is expected to land when a second independent example
record surfaces the same need, so the change stays grounded rather than
speculative. Breaking changes among these are held for a future major
revision.

- **Accessory photometric records.** When a mechanical accessory genuinely
  changes photometry (louver, snoot, distribution-altering lens), the
  accessorized record may need an `accessorized_from` relationship back to
  the base-fixture record. Pending more examples.
- **Plural attestation references.** `attestation_ref` is a single string,
  but a value like CRI Ra has two legitimate references (LM-79 data plus
  CIE 13 method). A future breaking revision may add a plural
  `attestation_refs`, or split the data-collection and method references.
- **Multi-claim lumen maintenance.** `manufacturer_rated_claim` is
  single-claim, but real products publish several thresholds at once (L70
  at X hours plus L95 at Y hours). A future breaking revision may make the
  block an array.
- **Multi-framework lumen maintenance.** `declaration_framework` inside
  `lumen_maintenance_luminaire` is single-valued, though its description
  allows multiple frameworks to coexist. A future revision may add array
  support.
- **Per-protocol dimming depth.** `dimming_range_percent` is a single pair,
  but real products publish distinct dim depths per control protocol (DALI
  DT8 0.1% vs 0-10V 1% on the same fixture). A future revision may add
  `dimming_range_percent_by_protocol`.
- **Separate temperature ranges.** Storage, startup, and operating
  temperature ranges as distinct fields.
  `shared_mechanical.ambient_operating_range` captures operating only today.
- **Multi-variant dimensions and EPA.** Product families that ship several
  physical variants on one cutsheet (pole-top vs pendant, with different
  masses and EPAs). The schema currently carries one set of dimensions per
  record and relies on the record being per-SKU.
- **Manufacturer-specific control protocols.** Protocols like Lutron
  Hi-lume stay extension-parked unless a second independent consumer need
  arises. (`lumentalk` was promoted to the core `DimmingProtocol` enum in
  v0.3 because it is licensed across multiple manufacturers, not solely its
  originator.)
- **New AttestationProgram values.** UNION MADE USA, AASHTO 2013 LTS-6
  pole-wind compliance, ASTM/PCI powder-coat finish certifications, and
  IP/IK ratings as first-class `AttestationProgram` values (today carried
  on `shared_mechanical.ip_rating` / `ik_rating`). Each is a candidate once
  a second independent record surfaces the same pattern.
- **Taxonomy wiring policy and staged vocabulary.** Every taxonomy enum earns
  its place by gating a tier, feeding the enrichment roadmap, or being
  structural machinery a graded field is built from; an enum wired to no field
  is dead weight. v0.9.0 wired the orphaned enums whose fields are simple
  additive drops (orientation, optical radiation band, adaptive lighting modes,
  per-source photometry format, and the TM-30 reference illuminant type). Two
  enums remain staged vocabulary: `TM30DesignIntent` and `TM30Level`. Their
  ground is partially surfaced today by the `colorimetry.tm_30.pvf_code`
  enrichment row, but the PVF code cannot express a `none` designator or
  independent claims against all three design intents, so the two enums stay
  unwired until a cutsheet needs the fuller shape.
- **Evidence-gated device enums.** `LEDDeviceClass` and `FailureMode` are
  defined but unwired. Wiring them may need new sub-structure rather than a
  simple field drop, so they are held until a real cutsheet or test report
  surfaces the shape, rather than committing an object design speculatively.
- **Retirement candidates for v1.0.0.** Two enums carry an open "earns its
  place?" question for the first backward-compatibility commitment:
  `LegacyCutoffClassification` (deprecated, kept only for lossless ingestion of
  legacy datasheets) and `RecordStatus`. Either may be pruned in v1.0.0.
- **Emergency-lighting operational data.** ULC carries `emergency_luminaire`
  and `exit_sign` categories and a UL 924 attestation, but not the defining
  emergency data: battery chemistry and capacity, rated emergency runtime,
  reduced emergency-mode lumen output, self-test capability (manual,
  status-indicator, remote, or integral automatic), and the normal-power
  transfer threshold. A future revision may add an `emergency` field group,
  grounded once an emergency-luminaire example record surfaces.
- **Entertainment fixture capabilities.** Moving-head and theatrical fixtures
  expose capabilities not modeled today: pan and tilt range, zoom (variable
  field-angle) range, beam-shaping hardware (framing shutters, gobo wheels,
  iris), strobe, subtractive CMY color mixing, and the entertainment control
  transports sACN and Art-Net (DMX and RDM already exist in `DimmingProtocol`).
  These are fixture capabilities, ranges and mechanisms; the as-aimed position
  on a project is design context and stays out of scope (see below). Held for
  an entertainment-fixture example.
- **Beam-spread classification for floods.** Area and sports floodlights are
  classified by NEMA beam-spread type (Type 1 through 7), distinct from the IES
  distribution Type I through V that `photometry.distribution_type` already
  carries. A future revision may add a `beam_spread_nema_type` field.
- **Structured safety-listing detail.** A safety listing is recorded today as a
  single `AttestationProgram` token. A listing is verified by its exact scope:
  the governing product-safety standard (UL 1598 / 1574 / 8750 / 924), the
  listing category (UL CCN), the exact listed model, and the warranting entity
  when it differs from the seller. A future revision may add a structured
  listing-detail object alongside the token. Relates to the `New
  AttestationProgram values` item above.

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
