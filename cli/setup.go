package main

// remagic setup — the whole installer in one command, pure Go, no bash/ssh/git
// on the laptop. Mirrors scripts/01..03 of the shell installer:
//
//	1. preflight   (model check — Paper Pro only)
//	2. ssh key     (generated if you have none, then installed)
//	3. xovi bundle (loader + scripts, from asivery's pinned release)
//	4. AppLoad     (only appload.so into extensions.d; shims into exthome)
//	5. hashtable   (required for AppLoad's UI button; built non-interactively)
//	6. tripletap   (power-button persistence; script streamed from the laptop
//	                over verified TLS, executed on the device)
//	7. ssh repair  (tripletap's installer knocks over the host-key mount on
//	                fresh devices; we fix it in-band over the live connection)
//	8. start xovi  (no reboot needed)
//	9. the Store   (so the next app install needs no computer at all)
//
// One password prompt at most: the single SSH connection is reused throughout.

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/maximerivest/remagic/cli/internal/device"
)

// Pinned upstream releases (same pins as scripts/sources.env).
const (
	xoviBundleURL = "https://github.com/asivery/rm-xovi-extensions/releases/download/v19-23052026/xovi-aarch64.tar.gz"
	apploadURL    = "https://github.com/asivery/rm-appload/releases/download/v0.5.3/appload-aarch64.zip"
	tripletapURL  = "https://raw.githubusercontent.com/rmitchellscott/xovi-tripletap/main/install.sh"
)

func cmdSetup(d *device.Device, catalogURL string) {
	defer d.Close()

	fmt.Println(`
  remagic setup — make your reMarkable Paper Pro magical.

  This will, over the connection just opened:
    1. Verify this is a Paper Pro in developer mode
    2. Install an SSH key (no more passwords; one is created if you have none)
    3. Install xovi + the AppLoad launcher (official upstream releases)
    4. Set up power-button persistence (triple-press toggles xovi)
    5. Install the Store app, so future installs need no computer

  Nothing here touches your notebooks or the bootloader. A triple-press or a
  reboot returns you to stock reMarkable instantly.`)
	fmt.Println()

	// ---- 1. preflight -----------------------------------------------------
	step("checking the device")
	model, _ := d.Run(`cat /proc/device-tree/model 2>/dev/null | tr -d '\0'`)
	model = strings.TrimSpace(model)
	if !isPaperPro(model) {
		die("device reports %q — remagic targets the reMarkable Paper Pro (Ferrari/Chiappa/Tatsu); refusing to touch an unrecognized device", model)
	}
	ok("device: %s", model)
	if osv, err := d.Run(`. /etc/os-release 2>/dev/null; printf %s "$IMG_VERSION"`); err == nil && strings.TrimSpace(osv) != "" {
		ok("OS version: %s", strings.TrimSpace(osv))
	}

	// ---- 2. ssh key --------------------------------------------------------
	step("setting up passwordless SSH")
	if err := ensureSSHKey(d); err != nil {
		warn("SSH key setup failed (%v) — continuing; you'll keep typing the password", err)
	}

	// ---- 3. xovi bundle ----------------------------------------------------
	step("downloading the xovi bundle")
	xovi, err := fetchURL(xoviBundleURL)
	if err != nil {
		die("download xovi: %v", err)
	}
	ok("%d KB", len(xovi)/1024)
	step("installing xovi onto the tablet")
	if err := d.Push(xovi, "/tmp/xovi.tar.gz", "644"); err != nil {
		die("%v", err)
	}
	if out, err := d.Run("tar -xzf /tmp/xovi.tar.gz -C /home/root && rm -f /tmp/xovi.tar.gz"); err != nil {
		die("extract xovi: %v: %s", err, out)
	}
	// Activate the message broker (ships inactive; several community apps use it).
	d.Run(`[ -f /home/root/xovi/inactive-extensions/xovi-message-broker.so ] && \
mv -f /home/root/xovi/inactive-extensions/xovi-message-broker.so /home/root/xovi/extensions.d/ 2>/dev/null; true`)
	ok("xovi installed at /home/root/xovi")

	// ---- 4. AppLoad ---------------------------------------------------------
	step("downloading AppLoad")
	appload, err := fetchURL(apploadURL)
	if err != nil {
		die("download AppLoad: %v", err)
	}
	ok("%d KB", len(appload)/1024)
	step("installing the AppLoad launcher")
	if err := d.Push(appload, "/tmp/appload.zip", "644"); err != nil {
		die("%v", err)
	}
	// ONLY appload.so is a xovi extension → extensions.d/. The zip also
	// carries shims/qtfb-shim*.so (runtime pieces for windowed apps) which
	// must NOT land in extensions.d/ or xovi tries to load them and errors.
	if out, err := d.Run(`cd /tmp && rm -rf appload-unz && mkdir appload-unz && \
(unzip -oq appload.zip -d appload-unz || busybox unzip -o appload.zip -d appload-unz) && \
cp -f appload-unz/appload.so /home/root/xovi/extensions.d/ && \
mkdir -p /home/root/xovi/exthome/appload && \
if [ -d appload-unz/shims ]; then cp -rf appload-unz/shims /home/root/xovi/exthome/appload/; fi && \
if [ -d appload-unz/exthome ]; then cp -rf appload-unz/exthome/. /home/root/xovi/exthome/; fi && \
rm -rf appload-unz appload.zip`); err != nil {
		die("install AppLoad: %v: %s", err, out)
	}
	ok("AppLoad installed")

	// ---- 5. Qt hashtable -----------------------------------------------------
	// Without the hashtab, qt-resource-rebuilder can't patch xochitl's QML and
	// the AppLoad button never appears in the UI — this step is NOT optional.
	// The stock rebuild_hashtable script is interactive (press-enter prompt,
	// parses xochitl stdout), so we run our own non-interactive rebuild,
	// detached via systemd: stopping/starting xochitl drops Wi-Fi SSH
	// connections, so the work must survive our session and we poll for the
	// result over fresh connections.
	step("building the Qt resource hashtable (AppLoad's UI hook needs it; ~1-2 min)")
	if err := rebuildHashtable(d); err != nil {
		warn("hashtable build failed: %v", err)
		warn("the AppLoad button may not appear — rerun 'remagic setup' or run /home/root/xovi/rebuild_hashtable over ssh")
	} else {
		ok("hashtable built")
	}
	// The rebuild (and later steps) can drop the shared connection; heal it.
	if _, err := d.Run("true"); err != nil {
		if fresh, cerr := device.ConnectKeyOnly(d.Addr); cerr == nil {
			*d = *fresh
		}
	}

	// ---- 6. tripletap persistence -------------------------------------------
	step("installing power-button persistence (xovi-tripletap)")
	fmt.Println("    triple-press the power button to toggle xovi — survives reboots,")
	fmt.Println("    needs no computer, and can't bootloop you")
	tt, err := fetchURL(tripletapURL)
	if err != nil {
		warn("download tripletap: %v — skipping; start xovi manually after reboots", err)
	} else if out, err := d.RunIn("bash -s", strings.NewReader(string(tt))); err != nil {
		warn("tripletap install didn't complete: %v: %s", err, tail(out, 300))
		warn("you can still start xovi manually: ssh root@%s '/home/root/xovi/start'", d.Addr)
	} else {
		ok("tripletap installed")
	}

	// ---- 7. repair the SSH keystore tripletap knocks over -------------------
	step("checking the SSH server survived")
	repairSSH(d)
	if fresh, err := device.ConnectKeyOnly(d.Addr); err == nil {
		fresh.Close()
		ok("SSH healthy — verified with a fresh connection")
	} else {
		warn("new SSH connections aren't completing; one reboot of the tablet fixes it")
	}

	// ---- 8. start xovi now ---------------------------------------------------
	step("starting xovi (no reboot needed)")
	if out, err := d.Run(`systemctl stop xovi-firststart 2>/dev/null; systemctl reset-failed xovi-firststart 2>/dev/null; \
systemd-run --unit=xovi-firststart --collect --service-type=oneshot /home/root/xovi/start`); err != nil {
		warn("couldn't start immediately (%s) — triple-press power, or reboot", tail(out, 120))
	} else {
		ok("xovi started — AppLoad should appear on the tablet")
	}

	// ---- 9. the Store ----------------------------------------------------------
	step("installing the Store app (browse + install apps on the tablet itself)")
	storeOK := false
	if err := installFromCatalog(d, "store", catalogURL); err == nil {
		storeOK = true
	} else {
		// Starting xovi restarts xochitl, which can drop the long-lived SSH
		// connection mid-push. A fresh connection almost always succeeds.
		if fresh, cerr := device.ConnectKeyOnly(d.Addr); cerr == nil {
			if err2 := installFromCatalog(fresh, "store", catalogURL); err2 == nil {
				storeOK = true
			} else {
				warn("store install skipped: %v — you can add it later: remagic install store", err2)
			}
			fresh.Close()
		} else {
			warn("store install skipped: %v — you can add it later: remagic install store", err)
		}
	}
	if storeOK {
		ok("Store installed")
	}

	fmt.Println(`
  ✓ Setup complete. Try it now, on the tablet:

    1. Wake the tablet — the reMarkable home screen looks unchanged. That's
       normal: your apps live inside the AppLoad launcher.
    2. Open AppLoad from the tablet's main menu (the icon xovi added).
       If you don't see it yet, triple-press the power button and wait a
       few seconds — that toggles xovi on.`)
	if storeOK {
		fmt.Println(`    3. Inside AppLoad, open the Store to browse and install apps —
       no computer needed from here on.`)
	} else {
		fmt.Println(`    3. Install apps from this computer with: remagic install <app>
       (the on-tablet Store was skipped; add it with: remagic install store)`)
	}
	fmt.Println(`
  Everyday commands:

    remagic install <app>     install apps from this computer
    remagic config <app>      configure an app from your browser
    remagic wifi on           make the cable optional
    remagic doctor            health check any time

  Triple-press the power button any time to toggle xovi on/off.
  Re-run 'remagic setup' after a reMarkable OS update.`)
}

// rebuildHashtable builds qt-resource-rebuilder's hashtab non-interactively.
// It runs xochitl once with only qt-resource-rebuilder loaded (in a scratch
// XOVI_ROOT) and QMLDIFF_HASHTAB_CREATE set; qmldiff hashes every QML/resource
// and writes the hashtab ~60-90s in. The whole thing runs as a transient
// systemd unit because it stops xochitl, which can drop Wi-Fi SSH sessions —
// we poll for completion over fresh connections instead.
func rebuildHashtable(d *device.Device) error {
	if _, err := d.Run("test -f /home/root/xovi/extensions.d/qt-resource-rebuilder.so"); err != nil {
		return nil // bundle does no QML patching; nothing to build
	}
	const script = `#!/bin/bash
set -u
xovi=/home/root/xovi
tab=$xovi/exthome/qt-resource-rebuilder/hashtab
systemctl stop xochitl 2>/dev/null
sleep 1
export XOVI_ROOT=/tmp/xovi-hashtab
rm -rf "$XOVI_ROOT"
mkdir -p "$XOVI_ROOT/extensions.d"
ln -s "$xovi/extensions.d/qt-resource-rebuilder.so" "$XOVI_ROOT/extensions.d/"
mkdir -p "$(dirname "$tab")"
rm -f "$tab"
QMLDIFF_HASHTAB_CREATE="$tab" QML_DISABLE_DISK_CACHE=1 LD_PRELOAD="$xovi/xovi.so" /usr/bin/xochitl >/dev/null 2>&1 &
pid=$!
for i in $(seq 1 150); do [ -s "$tab" ] && break; sleep 1; done
sleep 3
kill $pid 2>/dev/null; sleep 2; kill -9 $pid 2>/dev/null
rm -rf "$XOVI_ROOT" /tmp/remagic-hashtab.sh
[ -s "$tab" ]
`
	if err := d.Push([]byte(script), "/tmp/remagic-hashtab.sh", "755"); err != nil {
		return err
	}
	if out, err := d.Run(`systemctl stop remagic-hashtab 2>/dev/null; systemctl reset-failed remagic-hashtab 2>/dev/null; \
systemd-run --unit=remagic-hashtab --collect /bin/bash /tmp/remagic-hashtab.sh`); err != nil {
		return fmt.Errorf("launch rebuild unit: %v: %s", err, tail(out, 200))
	}
	// Poll over fresh connections; the unit stops xochitl, which can kill
	// existing SSH sessions (observed over Wi-Fi).
	deadline := time.Now().Add(4 * time.Minute)
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Second)
		fresh, err := device.ConnectKeyOnly(d.Addr)
		if err != nil {
			continue // device busy restarting xochitl; keep waiting
		}
		out, _ := fresh.Run("systemctl is-active remagic-hashtab || true")
		if !strings.Contains(out, "active") || strings.Contains(out, "inactive") || strings.Contains(out, "failed") {
			_, terr := fresh.Run("test -s /home/root/xovi/exthome/qt-resource-rebuilder/hashtab")
			fresh.Close()
			if terr != nil {
				return fmt.Errorf("rebuild finished but no hashtab was written")
			}
			return nil
		}
		fresh.Close()
	}
	return fmt.Errorf("timed out after 4 minutes")
}

func isPaperPro(model string) bool {
	for _, m := range []string{"Ferrari", "Chiappa", "Tatsu"} {
		if strings.Contains(model, m) {
			return true
		}
	}
	return false
}

// fetchURL downloads a release asset with the laptop's verified TLS.
func fetchURL(url string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s from %s", resp.Status, url)
	}
	return io.ReadAll(resp.Body)
}

// tail returns the last n bytes of s, for compact error reporting.
func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}
