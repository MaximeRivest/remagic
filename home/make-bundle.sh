#!/usr/bin/env bash
# Stage the AppLoad bundle into dist/home/, ready for `remagic publish`.
# Prereq: ./build.sh has produced Home and the sibling Quill checkout is built.
set -euo pipefail
cd "$(dirname "$0")"

QUILL=${QUILL_DIR:-../../quill}
BIN=build/home
[ -f "$BIN" ] || { echo "build first: ./build.sh" >&2; exit 1; }
[ -f "$QUILL/build/libquill.so" ] || { echo "missing $QUILL/build/libquill.so" >&2; exit 1; }

rm -rf dist/home
mkdir -p dist/home
install -m 755 "$BIN" dist/home/home
install -m 755 "$QUILL/build/libquill.so" dist/home/
install -m 755 scripts/appload-launch.sh scripts/home-takeover.sh dist/home/
install -m 644 external.manifest.json icon.png dist/home/

echo "staged: $(du -sh dist/home | cut -f1) in dist/home/"
echo "publish with: remagic publish dist/home -catalog-dir <remagic checkout>"
