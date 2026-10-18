#!/bin/sh
# Rebuild the Windows amd64 executable icon from assets/icons/goldminer.ico.
# Generator: https://github.com/akavel/rsrc/tree/v0.10.2
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo_root"

icon=assets/icons/goldminer.ico
output=cmd/goldminer/icon_windows_amd64.syso
if [ ! -f "$icon" ]; then
    printf 'Missing icon: %s\n' "$icon" >&2
    exit 1
fi

temporary=$(mktemp "${output}.XXXXXX")
trap 'rm -f "$temporary"' EXIT HUP INT TERM

# Run the generator on the host, even when GOOS/GOARCH select a cross-build.
GOOS=$(go env GOHOSTOS) GOARCH=$(go env GOHOSTARCH) \
    go run github.com/akavel/rsrc@v0.10.2 \
    -arch amd64 -ico "$icon" -o "$temporary"
chmod 644 "$temporary"
mv "$temporary" "$output"
printf 'Generated %s\n' "$output"
