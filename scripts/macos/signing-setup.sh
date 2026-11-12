#!/usr/bin/env bash
# Set up Developer ID signing on a macOS runner, from repository secrets:
#
#   MACOS_SIGN_P12       the "Developer ID Application" certificate with its
#                        private key, exported as .p12, base64-encoded
#   MACOS_SIGN_PASSWORD  the password of that .p12
#   MACOS_NOTARY_KEY     an App Store Connect API key (the AuthKey_XXXX.p8 file),
#                        base64-encoded or as is
#
# The certificate goes into a throwaway keychain and the key into a file in
# $RUNNER_TEMP; MACOS_SIGN_IDENTITY and MACOS_NOTARY_KEY_PATH are written to
# $GITHUB_ENV for scripts/macos/sign.sh. Without MACOS_SIGN_P12 this does
# nothing and the builds keep an ad-hoc signature.
set -euo pipefail

if [ -z "${MACOS_SIGN_P12:-}" ]; then
	echo "No Developer ID certificate in the secrets: the builds get an ad-hoc signature."
	exit 0
fi
tmp=${RUNNER_TEMP:?}
keychain=$tmp/briss-signing.keychain-db
pass=$(openssl rand -base64 24)

printf '%s' "$MACOS_SIGN_P12" | base64 --decode >"$tmp/developer-id.p12"
security create-keychain -p "$pass" "$keychain"
security set-keychain-settings -lut 21600 "$keychain"
security unlock-keychain -p "$pass" "$keychain"
security import "$tmp/developer-id.p12" -k "$keychain" -P "${MACOS_SIGN_PASSWORD:-}" -f pkcs12 -T /usr/bin/codesign
security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k "$pass" "$keychain" >/dev/null
# shellcheck disable=SC2046 # the keychain list is several words on purpose
security list-keychains -d user -s "$keychain" $(security list-keychains -d user | tr -d '"')
rm -f "$tmp/developer-id.p12"

identity=$(security find-identity -v -p codesigning "$keychain" | awk '/"Developer ID Application/ {print $2; exit}')
if [ -z "$identity" ]; then
	echo "::error::MACOS_SIGN_P12 holds no Developer ID Application certificate with its private key"
	exit 1
fi
echo "MACOS_SIGN_IDENTITY=$identity" >>"$GITHUB_ENV"
echo "Developer ID certificate ready."

if [ -n "${MACOS_NOTARY_KEY:-}" ]; then
	key=$tmp/notary-key.p8
	case $MACOS_NOTARY_KEY in
	-----BEGIN*) printf '%s\n' "$MACOS_NOTARY_KEY" >"$key" ;;
	*) printf '%s' "$MACOS_NOTARY_KEY" | base64 --decode >"$key" ;;
	esac
	echo "MACOS_NOTARY_KEY_PATH=$key" >>"$GITHUB_ENV"
	echo "Notary key ready."
else
	echo "No notary key in the secrets: the builds are signed but not notarized."
fi
