# remagic installer — fresh-device dry-run audit

Traced every command against a real Paper Pro (same OS image a reset produces).
Downloads verified live; contents inspected; device tools probed.

## 🔴 Blocker — will break the newcomer experience

### 1. AppLoad install copies the WRONG files into extensions.d/
`03-install-xovi.sh` step 2:
`find appload-unz -name "*.so" -exec cp -f {} .../extensions.d/`
The AppLoad zip contains **3** `.so` files: `appload.so`, `shims/qtfb-shim.so`,
`shims/qtfb-shim-32bit.so`. Only `appload.so` is a xovi extension. The working
device has **only appload.so** in `extensions.d/` and no shim files anywhere.
Copying the shims into `extensions.d/` makes xovi try to load them as
extensions → likely load errors / broken AppLoad.
**Fix:** copy only `appload.so` to `extensions.d/`; place `shims/` under the
appload exthome dir (or drop them — the working install has none).

### 2. Password prompt on every SSH call (no connection reuse)
A fresh device has only password auth (the code shown on the tablet). There is
no `ControlMaster`/`ControlPath`, so **every** `rm_ssh`/`rm_scp` opens a new
connection = a new password prompt. Preflight alone makes ~3 calls *before* the
key is installed; the whole run is ~12+ prompts. Worse, `rm_reachable` does
`rm_ssh true 2>/dev/null` — the `2>/dev/null` hides the password prompt, so it
looks like a silent hang.
**Fix:** add SSH multiplexing to `SSH_OPTS` (ControlMaster=auto, a ControlPath
in a temp dir, ControlPersist), and install the key FIRST so one password entry
covers the whole run. Print "enter the device password once" up front.

## 🟡 Should fix — works but rough

### 3. `curl` assumed but absent on device (host-side only — OK)
`fetch()` prefers curl, falls back to wget — but it runs on the **laptop**, so
this is fine. Only flag: if a Linux user lacks curl AND real wget, downloads
fail. Minor. (On-device, only BusyBox wget exists — the installer correctly
uses it just for the tripletap pipe.)

### 4. BusyBox wget shows a scary TLS warning
`wget -qO- <tripletap> | bash` works, but prints
`wget: note: TLS certificate validation not implemented` — no cert validation.
Functionally fine (verified: fetches the script and the GitHub archive), but
worth a one-line note so the user isn't alarmed, and a security caveat.

### 5. Hashtable rebuild runs xochitl foreground — may hang non-interactively
`rebuild_hashtable </dev/null` drives xochitl to capture the hashtab. It's
wrapped in a warn-not-die, so it won't block the install, but on a fresh device
it may sit for a while or fail silently. Acceptable (AppLoad doesn't need it),
but the timing/behavior is unverified on a truly fresh image.

## 🟢 Verified GOOD

- Pinned URLs all return 200 (xovi bundle, AppLoad, tripletap installer).
- xovi bundle extracts with `xovi/` prefix → `tar -C /home/root` is correct;
  contains `rebuild_hashtable` and `inactive-extensions/xovi-message-broker.so`.
- Device has `wget, unzip, tar, bash(real), sh, systemd-run, systemctl, find,
  mktemp`. `busybox unzip` fallback present.
- SSH key path is correct: dropbear reads `/home/root/.ssh/authorized_keys`
  (confirmed — that's where the working key lives); the `-G root` flag does not
  override it.
- BusyBox wget DOES do HTTPS + redirects (tripletap pipe works end-to-end).
- Message-broker activation, `/home/root` writability, tripletap install path
  all fine.

## Net verdict
The install is **~2 fixes away from working for a stranger**: the AppLoad
shim-copy bug (functional breakage) and SSH multiplexing (usability wall of
password prompts). Everything else is solid or cosmetic. Neither blocker needs
a factory reset to fix — both are fixable and re-testable against the current
device.
