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
# $GITHUB_ENV for scripts/macos/sign.sh. Without MACOS_SIGN_P12, or when it
# holds no usable Developer ID Application certificate (a warning says what it
# holds), the builds keep an ad-hoc signature.
set -euo pipefail

if [ -z "${MACOS_SIGN_P12:-}" ]; then
	echo "No Developer ID certificate in the secrets: the app gets an ad-hoc signature, the command line none."
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

# Apple's Developer ID intermediate certificates. A .p12 exported from Keychain
# Access holds only the certificate and its key; without the issuer, macOS
# calls the certificate untrusted and codesign cannot see it, and the runners
# do not always have the intermediates installed.
for ca in DeveloperIDG2CA DeveloperIDCA; do
	if curl -fsSL --retry 3 -o "$tmp/$ca.cer" "https://www.apple.com/certificateauthority/$ca.cer"; then
		security import "$tmp/$ca.cer" -k "$keychain" >/dev/null 2>&1 || true
	fi
done

identity=$(security find-identity -v -p codesigning "$keychain" | awk '/"Developer ID Application/ {print $2; exit}')
if [ -z "$identity" ]; then
	# Say what is there, with the names after the colon left out.
	echo "Certificates in the keychain made from MACOS_SIGN_P12 (with Apple's intermediates):"
	security find-certificate -a "$keychain" | awk -F'"' '/"labl"/ {print "  " $4}' | sed -E 's/: .*/: .../'
	echo "Signing identities (a certificate with its private key):"
	security find-identity -p codesigning "$keychain" | sed -E 's/"([^":]*): [^"]*"/"\1: ..."/; s/^/  /'
	# A release still gets every download, with the ad-hoc signature: the
	# warning shows on the run's summary page.
	echo "::warning::MACOS_SIGN_P12 holds no valid Developer ID Application certificate with its private key, so the macOS builds get an ad-hoc signature. No identity listed: the .p12 lacks the private key (export it in Keychain Access from My Certificates, where the key shows under the certificate). Another kind listed (Apple Development, Developer ID Installer): create a Developer ID Application certificate. CSSMERR_TP_NOT_TRUSTED: the issuer is missing (see docs/macos-signing.md)."
	security delete-keychain "$keychain" 2>/dev/null || true
	exit 0
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
