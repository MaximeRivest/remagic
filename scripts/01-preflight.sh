#!/usr/bin/env bash
# Preflight: is the tablet in developer mode, reachable, and the model we expect?
# Runs first; friendly, specific errors instead of a wall of ssh noise later.
HERE="$(cd "$(dirname "$0")" && pwd)"
source "$HERE/lib.sh"

step "Checking the connection to your reMarkable ($RM_HOST)"

if ! rm_reachable; then
    warn "Couldn't reach the tablet over SSH at $RM_HOST."
    cat <<EOF

  Checklist:
    • Is developer mode ON?  (See docs/DEVELOPER-MODE.md — without it there is
      no SSH and this kit cannot run.)
    • Is the USB cable plugged in?  The tablet is $RM_HOST over USB.
    • On Wi-Fi instead?  Re-run with:  RM_HOST=<tablet-ip> $0
    • First time connecting?  You may be prompted for the SSH password shown on
      the tablet under Settings → Help → Copyrights and licenses (GPLv3 / SSH).

EOF
    die "Not reachable. Fix the above and try again."
fi
ok "Connected."

MODEL="$(rm_model)"
if ! rm_is_paper_pro; then
    warn "Device reports: ${MODEL:-unknown}"
    die "This kit targets the reMarkable Paper Pro. Refusing to touch an unrecognized device."
fi
ok "Device: $MODEL"

OSV="$(rm_os_version || true)"
info "OS version: ${OSV:-unknown}"
ok "Preflight passed."
