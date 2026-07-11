#!/usr/bin/env bash
# Non-interactive Qt resource hashtab rebuild — same logic as remagic's Go CLI.
# Upstream xovi/rebuild_hashtable blocks on "press enter"; piping /dev/null
# skips that but still runs xochitl in a fragile foreground pipe. This script
# backgrounds xochitl, polls a log file, and verifies the hashtab on disk.
set -u

xovi=/home/root/xovi
qrr_so="$xovi/extensions.d/qt-resource-rebuilder.so"
qrr_dir="$xovi/exthome/qt-resource-rebuilder"
hashtab="$qrr_dir/hashtab"
xovi_root=/tmp/xovi-hashtab-build
log=/tmp/xovi-hashtab.log

if [[ ! -f "$qrr_so" ]]; then
    echo "qt-resource-rebuilder not installed" >&2
    exit 1
fi

systemctl stop xochitl 2>/dev/null || true
for sig in 15 9; do
    pid=$(pidof xochitl 2>/dev/null || true)
    [[ -n "$pid" ]] || break
    kill -"$sig" "$pid" 2>/dev/null || true
    sleep 1
done

rm -rf "$xovi_root" "$log"
mkdir -p "$xovi_root/extensions.d" "$qrr_dir"
ln -sf "$qrr_so" "$xovi_root/extensions.d/"
rm -f "$hashtab"

export XOVI_ROOT="$xovi_root"
QMLDIFF_HASHTAB_CREATE="$hashtab" \
QML_DISABLE_DISK_CACHE=1 \
LD_PRELOAD="$xovi/xovi.so" \
/usr/bin/xochitl >>"$log" 2>&1 &
xpid=$!

deadline=$(($(date +%s) + 180))
ok=0
while [[ $(date +%s) -lt $deadline ]]; do
    if [[ -s "$hashtab" ]]; then
        ok=1
        break
    fi
    if grep -qF "[qmldiff]: Hashtab saved to $hashtab" "$log" 2>/dev/null; then
        ok=1
        break
    fi
    if ! kill -0 "$xpid" 2>/dev/null; then
        break
    fi
    sleep 1
done

pid=$(pidof xochitl 2>/dev/null || true)
[[ -n "$pid" ]] && kill -15 "$pid" 2>/dev/null || true
sleep 1
pid=$(pidof xochitl 2>/dev/null || true)
[[ -n "$pid" ]] && kill -9 "$pid" 2>/dev/null || true

rm -rf "$xovi_root"

if [[ "$ok" = 1 && -s "$hashtab" ]]; then
    echo "Hashtab rebuilt at $hashtab"
    exit 0
fi

echo "Hashtab rebuild failed." >&2
echo "--- last log lines ---" >&2
tail -30 "$log" 2>/dev/null >&2 || true
exit 1
