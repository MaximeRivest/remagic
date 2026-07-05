#!/usr/bin/env bash
# Shared helpers for the rmpp-kit installer. Sourced, not run directly.
#
# Design goals: no dependencies beyond ssh/scp/curl, friendly output, and every
# device-side write that must survive a reboot goes through persist_write() —
# which handles the /etc overlay trap that bites everyone (see below).

set -euo pipefail

# ---- pretty output -------------------------------------------------------
if [ -t 1 ] && [ -z "${NO_COLOR:-}" ]; then
    C_OK=$'\033[32m'; C_WARN=$'\033[33m'; C_ERR=$'\033[31m'; C_DIM=$'\033[2m'; C_B=$'\033[1m'; C_0=$'\033[0m'
else
    C_OK=; C_WARN=; C_ERR=; C_DIM=; C_B=; C_0=
fi
step()  { printf '%s==>%s %s\n' "$C_B"  "$C_0" "$*"; }
ok()    { printf '%s  ✓%s %s\n' "$C_OK" "$C_0" "$*"; }
warn()  { printf '%s  !%s %s\n' "$C_WARN" "$C_0" "$*" >&2; }
die()   { printf '%s  ✗%s %s\n' "$C_ERR" "$C_0" "$*" >&2; exit 1; }
info()  { printf '%s    %s%s\n' "$C_DIM" "$*" "$C_0"; }

# ---- connection ----------------------------------------------------------
# The device host. Override with RM_HOST=1.2.3.4 for Wi-Fi. Default = USB.
RM_HOST="${RM_HOST:-10.11.99.1}"
RM_USER="${RM_USER:-root}"
# Options that make first-contact painless: accept the new host key, don't
# pollute the user's known_hosts (dev-mode resets rotate the key anyway).
SSH_OPTS=(-o ConnectTimeout=8 -o StrictHostKeyChecking=accept-new
          -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR)

rm_ssh()  { ssh "${SSH_OPTS[@]}" "$RM_USER@$RM_HOST" "$@"; }
rm_scp()  { scp "${SSH_OPTS[@]}" -O "$@"; }
rm_dest() { printf '%s@%s' "$RM_USER" "$RM_HOST"; }

# Reachable at all?
rm_reachable() { rm_ssh true 2>/dev/null; }

# ---- device identity -----------------------------------------------------
# Guard everything on a known reMarkable model so we never run the persistence
# path on an unexpected device.
rm_model() { rm_ssh 'cat /proc/device-tree/model 2>/dev/null | tr -d "\0"'; }
rm_is_paper_pro() { rm_model | grep -qE 'reMarkable (Ferrari|Chiappa|Tatsu)'; }
rm_os_version() { rm_ssh '. /etc/os-release 2>/dev/null; printf %s "${IMG_VERSION:-}"'; }

# ---- persistence (THE load-bearing trick) --------------------------------
# `/` is plain ext4 and writable after remount, BUT `/etc` is an overlay whose
# upper layer is tmpfs — a naive write to /etc lands in RAM and vanishes on
# reboot. To persist a file under /etc you must reach *under* the overlay.
#
# We use the gentlest method that works and doesn't disturb the SSH session:
# bind-mount / somewhere, which exposes the real ext4 /etc beneath the overlay,
# write there, sync, unmount, and set / back to read-only.
#
# persist_local_to_rootfs <local-file> <remote-path>
# Copies one local file to a path under the *real* rootfs /etc (survives reboot).
# Opens and closes the overlay-collapse for this single file; for a couple of
# files the overhead is trivial and the code stays simple.
persist_local_to_rootfs() {
    local src="$1" dest="$2"
    rm_scp "$src" "$(rm_dest):/tmp/.persist_payload"
    rm_ssh "REMOTE_DEST='$dest' bash -s" <<'REMOTE'
set -e
grep -qE "reMarkable (Ferrari|Chiappa|Tatsu)" /proc/device-tree/model || {
    echo "refusing to persist on unknown device" >&2; exit 1; }
mount -o remount,rw /
BIND=$(mktemp -d)
mount --bind / "$BIND"           # a bind of / sees the real rootfs, under the /etc overlay
UNDER="$BIND/${REMOTE_DEST#/}"
mkdir -p "$(dirname "$UNDER")"
cp /tmp/.persist_payload "$UNDER"
sync
umount "$BIND"; rmdir "$BIND"
mount -o remount,ro / 2>/dev/null || true
rm -f /tmp/.persist_payload
echo "persisted: $REMOTE_DEST"
REMOTE
}

# Confirmation prompt (skipped when --yes / RM_ASSUME_YES=1).
confirm() {
    [ "${RM_ASSUME_YES:-0}" = 1 ] && return 0
    local reply
    printf '%s  %s [y/N] %s' "$C_WARN" "$1" "$C_0"
    read -r reply
    [ "$reply" = y ] || [ "$reply" = Y ]
}
