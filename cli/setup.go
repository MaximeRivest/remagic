package main

// remagic setup — the whole installer in one command, pure Go, no bash/ssh/git
// on the laptop. Mirrors scripts/01..03 of the shell installer:
//
//	1. preflight   (model check — Paper Pro only)
//	2. ssh key     (generated if you have none, then installed)
//	3. xovi bundle (loader + scripts, from asivery's pinned release)
//	4. AppLoad     (only appload.so into extensions.d; shims into exthome)
//	5. hashtable   (required for AppLoad on OS 3.27+)
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
	osv, _ := d.Run(`. /etc/os-release 2>/dev/null; printf %s "$IMG_VERSION"`)
	osv = strings.TrimSpace(osv)
	if blob, label := apploadSOForOS(osv); blob != nil {
		step("installing AppLoad for OS %s", label)
		if err := d.Push(blob, "/home/root/xovi/extensions.d/appload.so", "755"); err != nil {
			die("%v", err)
		}
		step("downloading AppLoad shims")
		apploadZip, err := fetchURL(apploadURL)
		if err != nil {
			die("download AppLoad shims: %v", err)
		}
		if err := d.Push(apploadZip, "/tmp/appload.zip", "644"); err != nil {
			die("%v", err)
		}
		if out, err := d.Run(`cd /tmp && rm -rf appload-unz && mkdir appload-unz && \
(unzip -oq appload.zip -d appload-unz || busybox unzip -o appload.zip -d appload-unz) && \
mkdir -p /home/root/xovi/exthome/appload && \
if [ -d appload-unz/shims ]; then cp -rf appload-unz/shims /home/root/xovi/exthome/appload/; fi && \
if [ -d appload-unz/exthome ]; then cp -rf appload-unz/exthome/. /home/root/xovi/exthome/; fi && \
rm -rf appload-unz appload.zip`); err != nil {
			die("install AppLoad shims: %v: %s", err, out)
		}
		ok("AppLoad installed (3.28-compatible build)")
	} else {
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
	}

	// ---- 5. Qt hashtable (AppLoad hard-crashes xochitl without it on 3.27+) -
	step("rebuilding the Qt resource hashtable")
	hashtabOK := rebuildHashtab(d)

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
	if !hashtabOK {
		warn("skipping xovi start — without the hashtab AppLoad crash-loops xochitl on this OS")
		warn("after rebuild: remagic rebuild-hashtab   then triple-press power")
	} else if out, err := d.Run(`systemctl stop xovi-firststart 2>/dev/null; systemctl reset-failed xovi-firststart 2>/dev/null; \
systemd-run --unit=xovi-firststart --collect --service-type=oneshot /home/root/xovi/start`); err != nil {
		warn("couldn't start immediately (%s) — triple-press power, or reboot", tail(out, 120))
	} else {
		ok("xovi started — AppLoad should appear on the tablet")
	}

	// ---- 9. the Store ----------------------------------------------------------
	step("installing the Store app (browse + install apps on the tablet itself)")
	if err := installFromCatalog(d, "store", catalogURL); err != nil {
		warn("store install skipped: %v — you can add it later: remagic install store", err)
	} else {
		ok("Store installed")
	}

	fmt.Println(`
  ✓ Done. On the tablet: open AppLoad — the Store is inside.

    remagic install <app>     install apps from this computer
    remagic config <app>      configure an app from your browser
    remagic wifi on           make the cable optional
    remagic doctor            health check any time

  Re-run 'remagic setup' after a reMarkable OS update.`)
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
