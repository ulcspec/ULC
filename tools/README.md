# Tools

This directory contains reference implementations for working with ULC.

These tools are open-source reference implementations intended to verify the specification's correctness and to give manufacturers and implementers a baseline they can use, extend, or replace. Production tooling and ingestion pipelines are expected to live in consumers' own codebases.

## Contents

- `validator/` is a command-line validator that checks a ULC record against the JSON Schema, reports structural errors, and verifies that referenced source files match their declared content hashes.
- `package-builder/` is a command-line utility that assembles a ULC record alongside its source files (datasheet PDF, IES, LDT) into a canonical folder layout, computing content hashes and validating the resulting record.

## Language and dependencies

Reference tools are written in TypeScript and target Node.js 20 or later. AJV is used as the JSON Schema Draft 2020-12 validator. Single-binary distribution is provided where practical so the tools can run without a local Node installation.
