#!/usr/bin/env bash
# remagic — one command to turn a developer-mode reMarkable Paper Pro into a
# tinkerer's playground: passwordless SSH, the AppLoad launcher, and crash-safe
# autostart. Open source. No terminal wrangling for the user beyond running this.
#
#   Usage:   ./install.sh              # USB (10.11.99.1)
#            RM_HOST=192.168.1.42 ./install.sh   # over Wi-Fi
#            ./install.sh --yes        # don't prompt for confirmations
#
# Prerequisite: developer mode ON (see docs/DEVELOPER-MODE.md). That's the only
# manual step; everything below is automated.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)/scripts"
source "$HERE/lib.sh"

for arg in "$@"; do
    case "$arg" in
        --yes|-y) export RM_ASSUME_YES=1 ;;
        --host=*) export RM_HOST="${arg#*=}" ;;
        -h|--help) sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
        *) die "unknown option: $arg (try --help)" ;;
    esac
done

cat <<EOF

  ${C_B}remagic installer${C_0}
  ${C_DIM}Empowering builders on the reMarkable Paper Pro.${C_0}

  This will, over SSH to ${C_B}$RM_HOST${C_0}:
    1. Verify the connection and that developer mode is on
    2. Install your SSH key (no more passwords)
    3. Download and install xovi + the AppLoad launcher (official upstream)
    4. Set up power-button persistence (triple-press to toggle xovi)

  Nothing here touches your notebooks or the bootloader. It is reversible:
  a power-button triple-press, or a reboot, returns you to stock reMarkable.

EOF

confirm "Proceed?" || die "Cancelled. Nothing changed."

bash "$HERE/01-preflight.sh"
bash "$HERE/02-ssh-key.sh"
bash "$HERE/03-install-xovi.sh"

printf '\n%s%s  Done. Happy tinkering.%s\n' "$C_OK" "$C_B" "$C_0"
printf '%s  Want xovi on every boot with no button press? See\n  scripts/99-advanced-autostart.sh (advanced, riskier).%s\n\n' "$C_DIM" "$C_0"
