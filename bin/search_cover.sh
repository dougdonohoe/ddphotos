#!/usr/bin/env bash
# Find the fileName for a photo given its URL.
#
# Usage:
#   bin/search_cover.sh <url>
#
# Example:
#   bin/search_cover.sh http://localhost:5173/albums/banff-2002/full/0918bedf-2f7d-dedc-9e89-b99ec5bb2752.webp

set -eo pipefail

if [[ $# -ne 1 ]]; then
    echo "Usage: bin/search_cover.sh <url>" >&2
    exit 1
fi

url="$1"

# Extract slug (e.g. "banff-2002") and src path (e.g. "full/0918bedf-....webp")
# URL form: .../albums/{slug}/full/{uuid}.webp  or  .../albums/{slug}/grid/{uuid}.webp
slug=$(echo "$url" | sed -E 's|.*/albums/([^/]+)/.*|\1|')
src_path=$(echo "$url" | awk -F'/albums/[^/]+/' '{print $2}')

if [[ -z "$slug" || -z "$src_path" ]]; then
    echo "Could not parse slug or src path from URL: $url" >&2
    exit 1
fi

# Find index.enc.json (or index.json) for this album under albums/
index_file=$(find albums -maxdepth 3 -type f \( -name "index.enc.json" -o -name "index.json" \) \
    | grep "/${slug}/" | head -1)

if [[ -z "$index_file" ]]; then
    echo "No index file found for album slug: $slug" >&2
    exit 1
fi

echo "Album:  $slug"
echo "Index:  $index_file"
echo "src:    $src_path"
echo ""

# Decode (handles both encrypted and plain JSON) and search for the src path
decoded=$(go run cmd/decode/decode.go "$index_file" 2>/dev/null || cat "$index_file")

# Extract the fileName for the matching src path (matches full or grid)
echo "$decoded" | python3 -c "
import sys, json
data = json.load(sys.stdin)
photos = data.get('photos', [])
src = sys.argv[1]
for p in photos:
    srcs = p.get('src', {})
    if srcs.get('full') == src or srcs.get('grid') == src:
        print('fileName: ' + p.get('fileName', ''))
        print('id:       ' + p.get('id', ''))
        sp = p.get('sourcePath', '')
        if sp:
            print('sourcePath: ' + sp)
        sys.exit(0)
print('src path not found in index', file=sys.stderr)
sys.exit(1)
" "$src_path"
