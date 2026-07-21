# Documentation

This directory contains narrative documentation for the ULC specification. Where `schema/` holds the machine-readable definitions, this directory explains them in prose for implementers, authors, and readers who want to understand how and why ULC is structured the way it is.

## Contents

- `how-it-works.md`: how ULC works end to end for a consumer, covering what a record is, provenance and integrity, how records are consumed today, validation and the two computed views (completeness and achievements), and how to try it now.
- `methodology.md`: why ULC is shaped the way it is, covering the "one attested photometric scenario" unit, the design principles behind the schema, the conformance rubric, the two grading axes (completeness and achievements), and how the standard evolves.
- `authoring-patterns.md`: the four manufacturer authoring patterns (A/B/C/D), the exit-sign/emergency product-class authoring section, the architectural primitives, and the generated index, with each pattern grounded in a real cutsheet.
- `compliance-attestation.md`: the attestation-program glossary (every `AttestationProgram` token, its governance family, and its conformance role) and the achievement-themes appendix mapping tokens to the seven Product Achievements themes.

## See also

- `schema/` for the machine-readable schemas
- `mappings/` for crosswalks to GLDF, ETIM, and photometric source formats
- `examples/` for worked ULC records
