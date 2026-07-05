#!/usr/bin/env bash
# Set up key-based SSH so you never type the device password again.
# Idempotent: safe to run repeatedly.
HERE="$(cd "$(dirname "$0")" && pwd)"
source "$HERE/lib.sh"

step "Setting up passwordless SSH"

KEY="${RM_SSH_KEY:-$HOME/.ssh/id_ed25519}"
if [ ! -f "$KEY" ]; then
    info "No SSH key at $KEY — generating one."
    ssh-keygen -t ed25519 -N '' -f "$KEY" -C "rmpp-kit@$(hostname)" >/dev/null
    ok "Generated $KEY"
fi
PUB="$(cat "$KEY.pub")"

# Already installed?
if rm_ssh "grep -qF '$PUB' /home/root/.ssh/authorized_keys 2>/dev/null"; then
    ok "Key already authorized on the tablet."
    exit 0
fi

# authorized_keys lives under /home (encrypted, persistent) — a normal write,
# no overlay trick needed here.
printf '%s\n' "$PUB" | rm_ssh 'mkdir -p /home/root/.ssh && chmod 700 /home/root/.ssh && cat >> /home/root/.ssh/authorized_keys && chmod 600 /home/root/.ssh/authorized_keys'
ok "Installed your public key."

# Verify key auth works without a password prompt.
if ssh "${SSH_OPTS[@]}" -o BatchMode=yes -o PasswordAuthentication=no "$(rm_dest)" true 2>/dev/null; then
    ok "Passwordless login confirmed."
else
    warn "Key installed but password-free login didn't verify — continuing anyway."
fi

cat <<EOF

  ${C_DIM}Optional: add a shortcut to ~/.ssh/config so you can just 'ssh rmpp':

    Host rmpp
        HostName $RM_HOST
        User root
        IdentityFile $KEY${C_0}

EOF
