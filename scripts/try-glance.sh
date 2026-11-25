#!/usr/bin/env bash
# Try Glance (window.glance) without publishing a runtime: builds the full
# runtime from this checkout, wraps it with a mypack.json in a throwaway app
# under dist/try/, and opens it.
#
# Usage:
#   scripts/try-glance.sh                          # Wikipedia, defaults
#   scripts/try-glance.sh https://news.ycombinator.com
#   TRIGGER=alt WIDTH=90 ANIMATION=relaxed EXTERNAL=1 scripts/try-glance.sh
#   TABS=1 scripts/try-glance.sh                   # in a tabbed window
#
# Environment:
#   TRIGGER    alt (default; Option on a Mac) | shift | ctrl | meta
#   WIDTH      card width in percent, 50..95 (default 80)
#   ANIMATION  smooth (default) | quick | relaxed | off
#   EXTERNAL   1 opens links that leave the site in a glance too
#   TABS       1 gives the window a tab bar
#   TITLEBAR   default (default) | hidden | overlay | glass
#
# Once it is open, its Settings (⌘,) has a Glance pane with the same choices.
# The app's logs go to dist/try/glance.log.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}" CGO_ENABLED=0

URL="${1:-https://en.wikipedia.org/wiki/Web_browser}"
TRIGGER="${TRIGGER:-alt}"
WIDTH="${WIDTH:-80}"
ANIMATION="${ANIMATION:-smooth}"
EXTERNAL="${EXTERNAL:-0}"
TABS="${TABS:-0}"
TITLEBAR="${TITLEBAR:-default}"
OUT="$ROOT/dist/try"
NAME="Glance Try"
ID="dev.mypack.glance-try"

bool() { [ "$1" = 1 ] && echo true || echo false; }
CONFIG=$(cat <<JSON
{
  "schemaVersion": 1,
  "app": {"name": "$NAME", "identifier": "$ID", "profileId": "$ID", "version": "1.0.0",
          "source": {"type": "url", "value": "$URL"}, "icon": {}},
  "runtime": {"version": "1.0.13"},
  "window": {"width": 1200, "height": 800, "titleBarStyle": "$TITLEBAR", "tabs": $(bool "$TABS"),
             "glance": {"enabled": true, "trigger": "$TRIGGER", "width": $WIDTH,
                        "animation": "$ANIMATION", "externalLinks": $(bool "$EXTERNAL")}},
  "webview": {},
  "build": {}
}
JSON
)

mkdir -p "$OUT"
printf '%s\n' "$CONFIG" > "$OUT/mypack.json"
echo "Checking the configuration…"
go run ./cmd/mypack-cli project validate "$OUT/mypack.json"

echo "Building the runtime…"
pkill -f "Glance Try.app/Contents/MacOS/GlanceTry" 2>/dev/null || true; pkill -f "$OUT/glance-try" 2>/dev/null || true; sleep 0.5
case "$(go env GOOS)" in
darwin)
  APP="$OUT/$NAME.app"
  rm -rf "$APP"
  mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
  go build -o "$APP/Contents/MacOS/GlanceTry" ./runtime/cmd/mypack-runtime
  cp "$OUT/mypack.json" "$APP/Contents/Resources/mypack.json"
  cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleExecutable</key><string>GlanceTry</string>
  <key>CFBundleIdentifier</key><string>$ID</string>
  <key>CFBundleName</key><string>$NAME</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>1.0.0</string>
  <key>NSHighResolutionCapable</key><true/>
</dict></plist>
PLIST
  codesign --force -s - "$APP" >/dev/null 2>&1 || true
  "$APP/Contents/MacOS/GlanceTry" >"$OUT/glance.log" 2>&1 &
  ;;
*)
  go build -o "$OUT/glance-try" ./runtime/cmd/mypack-runtime
  MYPACK_CONFIG="$OUT/mypack.json" "$OUT/glance-try" >"$OUT/glance.log" 2>&1 &
  ;;
esac

case "$TRIGGER" in
alt) KEY="Option (Alt)" ;;
ctrl) KEY="Control" ;;
meta) KEY="Command" ;;
*) KEY="Shift" ;;
esac
cat <<EOF

Opened "$NAME" on $URL
  • Hold $KEY and click a link: it opens in a glance.
  • Esc, a click beside the card, or ✕ closes it; ⤢ makes it the page; 🔗 copies the link.
  • Settings (⌘,) → Glance changes the key, outside links, card width and animation.
Logs: $OUT/glance.log
EOF
