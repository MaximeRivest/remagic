#!/bin/sh
# remagic bootstrap: download the prebuilt CLI for this machine and run setup.
#
#   curl -fsSL https://raw.githubusercontent.com/maximerivest/remagic/main/get.sh | sh
#
# No git, no Go, no bash-isms — plain POSIX sh + curl/wget. The binary lands in
# ~/.local/bin/remagic (or ./remagic if that's not writable) and `remagic setup`
# runs immediately. Re-running is safe; it just refreshes the binary.
set -eu

REPO="maximerivest/remagic"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
    linux|darwin) ;;
    *) echo "remagic: unsupported OS '$os' — on Windows use get.ps1 (see README)" >&2; exit 1 ;;
esac
arch=$(uname -m)
case "$arch" in
    x86_64|amd64) arch=amd64 ;;
    aarch64|arm64) arch=arm64 ;;
    *) echo "remagic: unsupported architecture '$arch'" >&2; exit 1 ;;
esac

asset="remagic-${os}-${arch}"
url="https://github.com/${REPO}/releases/latest/download/${asset}"

# Pick an install dir: ~/.local/bin if it's on PATH or creatable, else CWD.
bindir="${HOME}/.local/bin"
mkdir -p "$bindir" 2>/dev/null || bindir="."
dest="${bindir}/remagic"

echo "==> downloading ${asset} (latest release)"
if command -v curl >/dev/null 2>&1; then
    curl -fSL -o "$dest" "$url"
elif command -v wget >/dev/null 2>&1; then
    wget -qO "$dest" "$url"
else
    echo "remagic: need curl or wget" >&2; exit 1
fi
chmod +x "$dest"
echo "  ✓ installed: $dest"

case ":$PATH:" in
    *":$bindir:"*) ;;
    *) echo "  ! $bindir is not on your PATH — run it as: $dest" ;;
esac

# Plugged in / on the same network? Go straight into setup. Skippable for
# people who only want the binary: REMAGIC_NO_SETUP=1
if [ "${REMAGIC_NO_SETUP:-0}" = 1 ]; then
    echo "==> done. Run '$dest setup' when your tablet is connected."
    exit 0
fi
echo "==> starting setup (Ctrl-C to stop; run '$dest setup' any time)"
exec "$dest" setup
