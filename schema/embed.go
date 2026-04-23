// Package schema exposes the ULC schema files as byte slices for Go
// consumers. The JSON files (`ulc.schema.json` and `taxonomy.schema.json`)
// are the canonical ULC specification; this file embeds them into the
// `ulc` binary via `//go:embed` so the CLI can run outside the source
// repository.
//
// Non-Go consumers can ignore embed.go entirely. The JSON files remain the
// authoritative source and are published at
// https://ulcspec.org/schema/ulc.schema.json and
// https://ulcspec.org/schema/taxonomy.schema.json. This Go file exists only
// so the shipped `ulc` binary is self-contained.
package schema

import _ "embed"

//go:embed ulc.schema.json
var ULCSchemaJSON []byte

//go:embed taxonomy.schema.json
var TaxonomySchemaJSON []byte
