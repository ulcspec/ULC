# Templates

This directory holds the **fill-in workbook template** for authoring ULC records with the deterministic `ulc from-sheet` converter:

- [`workbook/`](workbook/) is a `records.csv` plus the related sheets. Copy it, transcribe your source data into it (one row per photometric scenario), and run `ulc from-sheet <workbook> --out <dir>` to produce validated ULC records with no LLM and no cloud step. The column reference lives in [`workbook/README.md`](workbook/README.md).

For the full picture of how a manufacturer creates ULC records (the two authoring paths, which source documents to gather, and why the output is accurate by construction), see [How records are authored](../docs/how-it-works.md#how-records-are-authored) in how-it-works.md.

For a sense of what a finished record looks like, read the curated real-world records in [`../examples/`](../examples/), and for the schema each record conforms to, see [`../schema/ulc.schema.json`](../schema/ulc.schema.json).

Earlier releases also shipped per-category hand-fill JSON skeletons here (one `.ulc.json` per luminaire category). Those were removed: hand-authoring JSON is not a recommended authoring path, the reference compiler builds and grades a record from any source regardless of luminaire type, and the real examples plus the schema serve the "what does a record look like" need better than an arbitrary subset of category skeletons did.
