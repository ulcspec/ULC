# Security Policy

## Reporting a vulnerability

Report potential security vulnerabilities through GitHub's Private Vulnerability
Reporting at [github.com/ulcspec/ULC/security/advisories/new](https://github.com/ulcspec/ULC/security/advisories/new).

Acknowledgement target: 5 business days. Patch target depends on severity and
scope, communicated in the advisory thread.

## Scope

ULC is a specification project. Security considerations differ across the
artifacts in this repository.

### In scope

- The reference validator (`tools/validator/`, distributed as the `ulc` Go
  binary): parse safety, error handling, supply-chain integrity of Go module
  dependencies, archive integrity of release binaries.
- Schema files (`schema/`) where a malformed schema could cause downstream
  validators to misbehave (regex denial-of-service patterns, infinite `$ref`
  loops, oversized expansions).
- Build and release workflows (`.github/workflows/`) where compromise could
  affect the integrity of distributed binaries.

### Out of scope (these are not security issues)

- Disagreements with normative content of the specification. Open an issue
  using the "Spec clarification" or "Schema change proposal" template.
- Missing IES, LDT, GLDF, ETIM, or LM-79/LM-80/LM-84 features. Open a feature
  request.
- Taxonomy debates. Open a Discussion.
- Suggestions to expand or restrict ULC's scope. Open a Discussion or Issue.

## Supported versions

ULC is in the pre-1.0 phase. Only the latest tagged version receives security
fixes. Pin to a specific tag and upgrade promptly when a patch ships.

| Version | Security fixes |
|---------|----------------|
| Latest tagged release | Yes |
| Older tagged releases | No |
| Untagged commits on `main` | Best-effort, not guaranteed |

After v1.0.0, this policy will be revised to support the most recent minor
version in addition to the latest tag.
