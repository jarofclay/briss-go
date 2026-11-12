#!/usr/bin/env bash
# Run a briss program built for macOS once on a test PDF, to check that the PDF
# engine works under its signature (the hardened runtime blocks the code the
# engine compiles unless entitlements.plist is in the signature).
#
#   scripts/macos/check-engine.sh PROGRAM [GOOS GOARCH]
#
# Skipped unless this is a Mac that can run GOOS/GOARCH natively.
set -euo pipefail

prog=$1
goos=${2:-darwin}
goarch=${3:-}
[ "$goos" = darwin ] && [ "$(uname -s)" = Darwin ] || exit 0
case $(uname -m) in arm64) host=arm64 ;; x86_64) host=amd64 ;; *) host=unknown ;; esac
if [ -n "$goarch" ] && [ "$goarch" != "$host" ]; then
	echo "check-engine.sh: $prog is for $goarch, this Mac is $host; not run"
	exit 0
fi
root=$(cd "$(dirname "$0")/../.." && pwd)
"$prog" detect "$root/testdata/pdfs/example.pdf" >/dev/null
echo "check-engine.sh: the PDF engine runs in $prog under its signature"
