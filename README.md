# ✨ remagic

**Make your reMarkable Paper Pro magical.**

**→ [Visit the remagic website](https://maximerivest.github.io/remagic/)**

Turn on developer mode, run one command, and you have the AppLoad launcher and a
place to run your own apps — no terminal wrangling. Built to lower the barrier
for builders and tinkerers. Open source, MIT licensed.

> Works on the **reMarkable Paper Pro** (Ferrari / Chiappa / Tatsu — i.MX8MM,
> aarch64). Not for the reMarkable 1 or 2.

remagic is also the umbrella for a small family of apps that show what the tablet
can do once it's opened up — see [The remagic family](#the-remagic-family) below.

---

## Two steps

### 1. Turn on developer mode

This is the only manual part, and it **erases the tablet** (a reMarkable
security requirement — no tool can skip it), so **sync your notebooks to the
cloud first**. Full walkthrough:

**→ [docs/DEVELOPER-MODE.md](docs/DEVELOPER-MODE.md)**

### 2. Run the installer

Plug the tablet in over USB and run one line — no git, no compiler, nothing
else to install:

**Linux / macOS**

```sh
curl -fsSL https://raw.githubusercontent.com/maximerivest/remagic/main/get.sh | sh
```

**Windows** (PowerShell)

```powershell
irm https://raw.githubusercontent.com/maximerivest/remagic/main/get.ps1 | iex
```

This downloads the `remagic` CLI for your machine and runs `remagic setup`.
The tablet is found automatically — USB or Wi-Fi, no address needed (use
`remagic -host <ip> setup` to pick one explicitly).

<details>
<summary>Prefer the shell-script installer? (Linux/macOS, needs git + ssh)</summary>

```sh
git clone https://github.com/maximerivest/remagic
cd remagic
./install.sh                      # USB
RM_HOST=<tablet-ip> ./install.sh  # Wi-Fi
```

</details>

Setup:

1. **Checks the connection** and confirms it's a Paper Pro in developer mode.
2. **Installs your SSH key** — no more typing the device password.
3. **Installs xovi + AppLoad** from official upstream releases.
4. **Sets up persistence** via [xovi-tripletap](https://github.com/rmitchellscott/xovi-tripletap):
   **triple-press the power button** to toggle xovi on or off. Survives reboots,
   needs no computer, and can't bootloop you.
5. **Installs the Store app**, so from then on you can browse and install apps
   right on the tablet — no computer needed.

When it finishes, the **AppLoad** launcher appears on your tablet. You type
the device password at most once: setup installs an SSH key (and generates
one for you if you've never made one).

After a reMarkable OS update, just run `remagic setup` again.

---

## The remagic CLI

Beyond the installer there's a companion CLI — one static Go binary that makes
the tablet feel like a normal, friendly device, over USB **or Wi-Fi**:

```
remagic setup                     # the whole install, one command
./remagic find                    # discovers tablets (USB + LAN scan)
./remagic doctor                  # health check: stack, SSH, battery, apps
./remagic key                     # passwordless SSH from then on
./remagic wifi on                 # after this, the cable is optional
./remagic install <app|./dir>     # catalog app or local folder → AppLoad
./remagic config <app>            # settings form in your browser + QR for phone
./remagic repair-ssh              # fixes the "connection reset" SSH wedge
```

Details: **[cli/README.md](cli/README.md)**.

---

## Adding apps

Three ways, from easiest to most manual:

1. **The Store, on the tablet.** Install it once (`remagic install store`),
   then browse and install apps right on the device — no computer needed.
2. **From your computer:** `remagic install <app>` pulls a checksum-verified
   app from the [catalog](catalog.json), or `remagic install ./myapp` pushes a
   local folder straight into AppLoad.
3. **By hand:** AppLoad apps live in `/home/root/xovi/exthome/appload/<app>/`.
   An "external" app (wrapping any aarch64 binary) needs just two files:

   ```
   myapp/
   ├── external.manifest.json
   └── icon.png
   ```

   ```json
   {
     "name": "My App",
     "application": "myapp",
     "qtfb": true
   }
   ```

   Copy the folder over and tap **Reload** in AppLoad.

Building (or publishing) your own? The full app format — manifest fields,
settings schemas, UI conventions, and `remagic publish` — is specified in
**[docs/APP-SPEC.md](docs/APP-SPEC.md)**.

---

## Turning it off

- **Temporarily:** triple-press the power button, or just reboot — you're back
  to stock reMarkable software instantly. Your apps stay installed.
- **Fully:** `ssh <device> '/home/root/xovi/stock'`.
- **Back to a clean tablet:** disable developer mode via reMarkable's recovery
  application (this factory-resets again).

Nothing here touches the bootloader or your encrypted data. It is designed to be
safe and reversible.

---

## Advanced: autostart on every boot

By default xovi loads on a power-button triple-press (the safe, recommended
way). If you want it to load automatically on **every** boot with no press, see
[`scripts/99-advanced-autostart.sh`](scripts/99-advanced-autostart.sh) — it
installs a guarded systemd unit with a crash-loop safety net. It's riskier
(upstream advises against naive autostart on the encrypted device), so it's
opt-in and separate.

---

## How it works / layout

| Path | What it is |
|------|-----------|
| `install.sh` | Top-level installer; runs the steps below in order. |
| `scripts/lib.sh` | Shared helpers: SSH wrappers, device detection, and `persist_local_to_rootfs` (the `/etc`-overlay persistence trick). |
| `scripts/01-preflight.sh` | Connection + developer-mode + model checks with friendly errors. |
| `scripts/02-ssh-key.sh` | Passwordless SSH setup (idempotent). |
| `scripts/03-install-xovi.sh` | Downloads and installs xovi + AppLoad + tripletap. |
| `scripts/sources.env` | Pinned upstream release URLs (one place to update versions). |
| `scripts/99-advanced-autostart.sh` | Optional unattended boot autostart. |
| `get.sh` / `get.ps1` | One-line bootstrap: download the prebuilt CLI, run `remagic setup`. |
| `cli/` | The `remagic` companion CLI (Go), including `remagic setup` (the pure-Go installer) and the on-device Store app (`cli/store/`). |
| `catalog.json` | The app catalog: pinned versions, URLs, and sha256 checksums. Feeds `remagic install` and the Store. |
| `docs/DEVELOPER-MODE.md` | The developer-mode walkthrough. |
| `docs/APP-SPEC.md` | The remagic app format: manifest, settings schemas, publishing. |

Re-run `remagic setup` (or `install.sh`) after a reMarkable OS update to
refresh the pieces an update can disturb.

---

## Troubleshooting

**`ssh`/`scp` says `kex_exchange_identification: read: Connection reset by peer`.**
The SSH host-key mount on the tablet got knocked over (upstream tripletap's
installer does this on the Paper Pro; the installer now repairs it
automatically). One reboot of the tablet fixes it. Not to be confused with the
normal ~20&nbsp;s USB-network dropout whenever the reMarkable UI restarts —
that one heals itself.

**The tablet stops answering after a minute idle.** It's asleep — e-ink devices
sleep aggressively. Tap the power button and reconnect.

**Wi-Fi ssh refused.** The *SSH over Wi-Fi* toggle (Settings → General →
Software → Advanced) resets with developer-mode/factory resets; USB
(`10.11.99.1`) always works when the cable is in. Plug in once and run
`remagic wifi on` to flip it back.

---

## The remagic family

Once your tablet is open, here's what we build on top of it — install any of
them from the Store or with `remagic install <app>`.

- **[Chromium](https://github.com/maximerivest/chromium)** — a real browser on
  e-ink: on-device Chromium driven over CDP. Tap to click, swipe to scroll,
  type on the built-in keyboard, browse anywhere with the URL bar —
  ChatGPT-ready. (The app installs from the catalog; the ~830 MB engine
  installs once with the repo's bootstrap scripts.)
- **[Riddle](https://github.com/maximerivest/riddle)** — an enchanted diary. Write
  with the pen; after a pause the page drinks your ink and an answer writes
  itself back in a flowing hand. A magical blackboard, powered by an LLM.
- **Store** (in this repo, `cli/store/`) — browse and install apps right on
  the tablet, no computer needed.
- **[PaperTerm](https://github.com/maximerivest/paperterm)** — a real terminal
  emulator with pixel-wise partial e-ink updates and an on-screen keyboard. A
  shell on your tablet.
- **Quill** *(repo coming soon)* — the low-level takeover display host the
  apps stand on: it drives the e-ink panel directly through the vendor
  waveform engine for instant-ink latency. More a library than an app.

Building your own? These are worked examples of an AppLoad app, from a full
takeover renderer (Quill/Riddle) to a windowed qtfb app (the Store, Paperterm).
Start with **[docs/APP-SPEC.md](docs/APP-SPEC.md)**.

---

## Credits

This kit stands on the work of the reMarkable modding community:

- **[xovi](https://github.com/asivery/xovi)** and
  **[rm-appload](https://github.com/asivery/rm-appload)** by **asivery** — the
  function-hooking loader and app host this kit installs.
- **[xovi-tripletap](https://github.com/rmitchellscott/xovi-tripletap)** by
  **rmitchellscott** — the power-button persistence.
- **[vellum](https://github.com/vellum-dev/vellum-cli)** — a fuller package
  manager for the ecosystem, if you outgrow this kit.

remagic just wires these together into a one-command, beginner-friendly install.
Please support the upstream projects.

## License

MIT — see [LICENSE](LICENSE). This installs third-party software under their own
licenses; it does not redistribute reMarkable's proprietary components.

## Disclaimer

Not affiliated with reMarkable. Developer mode and third-party software are used
at your own risk. This kit avoids the bootloader and your encrypted data and is
designed to be reversible, but you are responsible for your device.
