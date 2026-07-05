#!/usr/bin/env bash
# Download the xovi bundle + AppLoad, install onto the tablet, activate the
# message broker, and set up persistence via the community-blessed tripletap
# (power-button triple-press). All upstream URLs live in sources.env.
HERE="$(cd "$(dirname "$0")" && pwd)"
source "$HERE/lib.sh"
# shellcheck disable=SC1091
source "$HERE/sources.env"

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

fetch() {  # fetch <url> <dest>
    [ -n "$1" ] || die "empty download URL (a release asset moved? see sources.env)"
    step "Downloading $(basename "$2")"
    if command -v curl >/dev/null; then
        curl -fL --retry 3 -o "$2" "$1" || die "download failed: $1"
    else
        wget -O "$2" "$1" || die "download failed: $1"
    fi
    ok "$(basename "$2")"
}

# --- 1. xovi bundle (loader + scripts + qt-resource-rebuilder) ---------------
fetch "$XOVI_BUNDLE_URL" "$WORK/xovi.tar.gz"
step "Installing xovi onto the tablet"
rm_scp "$WORK/xovi.tar.gz" "$(rm_dest):/tmp/xovi.tar.gz"
# Extracts to /home/root/xovi/ (persistent, encrypted home — no overlay trick).
rm_ssh 'tar -xzf /tmp/xovi.tar.gz -C /home/root && rm -f /tmp/xovi.tar.gz' \
    || die "failed to extract xovi bundle"
ok "xovi installed at /home/root/xovi"

# Activate the message broker (ships inactive in the bundle). Harmless if the
# app you run doesn't need it; several community apps do.
rm_ssh '[ -f /home/root/xovi/inactive-extensions/xovi-message-broker.so ] && \
        mv -f /home/root/xovi/inactive-extensions/xovi-message-broker.so \
              /home/root/xovi/extensions.d/ 2>/dev/null; true'
ok "Message broker activated."

# --- 2. AppLoad --------------------------------------------------------------
fetch "$APPLOAD_URL" "$WORK/appload.zip"
step "Installing the AppLoad launcher"
rm_scp "$WORK/appload.zip" "$(rm_dest):/tmp/appload.zip"
# AppLoad is a xovi extension: its .so goes in extensions.d/. The zip may also
# carry an exthome skeleton; unzip in place and let it populate.
rm_ssh 'cd /tmp && rm -rf appload-unz && mkdir appload-unz && \
        (unzip -oq appload.zip -d appload-unz || busybox unzip -o appload.zip -d appload-unz) && \
        find appload-unz -name "*.so" -exec cp -f {} /home/root/xovi/extensions.d/ \; && \
        if [ -d appload-unz/exthome ]; then cp -rf appload-unz/exthome/. /home/root/xovi/exthome/; fi && \
        rm -rf appload-unz appload.zip' \
    || die "failed to install AppLoad"
ok "AppLoad installed."

# --- 3. rebuild the Qt hashtable (needed by qt-resource-rebuilder) -----------
# qt-resource-rebuilder needs a per-OS-version hashtable, rebuilt on first
# install and after every OS update. The bundle ships upstream's
# `rebuild_hashtable`, which drives xochitl to capture it. AppLoad itself does
# not require it — only UI/QML mods do — so this is best-effort and never fatal.
step "Rebuilding the Qt resource hashtable"
if rm_ssh 'test -x /home/root/xovi/rebuild_hashtable'; then
    if rm_ssh '/home/root/xovi/rebuild_hashtable </dev/null' 2>/dev/null; then
        ok "Hashtable rebuilt."
    else
        warn "Couldn't rebuild the hashtable non-interactively."
        info "AppLoad still works. For UI/QML mods, run this once by hand:"
        info "  ssh $(rm_dest) '/home/root/xovi/rebuild_hashtable'  then reboot."
    fi
else
    info "No rebuild_hashtable in this bundle — skipping (AppLoad doesn't need it)."
fi

# --- 4. persistence: tripletap (blessed) -------------------------------------
step "Installing power-button persistence (xovi-tripletap)"
info "Triple-press the power button to toggle xovi on/off — survives reboots,"
info "no computer needed. This is the community-recommended, encryption-safe way."
if rm_ssh "wget -qO- '$TRIPLETAP_INSTALL_URL' | bash"; then
    ok "tripletap installed — triple-press power to launch xovi."
else
    warn "tripletap install didn't complete. You can still start xovi manually:"
    info "ssh $(rm_dest) '/home/root/xovi/start'"
fi

# --- 5. start it now ---------------------------------------------------------
step "Starting xovi now (no reboot needed)"
rm_ssh 'systemd-run --unit=xovi-firststart --collect --service-type=oneshot /home/root/xovi/start 2>/dev/null' \
    && ok "xovi started." \
    || warn "Couldn't start immediately; triple-press power, or reboot."

cat <<EOF

  ${C_OK}${C_B}AppLoad is installed.${C_0}
  The AppLoad launcher should appear on your tablet. Add apps by dropping them
  into ${C_B}/home/root/xovi/exthome/appload/<app>/${C_0} — see the README.

  ${C_DIM}Turn xovi on/off any time with a power-button triple-press.
  Full off:  ssh $(rm_dest) '/home/root/xovi/stock'  (or just reboot).${C_0}

EOF
