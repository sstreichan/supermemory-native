#!/usr/bin/env bash
# Captures dashboard screenshots at fixed viewports for the before/after design check.
#
#   ./capture.sh before            # baseline: renders the reference assets (docs/supermemory/)
#   ./capture.sh after [BASE_URL]  # result:   renders the native daemon (default http://127.0.0.1:6767)
#
# Output: docs/dashboard-design/before/ or docs/dashboard-design/after/
#   overview-desktop.png (1440x2600), memory-desktop.png (1440x1600), overview-mobile.png (390x1500)
#
# Set CHROME to override the browser binary path.
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO="$(cd "$HERE/../.." && pwd)"
CHROME="${CHROME:-/c/Program Files/Google/Chrome/Application/chrome.exe}"
REF_PORT="${REF_PORT:-8799}"

if [ ! -x "$CHROME" ] && ! command -v "$CHROME" >/dev/null 2>&1; then
  echo "Chrome not found at '$CHROME' — set CHROME=/path/to/chrome" >&2
  exit 1
fi

# Chrome is a native binary on Windows; hand it a Windows-style absolute path.
win_path() { (cd "$1" && pwd -W); }

shoot() { # <url> <WxH> <outfile>
  "$CHROME" --headless=new --disable-gpu --hide-scrollbars --no-sandbox \
    --force-device-scale-factor=1 --window-size="$2" --virtual-time-budget=15000 \
    --screenshot="$(win_path "$(dirname "$3")")/$(basename "$3")" "$1" >/dev/null 2>&1
  echo "wrote $3"
}

MODE="${1:-before}"
if [ "$MODE" = "before" ]; then
  BASE="http://127.0.0.1:$REF_PORT"
  OUT="$HERE/before"
  REF_ROOT="$REPO/docs/supermemory" REF_PORT="$REF_PORT" python "$HERE/reference_server.py" >/dev/null 2>&1 &
  SERVER_PID=$!
  trap 'kill "$SERVER_PID" 2>/dev/null || true' EXIT
  sleep 2
else
  BASE="${2:-http://127.0.0.1:6767}"
  OUT="$HERE/after"
fi
mkdir -p "$OUT"

shoot "$BASE/"         1440,2600 "$OUT/overview-desktop.png"
shoot "$BASE/#memory"  1440,1600 "$OUT/memory-desktop.png"
shoot "$BASE/"          390,1500 "$OUT/overview-mobile.png"
