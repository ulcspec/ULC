# Governance

## Origin

The ULC specification was introduced by Foad Shafighi, MIES, IALD, CLD, in a talk on AI in lighting design at IALD Enlighten Europe 2025 in Valencia. It has since been developed through active dialogue with industry bodies, including DIAL (the German organization behind DIALux and GLDF), the Illuminating Engineering Society (IES) in the United States, and the Lighting Industry Association (LIA) in the UK. Pilot discussions with luminaire manufacturers and software vendors are underway.

## Stewardship

ULC is stewarded by Foad Shafighi, MIES, IALD, CLD, with input from the industry stakeholders engaged in its development. The specification, schema, and surrounding artifacts are developed with the intent of being an open, collaborative standard with broad industry participation. Governance formalizes as the contributor base grows, with additional maintainers invited based on sustained, constructive participation and domain expertise.

## Governance model

- The steward maintains direction over scope, structure, and release cadence.
- Contributions, feedback, and proposals are welcome through issues and pull requests.
- Material changes to the schema or taxonomy are reviewed by maintainers before adoption.
- Decisions are recorded in the changelog and, where significant, in governance notes.

## Maintainers

- Foad Shafighi, MIES, IALD, CLD, originator and primary maintainer

Additional maintainers are invited based on sustained contribution, domain expertise, and a track record of constructive participation.

## Decision-making

For day-to-day work, maintainers resolve questions directly through review and discussion.

For material changes to the specification, the process is:

1. A Schema Change Proposal is opened as an issue, describing the change, motivation, affected consumers, and backward compatibility.
2. Discussion occurs in the issue. Maintainers may ask for revisions, additional examples, or real-world grounding.
3. If the proposal is accepted in principle, a pull request implements the schema change, at least one example, documentation updates, and a changelog entry.
4. The pull request is merged when maintainers agree that the change is coherent, consistent with the specification's principles, and ready.

## Guiding principles

ULC should remain:

- Open. The specification is published under a permissive license and developed in public.
- Practical. Fields and rules should map to real products, real workflows, and real manufacturer source files.
- Technically credible. The specification should be implementable, validatable, and unambiguous.
- Interoperable. ULC should read and write cleanly with adjacent standards such as GLDF, ETIM, IES, and LDT.
- Metadata-only. ULC records reference and link to external standards and source files. They do not redistribute the text of paid or restricted standards.
- Extensible. Manufacturer-specific and experimental data has a defined place via the extensions mechanism, without diluting the core schema.

## Reporting

Concerns about conduct or governance can be raised privately with the maintainers listed above. See `CODE_OF_CONDUCT.md` for details on how reports are handled.
