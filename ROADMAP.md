# ULC Roadmap

This roadmap describes near-term, next-major, and explicit out-of-scope work.
It is updated when scope decisions land. Concrete dates are not promised. ULC
ships when batches are ready, not on a calendar.

## Versioning model

ULC uses [SemVer](https://semver.org/). The version a record conforms to is
declared in its `ulc_version` field.

- **Major**: either a breaking schema change (a removed field, a narrowed type,
  a removed enum value, a changed `required[]`), or a milestone that marks a
  backward-compatibility commitment with an additive-only surface. v1.0.0 is the
  second kind: it adds a whole grading axis and removes nothing.
- **Minor**: backward-compatible additions and clarifications.
- **Patch**: corrections and non-structural edits.

v1.0.0 is the first formal backward-compatibility commitment for the schema
surface. From v1.0.0 forward the schema surface is additive-only across minors;
any breaking change (a removed field, a narrowed type, a removed enum value, a
changed `required[]`) requires the next major, v2.0.0. Pre-1.0 releases generally
aimed for additive changes; compatibility-tightening changes occurred only when
documented in the changelog (as with the v0.3.0 `cri_tier` enum tightening).

## Active version: v1.0.x

The current line, and ULC's first formal backward-compatibility commitment.
v1.0.0 adds **Product Achievements**, a second computed axis alongside data
completeness: a per-theme view (embodied carbon, circularity, material health,
energy, dark sky, emergency) of the third-party program qualifications a record
demonstrates, each `none`, `claimed`, or `documented` by whether a qualifying
attestation carries attached, unexpired evidence, computed from the attestations a
record already carries and
stamped into `index.achievements` (with the `index.restricted_substances_declared`
sibling flag). Emergency capability is part of that axis and applies to every
product carrying a qualifying token, a dedicated exit sign or a normal fixture
with a factory emergency-power option alike. The release is additive to the authored
surface: every authored schema change is a new optional field, def, or enum, and no
field, token, or code is removed or narrowed; grades and completeness findings are
byte-identical. The two new generated index members are required in the built index,
so each stored record re-stamps with `ulc build-index` to gain them.

The 1.0 milestone is defined by the two computed axes and the compatibility
commitment, justified by the additive-only release history and a validator
hardened against real cutsheets. Two items that earlier drafts framed as 1.0
gates, a completed end-to-end manufacturer pilot and drafted adjacent-standard
mappings, are not release gates: the 1.0 milestone is itself what enables the
pilots and the public-site rewrite, not the reverse. They are tracked as
post-1.0 adoption work below.

- Continued expansion of reference records, real cutsheets only (see
  `CONTRIBUTING.md` for sourcing rules)
- A real EPD-backed or Cradle to Cradle-backed example, added when a manufacturer
  supplies a real cutsheet with its source documents (real-data rule; the
  embodied-carbon and circularity ladders are fixture-tested until then, never
  fabricated on an example)
- A selected-manufacturer pilot carried through end-to-end (adoption, not a gate)
- Adjacent-standard mappings (GLDF, ETIM, IES LM-63, EULUMDAT) drafted (adoption,
  not a gate)
- Additional PIM platform mapping guides as patterns mature
- Patch releases for clarifications, doc fixes, mapping refinements, and advisory validator additions that change no normative surface, no computed value, and no default output
- No new normative fields without a minor bump

## Deferred schema work

Patterns observed in real cutsheets that the current schema does not yet
model natively. Each is expected to land when a second independent example
record surfaces the same need, so the change stays grounded rather than
speculative. With v1.0.0's compatibility commitment now in force, every
breaking item among these, the plural attestation references, the multi-claim
and multi-framework lumen-maintenance arrays, and the structured safety-listing
detail, is foreclosed to the next major, v2.0.0; minors stay additive-only.

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
- **Retirement candidates resolved as KEEP.** Two enums carried an open "earns
  its place?" question for the first backward-compatibility commitment.
  `LegacyCutoffClassification` is kept: it is the lossless-ingestion path for
  legacy datasheets and the substrate the dark-sky achievement theme reads.
  `RecordStatus` is kept: it is confirmed pure envelope metadata that the
  required envelope, the sheet converter default, every example record, and the
  PIM mapping guides depend on. v1.0.0 ships zero breaking changes, so neither is
  pruned; any future removal is a v2.0.0 change.
- **Normal-power transfer threshold.** The `emergency` block (v0.10.0) carries
  the defining emergency data (battery chemistry and capacity, rated runtime,
  emergency-mode lumen output, self-test capability), but not the voltage at
  which a unit transfers to emergency power. None of the surveyed cutsheets
  publishes a threshold value (the brownout circuit is described only
  qualitatively), so the field stays unwired and evidence-gated until a real
  sheet publishes one.
- **International emergency standards.** The emergency and exit-sign gates are
  US-first, anchored on UL 924 and the NFPA 101 / IBC evidence base. EN
  60598-2-22, EN 1838, and ISO 30061 are deferred until researched: no tokens
  and no gates are added speculatively. A non-NA dedicated product grades today
  against the general any-recognized-listing safety row plus the full class
  dataset.
- **Static monochromatic color token.** `ColorTunabilityCapability` has no token
  for a fixed single-color (non-white) output, so an exit sign's color lives in
  `exit_sign.legend_color` and the universal `color_tunability` core row is
  not-applicable for signs. The gap is broader than signs (monochrome amber and
  red architectural fixtures exist); a `static_color` token is a candidate once a
  second independent record surfaces the need.
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
- **Future achievement themes.** The achievements axis ships six themes under an
  open theme container, so new themes add without breaking. Controls and
  domestic-content themes are planned for v1.1; hazardous-location and
  regional-market-access-consolidation themes, a social-responsibility theme
  (the organization-level ILFI JUST label, `just_label`, is unthemed-tracked
  toward it), and a disposition for the project-level programs (LEED, WELL)
  beyond their current unthemed-tracked state, are candidates for v1.2.
- **Attestation-document byte-verification.** `VerifyHashes` byte-checks
  `source_files[].reference` hashes today; the evidence documents the achievements
  axis reads (`attestations[].source_document_ref`) are checked for presence, not
  byte-verified, consistent with the cutsheet treatment. Byte-verifying attachment
  documents is future work.
- **DarkSky as a sustainability facet, and C2C per-category levels.** The dark-sky
  theme stands alone today; folding it under a broader sustainability grouping,
  and recording Cradle to Cradle per-category levels (the `sustainability_metric`
  carries the overall level only), are non-breaking additions held for a real need.
- **Filename minimum length.** `FileReference.filename` has no `minLength`, so an empty-string filename is schema-valid and can still back a `documented` achievement. Tightening to `minLength: 1` touches every `FileReference` site across the schema, so it is held for a future schema minor.

## Explicitly out of scope

These items are deferred or out of scope. Listed explicitly so contributors
do not propose them:

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
