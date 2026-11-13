#!/usr/bin/env bash
# Build Briss.app on a Mac, the same way the GUI workflow does: the program,
# the icon, Info.plist, the signature, and a zip like the release download.
#
#   scripts/macos/build-app.sh [ARCH] [VERSION]
#
# ARCH is arm64 (Apple silicon) or amd64 (Intel) and defaults to this Mac's;
# VERSION (for example v2.0.0-beta) goes into Info.plist as plain numbers.
# The app lands in dist/<arch>/Briss.app and the zip in
# dist/Briss-macos-<apple-silicon|intel>.zip.
#
# Signing, through sign.sh: with nothing set the app gets an ad-hoc signature,
# so this Mac opens it straight away. With MACOS_SIGN_IDENTITY set to your
# certificate ("Developer ID Application: Your Name (TEAMID)", as Keychain
# Access shows it) it is signed with the Developer ID, and with
# MACOS_NOTARY_PROFILE (a profile saved by "xcrun notarytool store-credentials")
# it is also notarized and stapled. docs/macos-signing.md has the details.
#
# Needs Go and Xcode's command line tools (xcode-select --install).
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
case $(uname -m) in arm64) host=arm64 ;; *) host=amd64 ;; esac
arch=${1:-$host}
version=${2:-2.0.0}
version=${version#v}
version=${version%%-*} # macOS wants plain numbers: 2.0.0-beta is 2.0.0
case $version in [0-9]*) ;; *) version=2.0.0 ;; esac
case $arch in
arm64) suffix=apple-silicon ;;
amd64) suffix=intel ;;
*)
	echo "build-app.sh: ARCH must be arm64 or amd64, not $arch" >&2
	exit 2
	;;
esac

app=$root/dist/$arch/Briss.app
rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
(cd "$root/gui" && CGO_ENABLED=1 GOOS=darwin GOARCH=$arch go build -trimpath -ldflags "-s -w" -o "$app/Contents/MacOS/Briss" .)
iconutil -c icns "$root/assets/logo/Briss.iconset" -o "$app/Contents/Resources/Briss.icns"
cat >"$app/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleName</key><string>Briss</string>
  <key>CFBundleDisplayName</key><string>Briss</string>
  <key>CFBundleExecutable</key><string>Briss</string>
  <key>CFBundleIconFile</key><string>Briss</string>
  <key>CFBundleIdentifier</key><string>io.github.jarofclay.briss</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$version</string>
  <key>CFBundleVersion</key><string>$version</string>
  <key>LSMinimumSystemVersion</key><string>12.0</string>
  <key>NSHighResolutionCapable</key><true/>
  <key>CFBundleDocumentTypes</key><array><dict>
    <key>CFBundleTypeName</key><string>PDF document</string>
    <key>CFBundleTypeExtensions</key><array><string>pdf</string></array>
    <key>LSItemContentTypes</key><array><string>com.adobe.pdf</string></array>
    <key>CFBundleTypeRole</key><string>Viewer</string>
  </dict></array>
</dict></plist>
PLIST

# Developer ID and notarization when they are set up; otherwise an ad-hoc
# seal, without which macOS calls a downloaded copy damaged.
"$root/scripts/macos/sign.sh" "$app"
codesign --verify --strict --verbose=2 "$app"
codesign --display --verbose=2 "$app" 2>&1 | grep -E '^(Identifier|Format|Signature|Sealed Resources|Runtime Version)' || true

zip=$root/dist/Briss-macos-$suffix.zip
rm -f "$zip"
(cd "$root/dist/$arch" && ditto -c -k --keepParent Briss.app "$zip")
echo "Built $app"
echo "Zipped $zip"
