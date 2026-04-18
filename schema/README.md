# Schema

This directory contains JSON Schema files that formally define the structure of ULC records.

ULC schemas use JSON Schema Draft 2020-12. Each schema is identified by a canonical URL rooted at `https://ulcspec.org/schema/`, declared as the `$id` inside the schema document.

## Scope

The schemas in this directory define:

- The canonical ULC record structure: required fields, data types, unit patterns, source-file references, and compliance attestations
- Closed enumerations for luminaire categories, mounting types, and optical function tags

## Validation

A reference validator is available in `tools/validator/`. Any JSON Schema Draft 2020-12 compliant validator can verify ULC records against these schemas.

## See also

- `docs/specification.md` for the narrative specification of what these schemas enforce
- `docs/field-reference.md` for per-field definitions
