# remagic CLI

One static Go binary that makes a developer-mode reMarkable feel like a
normal, friendly device — over USB **or Wi-Fi**.

```
go build -o remagic .          # or grab a release binary
./remagic find                 # discovers tablets (USB + LAN scan)
./remagic doctor               # health check: stack, SSH, battery, apps
./remagic key                  # passwordless SSH from then on
./remagic wifi on              # after this, the cable is optional
./remagic install <app|dir>    # catalog app or local folder → AppLoad
./remagic config riddle        # settings form in your browser + QR for phone
./remagic repair-ssh           # fixes the "connection reset" SSH wedge
```

Highlights:

- **Discovery without mDNS**: a dev-mode Paper Pro answers port 22 with an
  `unlocked` line before its SSH banner — a unique fingerprint the LAN scan
  looks for.
- **`wifi on`** flips the same switch as Settings → *SSH over Wi-Fi* (a marker
  file on the persistent `/data` partition), so it survives reboots.
- **`config`** serves a one-shot local web form (never hosted): type an API
  key with a real keyboard — laptop or phone via the QR — and it lands on the
  tablet over SSH, `chmod 600`.
- Pure Go SSH: no ssh binary, no ControlMaster, same behavior on
  macOS / Linux / Windows.

`catalog.json` at the repo root is the app registry; the same file will feed
the on-device store app.
