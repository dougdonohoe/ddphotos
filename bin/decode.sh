#!/usr/bin/env bash
# Decode one or more encrypted .enc.json files produced by photogen.
#
# Usage:
#   bin/decode.sh <file> [file ...]
#
# Example:
#   bin/decode.sh web/albums/my-site/my-album/index.enc.json
#   bin/decode.sh web/albums/my-site/albums.enc.json web/albums/my-site/my-album/index.enc.json

set -eo pipefail

if [[ $# -eq 0 ]]; then
    echo "Usage: bin/decode.sh <file> [file ...]" >&2
    exit 1
fi

for file in "$@"; do
    if [[ $# -gt 1 ]]; then
        echo "=== $file ==="
    fi
    go run cmd/decode/decode.go "$file"
done
