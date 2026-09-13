#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
  echo "usage: $0 RELEASE_VERSION" >&2
  exit 2
fi

release_version=$1
constant_file=tools/validator/internal/sheet/specversion.go
line=$(grep -E '^const SpecVersion = "[0-9]+\.[0-9]+\.[0-9]+"$' "$constant_file" || true)
if [ "$(printf '%s\n' "$line" | grep -c .)" -ne 1 ]; then
  echo "SpecVersion check could not read exactly one version from $constant_file" >&2
  exit 1
fi
spec_version=$(printf '%s\n' "$line" | sed -E 's/^const SpecVersion = "([^"]+)"$/\1/')

if [ "$release_version" != "$spec_version" ]; then
  echo "SpecVersion mismatch: release version $release_version, converter SpecVersion $spec_version" >&2
  exit 1
fi

echo "SpecVersion $spec_version matches release version $release_version"
