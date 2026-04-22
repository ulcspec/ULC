#!/usr/bin/env python3
"""
ULC schema drift guard.

Loads every ULC schema file, walks every $ref, and fails with a non-zero exit
code on any dangling pointer. Prevents the taxonomy and the record schema
from drifting apart as they evolve.

Run from repo root:

    python3 tools/schema-drift-guard.py

Exit code 0 on success, 1 on any broken reference.
"""
import json
import sys
from pathlib import Path

SCHEMA_DIR = Path(__file__).resolve().parent.parent / "schema"
FILES = {
    "taxonomy.schema.json": SCHEMA_DIR / "taxonomy.schema.json",
    "ulc.schema.json": SCHEMA_DIR / "ulc.schema.json",
}

# The `ulc` CLI binary embeds these schema files via go:embed from its own
# module, so it must carry byte-identical copies. Kept in sync by this guard:
# on any drift, CI fails and the fix is a deterministic `cp` of the canonical
# schemas into the validator.
EMBED_MIRROR_DIR = Path(__file__).resolve().parent / "validator" / "schema"


def walk_refs(node, path=""):
    """Yield (json_path, ref_string) for every $ref string in the document."""
    if isinstance(node, dict):
        for k, v in node.items():
            if k == "$ref" and isinstance(v, str):
                yield path, v
            else:
                child = f"{path}.{k}" if path else k
                yield from walk_refs(v, child)
    elif isinstance(node, list):
        for i, v in enumerate(node):
            yield from walk_refs(v, f"{path}[{i}]")


def resolve_pointer(doc, pointer):
    """Return True if a JSON Pointer resolves inside doc.

    Supports the full Draft 2020-12 pointer grammar that ULC cares about:

      * `#`          resolves to the document root
      * `#/foo/bar`  resolves through object keys
      * `#/0/baz`    resolves through array indices (numeric segments)
      * `~0` / `~1`  escape sequences for `~` / `/` within a segment
    """
    if pointer == "#":
        return True
    if not pointer.startswith("#/"):
        return False
    parts = [p.replace("~1", "/").replace("~0", "~") for p in pointer[2:].split("/")]
    node = doc
    for p in parts:
        if isinstance(node, dict):
            if p in node:
                node = node[p]
            else:
                return False
        elif isinstance(node, list):
            try:
                idx = int(p)
            except ValueError:
                return False
            if 0 <= idx < len(node):
                node = node[idx]
            else:
                return False
        else:
            return False
    return True


def main():
    docs = {name: json.loads(p.read_text()) for name, p in FILES.items()}
    errors = []
    total = 0
    for source_name, doc in docs.items():
        for path, ref in walk_refs(doc):
            total += 1
            file_part, _, pointer_tail = ref.partition("#")
            # Whole-document ref (empty pointer or no fragment) resolves to root.
            pointer = "#" + pointer_tail if pointer_tail else "#"
            if file_part == "":
                target_doc, target_name = doc, source_name
            elif file_part in docs:
                target_doc, target_name = docs[file_part], file_part
            else:
                errors.append(f"{source_name} at {path}: unknown target file in {ref}")
                continue
            if not resolve_pointer(target_doc, pointer):
                errors.append(
                    f"{source_name} at {path}: dangling $ref {ref} (no {pointer} in {target_name})"
                )

    if errors:
        print(f"Schema drift detected -- {len(errors)} broken $ref(s):", file=sys.stderr)
        for e in errors:
            print(f"  {e}", file=sys.stderr)
        sys.exit(1)

    print(f"OK -- all {total} $ref pointers resolve across {len(docs)} schema files.")

    # Second guard: the validator's embedded copies must match the canonical
    # files byte-for-byte. The `ulc` binary bakes these in via go:embed, so
    # any drift means the shipped CLI would validate against stale schemas.
    mirror_errors = []
    for name, canonical in FILES.items():
        mirror = EMBED_MIRROR_DIR / name
        if not mirror.exists():
            mirror_errors.append(f"{mirror.relative_to(canonical.parent.parent)}: missing (copy from {name})")
            continue
        if mirror.read_bytes() != canonical.read_bytes():
            mirror_errors.append(
                f"{mirror.relative_to(canonical.parent.parent)} drifted from schema/{name}. "
                f"Fix with: cp schema/{name} tools/validator/schema/{name}"
            )
    if mirror_errors:
        print("Embedded-schema drift detected -- the ulc CLI ships stale schemas:", file=sys.stderr)
        for e in mirror_errors:
            print(f"  {e}", file=sys.stderr)
        sys.exit(1)
    print(f"OK -- tools/validator/schema/ mirrors match the canonical files.")


if __name__ == "__main__":
    main()
