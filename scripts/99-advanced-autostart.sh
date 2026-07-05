#!/usr/bin/env bash
# ADVANCED / EXPERIMENTAL — unattended boot autostart (NOT run by default).
#
# The default installer uses xovi-tripletap (power-button triple-press), which
# asivery recommends and which cannot bootloop you. This script instead makes
# xovi load on EVERY boot with no button press, via a guarded systemd unit.
#
# ⚠️  Read before using:  asivery explicitly warns against naive systemd
# autostart on the encrypted Paper Pro — a bad LD_PRELOAD or a stale Qt hashtab
# can crash xochitl, and rm-emergency then REBOOTS THE WHOLE DEVICE, wiping the
# volatile /etc drop-in, which looks like a boot loop.
#
# This unit mitigates that with a crash-loop safety net: after 3 quick reboots
# it SKIPS xovi and boots plain stock so you can SSH in. There is also a kill
# switch. It is still riskier than tripletap — only use it if you want true
# hands-off autostart and understand the recovery path.
#
#   Enable:   scripts/99-advanced-autostart.sh
#   Disable:  ssh <device> 'touch /home/root/.xovi-boot/disable'   (then reboot)
#   Recover:  hold power to force reboot ×3 → safe mode → SSH in → fix.
HERE="$(cd "$(dirname "$0")" && pwd)"
source "$HERE/lib.sh"

step "Advanced: unattended boot autostart"
cat <<EOF
${C_WARN}
  This makes xovi load on every boot without a button press. It is riskier than
  the default tripletap. A crash-loop safety net drops you to stock after 3 fast
  reboots, but you should be comfortable SSHing in to recover.
${C_0}
EOF
confirm "Install unattended boot autostart anyway?" || die "Cancelled. tripletap remains your persistence."

rm_is_paper_pro || die "Refusing on an unrecognized device."

step "Installing guarded autostart script + boot unit"
rm_scp "$HERE/advanced-boot-autostart.sh" "$(rm_dest):/home/root/xovi/xovi-autostart.sh"
rm_ssh 'chmod +x /home/root/xovi/xovi-autostart.sh'

# The systemd unit must survive reboot → real rootfs /etc via the overlay trick.
persist_local_to_rootfs "$HERE/xovi-boot.service" /etc/systemd/system/xovi-boot.service
persist_local_to_rootfs "$HERE/xovi-boot.service" /etc/systemd/system/multi-user.target.wants/xovi-boot.service
rm_ssh 'systemctl daemon-reload 2>/dev/null || true'

ok "Unattended autostart installed (3-fast-boot safe-mode fallback active)."
info "Disable any time:  ssh $(rm_dest) 'touch /home/root/.xovi-boot/disable'"
