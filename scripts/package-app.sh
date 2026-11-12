#!/usr/bin/env bash
# Assemble dist/Mypack.app from bin/mypack (built by `make gui`) and, with
# "dmg" as second argument, a drag-to-Applications disk image.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"
VERSION="${1:-1.0.0}"
APP="dist/Mypack.app"
export GOTOOLCHAIN="${GOTOOLCHAIN:-auto}"

[ -x bin/mypack ] || { echo "bin/mypack missing; run make gui" >&2; exit 1; }
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp bin/mypack "$APP/Contents/MacOS/mypack"
[ -x bin/mypack-native-host ] && cp bin/mypack-native-host "$APP/Contents/MacOS/mypack-native-host"
go run ./scripts/macicon assets/AppIcon-1024.png assets/AppIcon-macOS-1024.png
go run ./scripts/mkicns assets/AppIcon-macOS-1024.png "$APP/Contents/Resources/AppIcon.icns"
mkdir -p "$APP/Contents/Resources/licenses"
cp LICENSE NOTICE "$APP/Contents/Resources/licenses/"
if bash scripts/bundle-runtime.sh "darwin/$(go env GOARCH)"; then
  mkdir -p "$APP/Contents/Resources/runtimes" "$APP/Contents/Resources/bin"
  cp resources/runtimes/* "$APP/Contents/Resources/runtimes/"
  helper="resources/darwin-$(go env GOARCH)/bin/mypack-runtime"
  if [ -f "$helper" ]; then
    cp "$helper" "$APP/Contents/Resources/bin/mypack-runtime"
  fi
else
  echo "warning: no bundled runtime; first build will download from the registry" >&2
fi
printf 'APPL????' > "$APP/Contents/PkgInfo"
cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>Mypack</string>
  <key>CFBundleDisplayName</key><string>Mypack</string>
  <key>CFBundleIdentifier</key><string>dev.mypack.builder</string>
  <key>CFBundleExecutable</key><string>mypack</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
  <key>CFBundleShortVersionString</key><string>${VERSION}</string>
  <key>CFBundleVersion</key><string>${VERSION}</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>LSApplicationCategoryType</key><string>public.app-category.developer-tools</string>
  <key>NSPrincipalClass</key><string>NSApplication</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>NSSupportsAutomaticGraphicsSwitching</key><true/>
  <key>CFBundleURLTypes</key>
  <array><dict>
    <key>CFBundleURLName</key><string>dev.mypack.builder</string>
    <key>CFBundleURLSchemes</key><array><string>mypack</string></array>
  </dict></array>
  <key>NSHumanReadableCopyright</key><string>© 2026 Mypack. All rights reserved.</string>
</dict>
</plist>
PLIST
# SIGN_IDENTITY: "-" (ad-hoc, default) or a Developer ID Application identity.
SIGN_IDENTITY="${SIGN_IDENTITY:--}"
if [ "$SIGN_IDENTITY" = "-" ]; then
  if [ -f "$APP/Contents/Resources/bin/mypack-runtime" ]; then
    codesign --force --sign - --timestamp=none "$APP/Contents/Resources/bin/mypack-runtime"
  fi
  codesign --force --deep --sign - --timestamp=none "$APP"
else
  # Inside-out: helper executables first, then the bundle; hardened runtime for notarization.
  for bin in "$APP/Contents/Resources/bin/mypack-runtime" "$APP/Contents/MacOS/mypack-native-host" "$APP/Contents/MacOS/mypack"; do
    [ -f "$bin" ] && codesign --force --options runtime --timestamp --sign "$SIGN_IDENTITY" "$bin"
  done
  codesign --force --options runtime --timestamp --sign "$SIGN_IDENTITY" "$APP"
fi
codesign --verify --deep --strict "$APP"
echo "built $APP ($(du -sh "$APP" | cut -f1))"

if [ "${2:-}" = "dmg" ]; then
  STAGE="$(mktemp -d "$ROOT/dist/.dmg.XXXX")"
  cp -R "$APP" "$STAGE/"
  ln -s /Applications "$STAGE/Applications"
  rm -f "dist/Mypack-$VERSION.dmg"
  hdiutil create -volname "Mypack" -srcfolder "$STAGE" -ov -format UDZO "dist/Mypack-$VERSION.dmg" >/dev/null
  rm -rf "$STAGE"
  if [ "$SIGN_IDENTITY" != "-" ]; then
    codesign --force --timestamp --sign "$SIGN_IDENTITY" "dist/Mypack-$VERSION.dmg"
  fi
  echo "built dist/Mypack-$VERSION.dmg"
  # NOTARY_PROFILE: a profile stored with `xcrun notarytool store-credentials`.
  if [ -n "${NOTARY_PROFILE:-}" ]; then
    xcrun notarytool submit "dist/Mypack-$VERSION.dmg" --keychain-profile "$NOTARY_PROFILE" --wait
    xcrun stapler staple "dist/Mypack-$VERSION.dmg"
    xcrun stapler validate "dist/Mypack-$VERSION.dmg"
    spctl --assess --type open --context context:primary-signature -v "dist/Mypack-$VERSION.dmg"
  fi
fi
