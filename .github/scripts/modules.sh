#!/usr/bin/env bash
set -euo pipefail

valid_ref() {
    if [[ ! "$1" =~ ^[a-zA-Z0-9/_.^~-]+$ ]]; then
        echo "invalid ref: $1" >&2
        exit 1
    fi
}

module_dirs() {
    # A module under example/ is an illustration that builds against its parent through a
    # replace directive. It is never published, so it is neither tagged nor validated, and
    # demanding a VERSION of it would fail every run.
    find . -name go.mod -not -path './.git/*' \
        -not -path '*/example/*' -not -path '*/examples/*' \
        -exec dirname {} \; | sort
}

module_changed() {
    local base="$1" head="$2" module="$3"
    [[ -n $(git diff --name-only "$base...$head" -- "$module" 2>/dev/null) ]]
}

module_version() {
    local file="$1/VERSION"
    [[ -f "$file" ]] || return 1
    local version
    version=$(tr -d '[:space:]' < "$file")
    [[ -n "$version" ]] || return 1
    printf '%s' "$version"
}

tag_prefix() {
    [[ "$1" == "." ]] && return 0
    printf '%s/' "$1"
}

version_key() {
    local v="${1#v}"
    v="${v%%-*}"
    local major minor patch
    IFS='.' read -r major minor patch <<< "$v"
    printf '%d%03d%03d' "${major:-0}" "${minor:-0}" "${patch:-0}"
}
