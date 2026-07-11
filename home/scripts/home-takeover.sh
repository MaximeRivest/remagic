#!/bin/bash
# Remagic Home session host: stop xochitl ONCE, then loop between the home
# launcher and takeover apps — no xochitl round-trip between apps. Only when
# the user taps LEAVE (or power / 5-finger tap in home) is xochitl restored.
#
# Contract with apps: home writes the chosen app's *-takeover.sh path to
# /tmp/remagic-home-choice and exits 42. We run that script with
# REMAGIC_SESSION=1 so session-aware scripts skip their own stop/restore of
# xochitl. Legacy scripts that ignore the variable will restart xochitl on
# exit; we detect that and end the session cleanly (compatibility fallback).
#
# Escape hatch if anything wedges: ssh rm 'systemctl start xochitl'.

CHOICE=/tmp/remagic-home-choice

restore() {
    rm -f /tmp/epframebuffer.lock "$CHOICE"
    systemctl start xochitl
}
trap restore EXIT INT TERM

HERE=$(cd "$(dirname "$0")" && pwd)

systemctl stop xochitl
rm -f /tmp/epframebuffer.lock
sleep 1

while :; do
    rm -f "$CHOICE" /tmp/epframebuffer.lock
    LD_LIBRARY_PATH="$HERE:/home/root/quill:/usr/lib/plugins/scenegraph" \
        HOME=/home/root "$HERE/home"
    rc=$?
    [ "$rc" = 42 ] && [ -f "$CHOICE" ] || break

    APP_SCRIPT=$(head -n1 "$CHOICE")
    [ -x "$APP_SCRIPT" ] || [ -f "$APP_SCRIPT" ] || break
    rm -f /tmp/epframebuffer.lock
    REMAGIC_SESSION=1 /bin/bash "$APP_SCRIPT"

    # Legacy app script restarted xochitl itself? Then the session is over.
    if systemctl is-active --quiet xochitl; then
        echo "home-takeover: app restored xochitl (legacy script), ending session"
        trap - EXIT INT TERM
        rm -f "$CHOICE"
        exit 0
    fi
done

echo "home-takeover: session over, restoring xochitl"
