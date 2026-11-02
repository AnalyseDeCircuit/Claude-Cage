#!/usr/bin/env bash
# Validate the actual downloaded distribution without installing or launching it.
set -euo pipefail
directory="${1:?Usage: scripts/tests/verify-macos-release.sh <release-directory>}"
# stapler contacts Apple's CloudKit even for a stapled artifact. Hosted runners
# can lose this connection; retry service failures without bypassing validation.
validate_ticket() {
  local attempt status
  for attempt in 1 2 3; do
    if xcrun stapler validate "$1"; then return 0; else status=$?; fi
    if [[ "$status" != 68 || "$attempt" == 3 ]]; then return "$status"; fi
    echo "Apple ticket service unavailable; retry $attempt/2." >&2
    sleep 5
  done
}
shopt -s nullglob
archives=("$directory"/Cage-*-macos-arm64.dmg)
if [[ ${#archives[@]} != 1 ]]; then echo 'Expected exactly one Cage DMG.' >&2; exit 1; fi
archive="${archives[0]}"
version="${archive##*/Cage-}"
version="${version%-macos-arm64.dmg}"
(cd "$directory" && shasum -a 256 --check SHA256SUMS)
codesign --verify --strict --verbose=2 "$archive"
validate_ticket "$archive"
spctl --assess --type open --context context:primary-signature --verbose=2 "$archive"
mount="$(mktemp -d)"
cleanup() { hdiutil detach "$mount" >/dev/null 2>&1 || true; rmdir "$mount" 2>/dev/null || true; }
trap cleanup EXIT
hdiutil attach -readonly -nobrowse -mountpoint "$mount" "$archive" >/dev/null
bundle="$mount/Cage.app"
test "$(/usr/libexec/PlistBuddy -c 'Print :CageVersion' "$bundle/Contents/Info.plist")" = "$version"
codesign --verify --deep --strict --verbose=2 \
  -R='anchor apple generic and identifier "io.github.lee-xeo.claude-cage" and certificate 1[field.1.2.840.113635.100.6.2.6] exists and certificate leaf[field.1.2.840.113635.100.6.1.13] exists' "$bundle"
validate_ticket "$bundle"
spctl --assess --type execute --verbose=2 "$bundle"
echo "Verified Developer ID, notarization, Gatekeeper, and version: Cage $version"
