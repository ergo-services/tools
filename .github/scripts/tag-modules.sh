#!/usr/bin/env bash
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
source .github/scripts/modules.sh

BASE_REF="${1:-origin/master}"
HEAD_REF="${2:-HEAD}"
valid_ref "$BASE_REF"
valid_ref "$HEAD_REF"

if [[ -z $(git config user.email 2>/dev/null) ]]; then
    git config user.email "github-actions[bot]@users.noreply.github.com"
    git config user.name "github-actions[bot]"
fi

created=()
failed=()

while IFS= read -r dir; do
    module="${dir#./}"

    if module_changed "$BASE_REF" "$HEAD_REF" "$module"; then
        :
    else
        continue
    fi

    if ! version=$(module_version "$module"); then
        failed+=("$module: no usable VERSION")
        continue
    fi

    if [[ "$version" == *-rc.* ]]; then
        echo "$module: skipping prerelease $version"
        continue
    fi

    tag="$(tag_prefix "$module")$version"

    if out=$(git tag -a "$tag" "$HEAD_REF" -m "Release $tag" 2>&1); then
        created+=("$tag")
        echo "tagged $tag"
    elif grep -q 'already exists' <<< "$out"; then
        echo "$tag already exists"
    else
        failed+=("$tag: $out")
    fi
done < <(module_dirs)

if [[ ${#created[@]} -gt 0 ]]; then
    git push origin "${created[@]}"
fi

if [[ ${#failed[@]} -gt 0 ]]; then
    printf 'failed: %s\n' "${failed[@]}" >&2
    exit 1
fi
