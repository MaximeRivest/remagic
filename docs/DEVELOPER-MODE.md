# Turning on Developer Mode

Developer mode unlocks your reMarkable Paper Pro so you can run your own
software on it — SSH in as root, install launchers like AppLoad, and load the
apps in this ecosystem. This guide walks you through it in plain language.

> **Read this first — the one thing that surprises everyone.**
> Enabling developer mode **erases everything on the tablet** (a factory
> reset). This is a security measure built into the device by reMarkable, not
> something any tool can remove. Your notebooks are safe **if** they are synced
> to the reMarkable cloud first — they download again afterward. Anything not
> synced is gone. **Back up before you start.**

---

## Before you begin

1. **Charge the tablet** to at least 30%. Don't do this on a dying battery.
2. **Sync your notebooks.** Open each notebook so it uploads, or confirm the
   cloud sync indicator is settled. If you don't use the reMarkable cloud,
   copy anything precious off the device first (via the USB web interface at
   `http://10.11.99.1` while plugged in, or the desktop/mobile app).
3. **Know that this is reversible.** You can turn developer mode back off later
   (through reMarkable's recovery application; that also factory-resets),
   returning the device to its stock, secure state. Developer mode does **not**
   void your hardware warranty, but reMarkable won't support software problems
   you introduce.

---

## Enable it

On the tablet, follow this path (reMarkable's official wording):

> **Settings → General → Paper Tablet → Software → Advanced → Developer Mode**

1. Tap **Developer mode**, then **Enable**.
2. Read the warning. reMarkable states plainly: *"enabling developer mode also
   performs a factory reset … data on the device at the time of enabling
   developer mode will be lost."* It also warns that a notice appears at **every
   boot** — that's normal and cannot be removed; it's part of the security
   design.
3. Confirm. The tablet resets and reboots into developer mode. Set it up again
   like a new device (sign in, let your notebooks sync back down).

> Developer mode is specific to the Paper Pro line here; it isn't offered on the
> reMarkable 1 and 2.

That's the whole manual part. Everything after this — SSH, launchers, apps —
is automated by the installer in this repo.

---

## Connect over USB

Developer mode gives you a root shell over a USB network connection.

1. Plug the tablet into your computer with the USB cable.
2. The tablet appears as a USB network device at **`10.11.99.1`**.
3. Find the SSH password on the tablet: **Settings → General → Help →
   Copyrights and licenses** (or the About screen) shows a
   **GPLv3 Compliance / SSH** section with a one-time password and the
   `root@10.11.99.1` address.
4. Test it:

   ```sh
   ssh root@10.11.99.1
   ```

   Enter that password when asked. You're in.

> **Tip:** the installer sets up key-based login for you, so this is the last
> time you'll need to type that password.

---

## What "unlocked" actually means (and doesn't)

Developer mode relaxes the parts of the device's security that keep *you* out
of *your own* tablet's software:

- ✅ You get **root SSH**.
- ✅ You can **modify the root filesystem** and run your own programs.
- ✅ You can **load a custom kernel** (advanced; not needed for apps).

It deliberately leaves a few things locked, and these are **not** bugs to work
around — they're what makes developer mode safe and reversible:

- 🔒 The **bootloader stays signed** (you can't replace the earliest boot code).
- 🔒 **Disk encryption stays on** (your data is still encrypted at rest).
- 🔒 The **boot notice** and the **enable-time reset** stay.

If a tool ever promises to remove the reset or the boot notice, be skeptical:
doing so means defeating the device's secure boot, which breaks on every OS
update and puts your device (and warranty status) at risk.

---

## Next step

Head back to the [README](../README.md) and run the installer. One command
sets up SSH keys, installs the AppLoad launcher, and gets you ready to add
apps — no terminal wrangling required.
