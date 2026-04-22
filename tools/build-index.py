#!/usr/bin/env python3
"""
ULC index builder.

The canonical source of a ULC record's `index` block. Reads the record's deep
blocks (product_family, configuration, electrical, photometry, colorimetry,
outdoor_classification, attestations, ...) and emits the denormalized summary
fields used by AI scan, search indexers, and filter UIs.

The spec forbids hand-authoring the index. This script is the single authority.
Any record whose `index` does not exactly match `build-index --check` output is
considered stale.

Usage:

    python3 tools/build-index.py record.ulc              # write in place
    python3 tools/build-index.py record.ulc --stdout     # print without writing
    python3 tools/build-index.py record.ulc --check      # exit 1 on mismatch

Exit codes:
    0  success (wrote / printed / index matches)
    1  mismatch in --check mode, or fatal error
"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

# Bump on any change to builder logic so stale indices are detectable.
# 0.2.0: removed `nominal_cct_k` from REQUIRED_INDEX_KEYS in sync with the
# v0.3 schema change. Color-changing fixtures (RGB, RGBW, RGBA, multichannel)
# no longer need a placeholder CCT value to pass validation.
BUILDER_VERSION = "0.2.0"

# Keys the schema's Index.required list demands. Must stay aligned with
# schema/ulc.schema.json#/$defs/Index.required. tools/builder-parity-guard.py
# enforces that alignment at CI time so this local copy cannot drift unnoticed.
# `nominal_cct_k` is intentionally NOT required at Index level: color-changing
# fixtures (RGB, RGBW, RGBA, multichannel) have no meaningful single CCT and
# would otherwise be forced to populate a placeholder value. The builder still
# emits nominal_cct_k when configuration.tested_conditions.nominal_cct_at_test
# is populated (every white-only fixture); it just does not require it.
REQUIRED_INDEX_KEYS = {
    "x-ulc-generated",
    "builder_version",
    "manufacturer_slug",
    "catalog_model",
    "primary_category",
    "nominal_total_lumens",
    "nominal_input_power_w",
}

# Human-readable source paths for each required key, used when the builder
# refuses to emit an invalid index.
REQUIRED_KEY_SOURCES = {
    "manufacturer_slug": "product_family.manufacturer.slug",
    "catalog_model": "product_family.catalog_model",
    "primary_category": "product_family.primary_category",
    "nominal_total_lumens": "photometry.total_luminous_flux_lm.value",
    "nominal_input_power_w": "electrical.input_power_w.value",
}


def _get(d, *path, default=None):
    """Safe nested getter. _get(obj, 'a', 'b', 'c') returns obj['a']['b']['c'] or default."""
    node = d
    for p in path:
        if not isinstance(node, dict) or p not in node:
            return default
        node = node[p]
    return node


def _collect_attestation_programs(record):
    programs = set()
    for a in _get(record, "product_family", "shared_attestations", default=[]) or []:
        if isinstance(a, dict) and a.get("program"):
            programs.add(a["program"])
    for a in record.get("attestations", []) or []:
        if isinstance(a, dict) and a.get("program"):
            programs.add(a["program"])
    sd = record.get("sustainability_declaration")
    if isinstance(sd, dict) and sd.get("declaration_type"):
        # Map sustainability declaration types to their AttestationProgram enum values.
        dt = sd["declaration_type"]
        mapping = {
            "ilfi_declare": "declare",
            "red_list_free": "lbc_red_list_free",
            "red_list_approved": "lbc_red_list_approved",
            "red_list_declared": "lbc_red_list_declared",
        }
        if dt in mapping:
            programs.add(mapping[dt])
    return sorted(programs)


def _format_bug(bug):
    if not isinstance(bug, dict):
        return None
    b, u, g = bug.get("b"), bug.get("u"), bug.get("g")
    if None in (b, u, g):
        return None
    return f"B{b}-U{u}-G{g}"


def build_index(record: dict) -> dict:
    """Deterministic projection of the record's deep blocks into the index shape."""
    pf = record.get("product_family", {}) or {}
    cfg = record.get("configuration", {}) or {}
    elec = record.get("electrical", {}) or {}
    phot = record.get("photometry", {}) or {}
    color = record.get("colorimetry", {}) or {}
    outdoor = record.get("outdoor_classification", {}) or {}

    index = {
        "x-ulc-generated": True,
        "builder_version": BUILDER_VERSION,
    }

    # Required core-identity projections.
    mfr_slug = _get(pf, "manufacturer", "slug")
    if mfr_slug:
        index["manufacturer_slug"] = mfr_slug
    if pf.get("catalog_model"):
        index["catalog_model"] = pf["catalog_model"]
    if pf.get("primary_category"):
        index["primary_category"] = pf["primary_category"]

    # Catalog number: the specific orderable number authored in configuration, or
    # fall back to record_id for Pattern B records where the scenario covers a
    # catalog_pattern rather than a single SKU.
    catalog_number = (
        _get(cfg, "catalog_number")
        or record.get("record_id")
    )
    if catalog_number:
        index["catalog_number"] = catalog_number

    display = cfg.get("scenario_label") or pf.get("family_display_name")
    if display:
        index["display_name"] = display

    # Classification projections. Dedupe + sort semantically unordered arrays
    # so repeated builder runs on the same record produce byte-identical output.
    if pf.get("secondary_function"):
        index["secondary_function"] = sorted(set(pf["secondary_function"]))
    if pf.get("indoor_outdoor"):
        index["indoor_outdoor"] = pf["indoor_outdoor"]
    if pf.get("mounting_types"):
        index["mounting_types"] = sorted(set(pf["mounting_types"]))
    if pf.get("environment_rating"):
        index["environment_rating"] = pf["environment_rating"]

    # Colorimetric and electrical projections (baseline-measured, not derived).
    cct = _get(cfg, "tested_conditions", "nominal_cct_at_test")
    if cct is not None:
        index["nominal_cct_k"] = cct

    flux = _get(phot, "total_luminous_flux_lm", "value")
    if flux is not None:
        index["nominal_total_lumens"] = flux

    watts = _get(elec, "input_power_w", "value")
    if watts is not None:
        index["nominal_input_power_w"] = watts

    cri = _get(color, "cri_ra", "value")
    if cri is not None:
        index["nominal_cri_ra"] = cri

    eff = _get(phot, "luminaire_efficacy_lm_per_w", "value")
    if eff is not None:
        index["nominal_efficacy_lm_per_w"] = eff
    elif flux is not None and watts:
        index["nominal_efficacy_lm_per_w"] = round(flux / watts, 2)

    # Distribution projections.
    dist = phot.get("distribution_type")
    if dist:
        index["primary_distribution"] = dist
    od = outdoor.get("outdoor_distribution_type")
    if od:
        index["outdoor_distribution"] = od
    bf = phot.get("beam_family")
    if bf:
        index["beam_family"] = bf

    # Capability projections.
    ct = _get(cfg, "tested_axes", "color_tunability")
    if ct:
        index["color_tunability"] = ct
    dp = elec.get("driver_protocol")
    if dp:
        index["dimming_protocols"] = [dp]

    # Ingress and impact ratings.
    ip = _get(pf, "shared_mechanical", "ip_rating")
    if ip:
        index["ip_rating"] = ip
    ik = _get(pf, "shared_mechanical", "ik_rating")
    if ik:
        index["ik_rating"] = ik

    # BUG and glare.
    bug_string = _format_bug(outdoor.get("bug_rating"))
    if bug_string:
        index["bug_rating"] = bug_string
    ugr = _get(phot, "ugr_4h_8h", "value")
    if ugr is not None:
        index["ugr_4h_8h"] = ugr

    # Attestations rollup.
    programs = _collect_attestation_programs(record)
    if programs:
        index["attestation_programs"] = programs

    # Search keywords from family display + catalog line + scenario. Dedupe +
    # sort for deterministic output across runs. Skip whitespace-only values
    # so "   " does not land as an empty entry after strip().
    keywords = set()
    for candidate in [pf.get("family_display_name"), pf.get("catalog_line"), cfg.get("scenario_label")]:
        if candidate and isinstance(candidate, str):
            stripped = candidate.strip()
            if stripped:
                keywords.add(stripped)
    if keywords:
        index["search_keywords"] = sorted(keywords)

    # Source-file types rollup. Deduplicated sorted list of file_type values so
    # consumers can quickly check which source formats the record ships with.
    types_present = sorted({
        entry.get("file_type")
        for entry in (record.get("source_files") or [])
        if isinstance(entry, dict) and entry.get("file_type")
    })
    if types_present:
        index["source_file_types_present"] = types_present

    return index


def missing_required_keys(built: dict) -> list[str]:
    """Return the sorted list of Index.required keys absent from a built index."""
    return sorted(REQUIRED_INDEX_KEYS - set(built.keys()))


def _diff(a: dict, b: dict) -> list[str]:
    """Shallow per-key diff, good enough for mismatch reporting."""
    diffs = []
    keys = set(a.keys()) | set(b.keys())
    for k in sorted(keys):
        if a.get(k) != b.get(k):
            diffs.append(f"  {k}: stored={a.get(k)!r} != computed={b.get(k)!r}")
    return diffs


def main():
    ap = argparse.ArgumentParser(description="Build or verify a ULC record's index block.")
    ap.add_argument("record", type=Path, help="Path to the ULC record JSON file.")
    mode = ap.add_mutually_exclusive_group()
    mode.add_argument("--check", action="store_true", help="Verify existing index matches the builder output. Exit 1 on mismatch.")
    mode.add_argument("--stdout", action="store_true", help="Print the built index to stdout without modifying the record.")
    args = ap.parse_args()

    if not args.record.exists():
        print(f"error: {args.record} does not exist", file=sys.stderr)
        sys.exit(1)

    record = json.loads(args.record.read_text())
    built = build_index(record)

    # Refuse to proceed if the builder cannot derive every Index.required key.
    # This catches records whose deep blocks are too sparse to produce a
    # schema-valid index, which `--check` parity-diffing alone would miss when
    # both the stored and built indices are missing the same required key.
    missing = missing_required_keys(built)
    if missing:
        print(f"ERROR: builder cannot derive required index keys for {args.record}:", file=sys.stderr)
        for key in missing:
            src = REQUIRED_KEY_SOURCES.get(key, "(always-present marker)")
            print(f"  - {key}  (populate {src})", file=sys.stderr)
        sys.exit(1)

    if args.stdout:
        print(json.dumps(built, indent=2))
        return

    if args.check:
        stored = record.get("index") or {}
        diffs = _diff(stored, built)
        if diffs:
            print(f"Index drift in {args.record}:", file=sys.stderr)
            for d in diffs:
                print(d, file=sys.stderr)
            sys.exit(1)
        print(f"OK -- index in {args.record} matches builder {BUILDER_VERSION}.")
        return

    # Default: write in place.
    record["index"] = built
    args.record.write_text(json.dumps(record, indent=2) + "\n")
    print(f"OK -- wrote index (builder {BUILDER_VERSION}) to {args.record}.")


if __name__ == "__main__":
    main()
