#!/usr/bin/env bash
# Cross-compile the store and stage its AppLoad bundle into dist/store/.
set -euo pipefail
cd "$(dirname "$0")/.."
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -o store/store-arm64 ./store
mkdir -p store/dist/store
install -m 755 store/store-arm64 store/dist/store/store
install -m 644 store/icon.png store/dist/store/
[ -f store/dist/store/external.manifest.json ] || { echo "manifest missing"; exit 1; }
echo "staged: store/dist/store — publish with: ./remagic publish store/dist/store -catalog-dir .."
