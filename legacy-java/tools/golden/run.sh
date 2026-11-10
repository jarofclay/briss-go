#!/usr/bin/env bash
# Regenerates testdata/golden/*.json from the Java Briss build.
#
# Usage: legacy-java/tools/golden/run.sh [CLASSPATH]
# CLASSPATH defaults to the shadow jar built by `./gradlew shadowJar` in
# legacy-java/ (it contains Briss and all its dependencies). A JDK 11 to 19
# runs Gradle 7.6; any JDK 8+ can run this script.
set -euo pipefail
here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/../../.." && pwd)"
cp="${1:-$(ls "$root"/legacy-java/build/libs/*-all.jar 2>/dev/null | head -1)}"
if [ -z "$cp" ]; then
  echo "build the shadow jar first: (cd legacy-java && ./gradlew shadowJar), or pass a classpath" >&2
  exit 1
fi
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
javac -nowarn -cp "$cp" -d "$work/classes" "$here/GoldenDump.java"
run() { java -Djava.awt.headless=true -cp "$work/classes:$cp" GoldenDump "$@"; }
out="$root/testdata/golden"
mkdir -p "$out"
run geom > "$out/geom.json"
for spec in "example.pdf -" "example-protected.pdf secret" "CREATIVE_COMMONS.pdf -" "book300.pdf -"; do
  set -- $spec
  run pdf "$root/testdata/pdfs/$1" "$2" "$work/out" > "$out/${1%.pdf}.json"
  echo "wrote $out/${1%.pdf}.json"
done
