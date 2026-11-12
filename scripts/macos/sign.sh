#!/usr/bin/env bash
# Sign a macOS program or .app bundle and, when the notary key is set up,
# notarize it (and staple the ticket to an .app).
#
#   scripts/macos/sign.sh PATH [GOOS]
#
# With MACOS_SIGN_IDENTITY (see signing-setup.sh) the signature is the Developer
# ID, with the hardened runtime, a secure timestamp and entitlements.plist. Without
# it an .app gets an ad-hoc signature, so macOS offers "Open Anyway" instead of
# calling it damaged, and a bare program is left as the Go linker made it.
# GOOS lets goreleaser call this for every build: anything but darwin is skipped.
# Notarizing needs either MACOS_NOTARY_PROFILE, a profile saved on this Mac with
# "xcrun notarytool store-credentials" (handy on your own Mac), or the API key
# in MACOS_NOTARY_KEY_PATH, MACOS_NOTARY_KEY_ID and MACOS_NOTARY_ISSUER_ID (CI).
set -euo pipefail

path=$1
goos=${2:-darwin}
[ "$goos" = darwin ] || exit 0
if ! command -v codesign >/dev/null; then
	echo "sign.sh: no codesign on this machine, $path stays as it is"
	exit 0
fi
here=$(cd "$(dirname "$0")" && pwd)

if [ -z "${MACOS_SIGN_IDENTITY:-}" ]; then
	case $path in
	*.app) codesign --force --sign - "$path" ;;
	esac
	exit 0
fi

codesign --force --timestamp --options runtime --entitlements "$here/entitlements.plist" \
	--sign "$MACOS_SIGN_IDENTITY" "$path"
codesign --verify --strict --verbose=1 "$path"
echo "Signed $path with the Developer ID."

if [ -n "${MACOS_NOTARY_PROFILE:-}" ]; then
	notary=(--keychain-profile "$MACOS_NOTARY_PROFILE")
elif [ -n "${MACOS_NOTARY_KEY_PATH:-}" ]; then
	notary=(--key "$MACOS_NOTARY_KEY_PATH" --key-id "${MACOS_NOTARY_KEY_ID:?}" --issuer "${MACOS_NOTARY_ISSUER_ID:?}")
else
	echo "sign.sh: no notary profile or key, $path is signed but not notarized"
	exit 0
fi
work=$(mktemp -d)
zip=$work/$(basename "$path").zip
ditto -c -k --keepParent "$path" "$zip"
out=$(xcrun notarytool submit "$zip" "${notary[@]}" --wait --timeout 30m --output-format json)
echo "$out"
if ! grep -Eq '"status" *: *"Accepted"' <<<"$out"; then
	id=$(sed -nE 's/.*"id" *: *"([^"]+)".*/\1/p' <<<"$out")
	[ -z "$id" ] || xcrun notarytool log "$id" "${notary[@]}" || true
	echo "::error::Apple did not accept $path for notarization"
	exit 1
fi
rm -rf "$work"
case $path in
*.app)
	xcrun stapler staple "$path"
	spctl --assess --type execute "$path"
	echo "Notarized $path; Gatekeeper accepts it."
	;;
*) echo "Notarized $path (a bare program cannot carry the ticket; macOS looks it up online)." ;;
esac
