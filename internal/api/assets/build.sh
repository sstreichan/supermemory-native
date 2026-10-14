#!/usr/bin/env bash
# Builds the dashboard assets with Bun and enforces the Phase 8 bundle gates.
#
#   ./build.sh
#
# Requires the Bun runtime (https://bun.sh). The generated index.html and
# local-console.js are committed so Go builds never need Bun.
set -euo pipefail

cd "$(dirname "${BASH_SOURCE[0]}")"

bun run build

SIZE=$(wc -c < local-console.js | tr -d '[:space:]')
LIMIT=51200 # 50 KB hard gate (Phase 2 target: < 40 KB)

echo "local-console.js: ${SIZE} bytes"
echo "index.html:       $(wc -c < index.html | tr -d '[:space:]') bytes"

if [ "$SIZE" -ge "$LIMIT" ]; then
  echo "ERROR: local-console.js is ${SIZE} bytes, at or above the ${LIMIT} byte gate." >&2
  exit 1
fi

# A bundle that Bun tree-shook down to its empty IIFE wrapper is tiny but
# useless, so require a real API path to be present as well.
if ! grep -q "/v3/documents/documents" local-console.js; then
  echo "ERROR: local-console.js is missing the /v3/documents/documents API path." >&2
  exit 1
fi
if ! grep -q "/v4/search" local-console.js; then
  echo "ERROR: local-console.js is missing the /v4/search API path." >&2
  exit 1
fi

echo "build ok"
