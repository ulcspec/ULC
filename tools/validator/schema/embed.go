// Package schema embeds the ULC schema files into the `ulc` binary so the
// validator can run outside the source repository.
//
// The files in this directory are mirrors of the repo-root `schema/` files;
// they are kept byte-identical by a drift guard in CI. Never edit them here
// directly — edit schema/ulc.schema.json or schema/taxonomy.schema.json at
// the repo root and let the copy-step (see tools/README.md) or the drift
// guard's fix hint bring them back into sync.
package schema

import _ "embed"

//go:embed ulc.schema.json
var ULCSchemaJSON []byte

//go:embed taxonomy.schema.json
var TaxonomySchemaJSON []byte
