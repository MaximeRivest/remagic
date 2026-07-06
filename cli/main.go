// remagic — the reMarkable companion CLI.
//
// One static binary that finds your tablet (USB or Wi-Fi), health-checks it,
// installs apps into AppLoad, and configures them from a real keyboard
// (browser form + QR for your phone). Requires only developer mode.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mdp/qrterminal/v3"

	"github.com/maximerivest/remagic/cli/internal/device"
	"github.com/maximerivest/remagic/cli/internal/registry"
	"github.com/maximerivest/remagic/cli/internal/webconfig"
)

const version = "0.1.0"

const usage = `remagic %s — the reMarkable companion

usage: remagic <command> [args]

  find                    discover tablets on USB and this machine's networks
  doctor                  connect and health-check the tablet
  key                     install your SSH key (no more password prompts)
  install <app|folder>    install an app from the catalog, or push a local
                          app folder straight into AppLoad
  config <app>            configure an app from your browser — prints a QR
                          code so you can use your phone's keyboard
  wifi on|off|status      SSH over Wi-Fi, so the cable is only ever needed once
  repair-ssh              fix the "connection reset" SSH wedge in-band
  version                 print the version

options (before the command):
  -host <addr>            tablet address (default: found automatically)
  -catalog <url>          app catalog URL
  -local                  config: serve on 127.0.0.1 only (no QR/phone)
`

func main() {
	host := ""
	catalogURL := registry.DefaultCatalogURL
	localOnly := false

	args := os.Args[1:]
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-host", "--host":
			if len(args) < 2 {
				die("-host needs a value")
			}
			host, args = args[1], args[2:]
		case "-catalog", "--catalog":
			if len(args) < 2 {
				die("-catalog needs a value")
			}
			catalogURL, args = args[1], args[2:]
		case "-local", "--local":
			localOnly, args = true, args[1:]
		default:
			die("unknown option %s", args[0])
		}
	}
	if len(args) == 0 {
		fmt.Printf(usage, version)
		return
	}
	cmd, args := args[0], args[1:]

	switch cmd {
	case "version":
		fmt.Println("remagic", version)
	case "find":
		cmdFind()
	case "doctor":
		cmdDoctor(mustConnect(host))
	case "key":
		cmdKey(mustConnect(host))
	case "install":
		if len(args) != 1 {
			die("usage: remagic install <app-id | local-folder>")
		}
		cmdInstall(mustConnect(host), args[0], catalogURL)
	case "config":
		if len(args) != 1 || args[0] != "riddle" {
			die("usage: remagic config riddle   (more apps as the catalog grows)")
		}
		cmdConfig(mustConnect(host), !localOnly)
	case "wifi":
		if len(args) != 1 {
			die("usage: remagic wifi on|off|status")
		}
		cmdWifi(mustConnect(host), args[0])
	case "repair-ssh":
		cmdRepairSSH(mustConnect(host))
	default:
		die("unknown command %q — run remagic with no arguments for help", cmd)
	}
}

func die(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "remagic: "+f+"\n", a...)
	os.Exit(1)
}

func step(f string, a ...any) { fmt.Printf("==> "+f+"\n", a...) }
func ok(f string, a ...any)   { fmt.Printf("  ✓ "+f+"\n", a...) }
func warn(f string, a ...any) { fmt.Printf("  ! "+f+"\n", a...) }

// findHost picks the tablet: USB first (sub-second), then a LAN sweep.
func findHost() string {
	if p, err := device.ProbeAddr(device.DefaultUSBAddr, 900*time.Millisecond); err == nil && p.IsDropbear() {
		return device.DefaultUSBAddr
	}
	step("no tablet on USB — sweeping local networks…")
	probes := device.Discover(500 * time.Millisecond)
	var tablets []*device.Probe
	for _, p := range probes {
		if p.IsPaperPro() {
			tablets = append(tablets, p)
		}
	}
	if len(tablets) == 0 && len(probes) == 1 {
		tablets = probes // a lone dropbear box is probably an rM1/rM2
	}
	switch len(tablets) {
	case 0:
		die("no tablet found. Is it awake, in developer mode, and on this network (or USB)?")
	case 1:
		ok("found tablet at %s", tablets[0].Addr)
	default:
		for _, p := range tablets {
			fmt.Printf("  %s  (%s)\n", p.Addr, p.Banner)
		}
		die("several candidates — pick one with -host <addr>")
	}
	return tablets[0].Addr
}

func mustConnect(host string) *device.Device {
	if host == "" {
		host = findHost()
	}
	d, err := device.Connect(host)
	if err != nil {
		die("%v", err)
	}
	return d
}

func cmdFind() {
	step("probing USB and local networks…")
	probes := device.Discover(500 * time.Millisecond)
	if len(probes) == 0 {
		warn("nothing found. Tablet awake? Developer mode on? Same network / USB plugged?")
		return
	}
	for _, p := range probes {
		kind := "dropbear device (maybe an rM1/rM2)"
		if p.IsPaperPro() {
			kind = "reMarkable Paper Pro (dev mode, " + p.Gate + ")"
		}
		usb := ""
		if p.Addr == device.DefaultUSBAddr {
			usb = " [USB]"
		}
		fmt.Printf("  %s%s — %s — %s\n", p.Addr, usb, kind, p.Banner)
	}
}

func cmdDoctor(d *device.Device) {
	defer d.Close()
	get := func(cmd string) string {
		out, _ := d.Run(cmd)
		return strings.TrimSpace(out)
	}
	step("device")
	fmt.Printf("  model:      %s\n", get("cat /proc/device-tree/model 2>/dev/null | tr -d '\\0'"))
	fmt.Printf("  os:         %s\n", get(". /etc/os-release 2>/dev/null; printf %s \"$IMG_VERSION\""))
	// The marker's battery also exposes capacity and sorts first; the tablet
	// battery is the max1726x fuel gauge.
	fmt.Printf("  battery:    %s%%\n", get("cat /sys/class/power_supply/max1726x_battery/capacity 2>/dev/null || cat /sys/class/power_supply/*/capacity 2>/dev/null | sort -n | tail -n1"))
	// BusyBox df wraps long device names onto a second line; count fields
	// from the end instead of the start.
	fmt.Printf("  free space: %s\n", get("df -h /home | tail -n1 | awk '{print $(NF-2)}'"))

	step("tinkering stack")
	check := func(label, cmd string) {
		if _, err := d.Run(cmd); err == nil {
			ok("%s", label)
		} else {
			warn("%s — missing (run the remagic installer)", label)
		}
	}
	check("xovi installed", "test -d /home/root/xovi")
	check("AppLoad installed", "test -f /home/root/xovi/extensions.d/appload.so")
	// Any of the community persistence flavors counts.
	if _, err := d.Run("systemctl is-enabled xovi-tripletap 2>/dev/null || systemctl is-enabled xovi-always-on 2>/dev/null || systemctl is-enabled xovi-boot 2>/dev/null"); err == nil {
		ok("boot persistence (tripletap / always-on unit)")
	} else {
		warn("no boot persistence — xovi needs a manual start after reboots")
	}
	if _, err := d.Run("test -e /data/internal/rm_enable_ssh_wifi_marker"); err == nil {
		ok("SSH over Wi-Fi enabled")
	} else {
		warn("SSH over Wi-Fi off — run: remagic wifi on   (then the cable is optional)")
	}

	step("SSH health")
	if _, err := d.Run("test -e /etc/dropbear/dropbear_ed25519_host_key"); err == nil {
		ok("host-key store mounted (new connections will work)")
	} else {
		warn("host-key store BROKEN — new SSH connections will be reset at key")
		warn("exchange. Run: remagic repair-ssh")
	}
	if _, err := d.Run("mountpoint -q /etc"); err == nil {
		ok("/etc overlay mounted")
	} else {
		warn("/etc overlay is down (harmless until reboot; repair-ssh restores it)")
	}
	apps := get("ls " + device.ApploadDir + " 2>/dev/null")
	if apps != "" {
		step("AppLoad apps")
		for _, a := range strings.Fields(apps) {
			fmt.Printf("  %s\n", a)
		}
	}
}

func cmdKey(d *device.Device) {
	defer d.Close()
	home, _ := os.UserHomeDir()
	var pub []byte
	var err error
	for _, name := range []string{"id_ed25519.pub", "id_rsa.pub"} {
		if pub, err = os.ReadFile(filepath.Join(home, ".ssh", name)); err == nil {
			break
		}
	}
	if err != nil {
		die("no SSH public key found in ~/.ssh — create one with: ssh-keygen -t ed25519")
	}
	key := strings.TrimSpace(string(pub))
	cmd := fmt.Sprintf(
		"mkdir -p /home/root/.ssh && chmod 700 /home/root/.ssh && touch /home/root/.ssh/authorized_keys && chmod 600 /home/root/.ssh/authorized_keys && grep -qF '%s' /home/root/.ssh/authorized_keys || echo '%s' >> /home/root/.ssh/authorized_keys",
		key, key)
	if out, err := d.Run(cmd); err != nil {
		die("installing key: %v: %s", err, out)
	}
	ok("SSH key installed — no more password prompts.")
}

func cmdInstall(d *device.Device, what, catalogURL string) {
	defer d.Close()
	if st, err := os.Stat(what); err == nil && st.IsDir() {
		name := filepath.Base(strings.TrimRight(what, "/"))
		step("pushing %s → AppLoad/%s", what, name)
		if err := d.PushDir(what, device.ApploadDir+"/"+name); err != nil {
			die("%v", err)
		}
		ok("installed. On the tablet: open AppLoad and tap Reload.")
		return
	}

	step("looking up %q in the catalog", what)
	cat, err := registry.Fetch(catalogURL)
	if err != nil {
		die("%v", err)
	}
	app := cat.Find(what)
	if app == nil {
		var ids []string
		for _, a := range cat.Apps {
			ids = append(ids, a.ID)
		}
		die("no app %q. Available: %s", what, strings.Join(ids, ", "))
	}
	step("downloading %s %s", app.Name, app.Version)
	zip, err := app.Download()
	if err != nil {
		die("%v", err)
	}
	defer os.Remove(zip)
	content, err := os.ReadFile(zip)
	if err != nil {
		die("%v", err)
	}
	step("installing onto the tablet")
	if err := d.Push(content, "/tmp/remagic-app.zip", "644"); err != nil {
		die("%v", err)
	}
	target := device.ApploadDir + "/" + app.ID
	cmd := fmt.Sprintf("mkdir -p %s && (unzip -oq /tmp/remagic-app.zip -d %s || busybox unzip -o /tmp/remagic-app.zip -d %s) && rm -f /tmp/remagic-app.zip",
		target, target, target)
	if out, err := d.Run(cmd); err != nil {
		die("unpack failed: %v: %s", err, out)
	}
	ok("%s %s installed. On the tablet: open AppLoad and tap Reload.", app.Name, app.Version)
}

func cmdConfig(d *device.Device, lan bool) {
	defer d.Close()
	err := webconfig.Serve(d, webconfig.Riddle(), lan, func(url string) {
		step("settings form is live (one save, then it closes):")
		fmt.Printf("\n  %s\n\n", url)
		if lan {
			fmt.Println("  scan to open on your phone:")
			fmt.Println()
			qrterminal.GenerateWithConfig(url, qrterminal.Config{
				Level: qrterminal.L, Writer: os.Stdout,
				BlackChar: qrterminal.BLACK, WhiteChar: qrterminal.WHITE,
				HalfBlocks: true, QuietZone: 2,
			})
			fmt.Println()
		}
	})
	if err != nil {
		die("%v", err)
	}
	ok("saved to the tablet. Relaunch The Diary to pick it up.")
}

// cmdWifi flips the same switch as Settings → "SSH over Wi-Fi": the
// dropbear-wlan socket is always enabled but gated on a marker file living on
// the persistent /data partition, so this survives reboots.
func cmdWifi(d *device.Device, action string) {
	defer d.Close()
	const marker = "/data/internal/rm_enable_ssh_wifi_marker"
	switch action {
	case "on":
		if out, err := d.Run("mkdir -p /data/internal && touch " + marker + " && systemctl start dropbear-wlan.socket"); err != nil {
			die("enable failed: %v: %s", err, out)
		}
		ip, _ := d.Run("ip -4 addr show wlan0 2>/dev/null | sed -n 's/.*inet \\([0-9.]*\\).*/\\1/p'")
		ip = strings.TrimSpace(ip)
		if ip == "" {
			ok("enabled — will listen once the tablet joins a Wi-Fi network.")
		} else {
			ok("enabled — the tablet now answers on %s (no cable needed).", ip)
		}
	case "off":
		if out, err := d.Run("rm -f " + marker + " && systemctl stop dropbear-wlan.socket"); err != nil {
			die("disable failed: %v: %s", err, out)
		}
		ok("SSH over Wi-Fi disabled.")
	case "status":
		_, mErr := d.Run("test -e " + marker)
		act, _ := d.Run("systemctl is-active dropbear-wlan.socket")
		ip, _ := d.Run("ip -4 addr show wlan0 2>/dev/null | sed -n 's/.*inet \\([0-9.]*\\).*/\\1/p'")
		fmt.Printf("  marker:  %v\n  socket:  %s  wlan ip: %s\n",
			mErr == nil, strings.TrimSpace(act), strings.TrimSpace(ip))
	default:
		die("usage: remagic wifi on|off|status")
	}
}

// cmdRepairSSH applies the known fix for the Paper Pro SSH wedge: something
// (xovi-tripletap's installer, an OS quirk) ran `umount -R /etc` and took the
// dropbear host-key bind mount down with it — new connections then die at key
// exchange until reboot. Restores the /etc overlay, the key mount, and
// read-only /.
func cmdRepairSSH(d *device.Device) {
	defer d.Close()
	step("repairing the SSH key store")
	script := `
grep -qE "reMarkable (Ferrari|Chiappa|Tatsu)" /proc/device-tree/model || { echo "not a Paper Pro; nothing to do"; exit 0; }
if ! mountpoint -q /etc; then
    mount -t overlay overlay -o lowerdir=/etc,upperdir=/var/volatile/etc,workdir=/var/volatile/.etc-work,uuid=on /etc \
      || mount -t overlay overlay -o lowerdir=/etc,upperdir=/var/volatile/etc,workdir=/var/volatile/.etc-work /etc
fi
if [ ! -e /etc/dropbear/dropbear_ed25519_host_key ]; then
    systemctl restart etc-dropbear.mount 2>/dev/null || true
fi
if [ ! -e /etc/dropbear/dropbear_ed25519_host_key ] && [ -e /home/root/.dropbear/dropbear_ed25519_host_key ]; then
    mount --bind /home/root/.dropbear /etc/dropbear
fi
mount -o remount,ro / 2>/dev/null || true
test -e /etc/dropbear/dropbear_ed25519_host_key && echo REPAIRED
`
	out, err := d.Run(script)
	if err != nil {
		die("repair failed: %v: %s", err, out)
	}
	if strings.Contains(out, "REPAIRED") {
		ok("host-key store restored — new SSH connections work again.")
	} else {
		fmt.Print(out)
	}
}
