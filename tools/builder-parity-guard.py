#!/usr/bin/env python3
"""
ULC builder-schema parity guard.

Verifies that `tools/build-index.py` and `schema/ulc.schema.json` agree on the
shape of the `index` block. Catches two classes of drift:

  1. Builder emits a field the schema does not declare. An unknown key ends up
     on records and downstream validators reject them.

  2. Schema marks a field required but the builder does not produce it under
     any input. The builder could silently produce records that fail core-level
     validation.

Runs in CI on every pull request touching schema/ or tools/build-index.py, and
locally via the pre-commit hook. Exit code 0 on parity, 1 on mismatch.

Usage:

    python3 tools/builder-parity-guard.py
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

SCHEMA_PATH = Path(__file__).resolve().parent.parent / "schema" / "ulc.schema.json"

# Python cannot import a module whose filename uses hyphens, so load via importlib.util.
import importlib.util

_spec = importlib.util.spec_from_file_location(
    "build_index", Path(__file__).resolve().parent / "build-index.py"
)
_module = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(_module)
build_index = _module.build_index
BUILDER_VERSION = _module.BUILDER_VERSION


def _maximal_fixture() -> dict:
    """A synthetic record that exercises every deep-block path the builder reads.

    Used to compute the set of keys the builder emits under maximal input. The
    values do not have to be realistic; they only have to traverse every path
    the builder consults. Keep this aligned with build_index().
    """
    return {
        "record_id": "test-record",
        "product_family": {
            "family_id": "test-fam",
            "family_display_name": "Test Family",
            "catalog_line": "TF",
            "catalog_model": "TF-100",
            "primary_category": "downlight",
            "secondary_function": ["wall_washing"],
            "indoor_outdoor": "indoor",
            "mounting_types": ["recessed_ceiling"],
            "environment_rating": "indoor_dry",
            "manufacturer": {"slug": "testmfr", "display_name": "Test Manufacturer"},
            "shared_mechanical": {"ip_rating": "IP20", "ik_rating": "IK06"},
            "shared_attestations": [{"program": "ul_listed", "value_type": "rated"}],
            "cutsheet": {"filename": "x.pdf", "sha256": "a" * 64},
        },
        "configuration": {
            "photometric_scenario_id": "test-scenario",
            "scenario_label": "Test Scenario",
            "catalog_number": "TF-100-30-90",
            "tested_axes": {"color_tunability": "static_white"},
            "tested_conditions": {"nominal_cct_at_test": 3000},
        },
        "electrical": {
            "input_power_w": {"value": 17.0, "value_type": "measured"},
            "driver_protocol": "0-10v",
        },
        "photometry": {
            "total_luminous_flux_lm": {"value": 1301, "value_type": "measured"},
            "luminaire_efficacy_lm_per_w": {"value": 77, "value_type": "measured"},
            "distribution_type": "type_v",
            "beam_family": "medium_flood",
            "ugr_4h_8h": {"value": 19, "value_type": "rated"},
        },
        "colorimetry": {"cri_ra": {"value": 92, "value_type": "measured"}},
        "outdoor_classification": {
            "outdoor_distribution_type": "type_iii",
            "bug_rating": {"b": 1, "u": 0, "g": 1},
        },
        "attestations": [{"program": "lm_79_24", "value_type": "measured"}],
        "sustainability_declaration": {"declaration_type": "ilfi_declare"},
        "source_files": [
            {"file_type": "datasheet_pdf", "reference": {"filename": "x.pdf", "sha256": "a" * 64}},
            {"file_type": "ies", "reference": {"filename": "x.ies", "sha256": "b" * 64}},
        ],
    }


def main():
    schema = json.loads(SCHEMA_PATH.read_text())
    index_def = schema.get("$defs", {}).get("Index", {})
    schema_props = set(index_def.get("properties", {}).keys())
    schema_required = set(index_def.get("required", []))

    fixture = _maximal_fixture()
    emitted = build_index(fixture)
    emitted_keys = set(emitted.keys())

    errors = []

    # Check 1: every emitted key is declared in the schema.
    unknown = emitted_keys - schema_props
    if unknown:
        errors.append(
            f"Builder emits {len(unknown)} key(s) not declared in schema Index.properties: "
            + ", ".join(sorted(unknown))
        )

    # Check 2: every required schema key is produced by the builder under maximal input.
    missing_required = schema_required - emitted_keys
    if missing_required:
        errors.append(
            f"Schema requires {len(missing_required)} key(s) the builder does not produce: "
            + ", ".join(sorted(missing_required))
        )

    if errors:
        print(f"Builder-schema parity FAILED ({len(errors)} issue(s)):", file=sys.stderr)
        for e in errors:
            print(f"  {e}", file=sys.stderr)
        sys.exit(1)

    print(
        f"OK -- builder emits {len(emitted_keys)} key(s); "
        f"schema declares {len(schema_props)} Index property/ies; "
        f"{len(schema_required)} required key(s) all produced. "
        f"Builder version: {BUILDER_VERSION}."
    )


if __name__ == "__main__":
    main()
