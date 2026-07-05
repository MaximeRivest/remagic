#!/bin/bash
# Boot-time xovi autostart with a crash-loop safety net.
#
# Runs /home/root/xovi/start (the real injector) at boot. To avoid ever wedging
# you into an unbootable state: if recent boots have been rapidly failing, we
# SKIP starting xovi this boot and leave a marker, so the device comes up as
# plain stock xochitl and you can SSH in to fix things. The counter lives on
# persistent /home; a stable boot resets it.
#
# Kill switch: `touch /home/root/.xovi-boot/disable` to stop autostart entirely.
set -u

XOVI=/home/root/xovi
STATE=/home/root/.xovi-boot
FAILC="$STATE/consecutive-fast-boots"
LASTBOOT="$STATE/last-boot-epoch"
MAX_FAST_BOOTS=3      # after this many quick reboots in a row, stop autostarting
FAST_BOOT_SECS=120    # a boot within this many seconds of the last = crash-loop signal

mkdir -p "$STATE"

# Explicit kill switch.
[ -e "$STATE/disable" ] && { echo "xovi-autostart: disabled by kill switch" >&2; exit 0; }

now=$(date +%s 2>/dev/null || echo 0)
last=$(cat "$LASTBOOT" 2>/dev/null || echo 0)
echo "$now" > "$LASTBOOT"

count=$(cat "$FAILC" 2>/dev/null || echo 0)
if [ "$now" -gt 0 ] && [ "$last" -gt 0 ] && [ $((now - last)) -lt "$FAST_BOOT_SECS" ]; then
    count=$((count + 1))
else
    count=0
fi
echo "$count" > "$FAILC"

if [ "$count" -ge "$MAX_FAST_BOOTS" ]; then
    echo "xovi-autostart: $count fast boots in a row — SKIPPING xovi (safe mode)." >&2
    echo "xovi-autostart: rm $FAILC and reboot, or run 'bash $XOVI/start' by hand." >&2
    exit 0
fi

[ -x "$XOVI/start" ] || { echo "xovi-autostart: $XOVI/start missing" >&2; exit 0; }

echo "xovi-autostart: starting xovi (fast-boot count=$count)" >&2
bash "$XOVI/start"

# Reset the crash-loop counter shortly after a start that didn't reboot us.
( sleep 90; echo 0 > "$FAILC" ) &
exit 0
