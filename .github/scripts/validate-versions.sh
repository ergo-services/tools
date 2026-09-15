#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
source .github/scripts/modules.sh

BASE_REF="${1:-origin/master}"
HEAD_REF="${2:-HEAD}"
valid_ref "$BASE_REF"
valid_ref "$HEAD_REF"

errors=0

while IFS= read -r dir; do
    module="${dir#./}"

    if module_changed "$BASE_REF" "$HEAD_REF" "$module"; then
        :
    else
        continue
    fi

    if ! version=$(module_version "$module"); then
        echo "$module changed but has no usable VERSION" >&2
        errors=$((errors + 1))
        continue
    fi

    if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?$ ]]; then
        echo "$module: $version is not vX.Y.Z or vX.Y.Z-rc.N" >&2
        errors=$((errors + 1))
        continue
    fi

    prefix=$(tag_prefix "$module")
    latest=$(git tag -l "${prefix}v*" --sort=-version:refname | head -1)
    [[ -n "$latest" ]] || continue

    if [[ $(version_key "$version") -le $(version_key "${latest#"$prefix"}") ]]; then
        echo "$module: VERSION $version is not above the latest tag $latest" >&2
        errors=$((errors + 1))
        continue
    fi

    echo "$module: $version"
done < <(module_dirs)

if [[ $errors -gt 0 ]]; then
    exit 1
fi
