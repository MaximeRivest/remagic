package main

import (
	"strings"

	"github.com/maximerivest/remagic/cli/internal/device"
)

// rebuildHashtabScript drives xochitl with only qt-resource-rebuilder loaded
// and waits until the per-OS-version hashtab file exists. Upstream's
// rebuild_hashtable blocks on "press enter" and remagic's old setup used
// `timeout … rebuild_hashtable </dev/null`, which fails on device images
// without coreutils timeout — leaving AppLoad to crash-loop xochitl on
// OS 3.27+ when the hashtab is missing.
const rebuildHashtabScript = `
set -u
xovi=/home/root/xovi
qrr_so="$xovi/extensions.d/qt-resource-rebuilder.so"
qrr_dir="$xovi/exthome/qt-resource-rebuilder"
hashtab="$qrr_dir/hashtab"
xovi_root=/tmp/xovi-hashtab-build
log=/tmp/xovi-hashtab.log

if [ ! -f "$qrr_so" ]; then
  echo "qt-resource-rebuilder not installed" >&2
  exit 1
fi

systemctl stop xochitl 2>/dev/null || true
for sig in 15 9; do
  pid=$(pidof xochitl 2>/dev/null || true)
  [ -n "$pid" ] || break
  kill -$sig $pid 2>/dev/null || true
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
while [ "$(date +%s)" -lt "$deadline" ]; do
  if [ -s "$hashtab" ]; then
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
[ -n "$pid" ] && kill -15 $pid 2>/dev/null || true
sleep 1
pid=$(pidof xochitl 2>/dev/null || true)
[ -n "$pid" ] && kill -9 $pid 2>/dev/null || true

rm -rf "$xovi_root"

if [ "$ok" = 1 ] && [ -s "$hashtab" ]; then
  echo "Hashtab rebuilt at $hashtab"
  exit 0
fi

echo "Hashtab rebuild failed." >&2
echo "--- last log lines ---" >&2
tail -30 "$log" 2>/dev/null >&2 || true
exit 1
`

const hashtabPath = "/home/root/xovi/exthome/qt-resource-rebuilder/hashtab"

// rebuildHashtab runs the non-interactive rebuild on the tablet. Returns true
// when qt-resource-rebuilder is absent (nothing to do) or the hashtab exists.
func rebuildHashtab(d *device.Device) bool {
	if _, err := d.Run("test -f /home/root/xovi/extensions.d/qt-resource-rebuilder.so"); err != nil {
		ok("not needed (no qt-resource-rebuilder in this bundle)")
		return true
	}
	out, err := d.RunIn("bash -s", strings.NewReader(rebuildHashtabScript))
	out = strings.TrimSpace(out)
	if err != nil {
		if out != "" {
			for _, line := range strings.Split(out, "\n") {
				warn("%s", strings.TrimSpace(line))
			}
		}
		warn("AppLoad needs this hashtab on OS 3.27+ — without it xochitl crash-loops")
		warn("retry with: remagic rebuild-hashtab")
		return false
	}
	if out != "" {
		ok("%s", out)
	} else {
		ok("hashtable rebuilt")
	}
	return true
}

// hasHashtab reports whether the on-device hashtab file exists and is non-empty.
func hasHashtab(d *device.Device) bool {
	_, err := d.Run("test -s " + hashtabPath)
	return err == nil
}
