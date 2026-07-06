package device

import (
	"bufio"
	"encoding/binary"
	"net"
	"sort"
	"strings"
	"sync"
	"time"
)

// Probe is what a TCP peek at port 22 reveals. The Paper Pro is uniquely
// fingerprintable: its per-connection SSH service leaks the secure-boot gate
// state ("unlocked") as a pre-banner line before the dropbear version string.
type Probe struct {
	Addr   string
	Gate   string // pre-banner line ("unlocked" on a dev-mode Paper Pro), or ""
	Banner string // e.g. SSH-2.0-dropbear_2025.88
}

func (p *Probe) IsPaperPro() bool {
	return p.Gate != "" && strings.Contains(strings.ToLower(p.Banner), "dropbear")
}

// IsDropbear also matches rM1/rM2 (no gate line) — and, admittedly, any other
// dropbear box on the network; callers should present it as "possible".
func (p *Probe) IsDropbear() bool {
	return strings.Contains(strings.ToLower(p.Banner), "dropbear")
}

// ProbeAddr peeks at addr:22 and reads up to the SSH banner.
func ProbeAddr(addr string, timeout time.Duration) (*Probe, error) {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(addr, "22"), timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(timeout))
	p := &Probe{Addr: addr}
	r := bufio.NewReader(conn)
	for i := 0; i < 3; i++ {
		line, err := r.ReadString('\n')
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "SSH-") {
			p.Banner = line
			break
		}
		if line != "" && p.Gate == "" {
			p.Gate = line
		}
		if err != nil {
			break
		}
	}
	return p, nil
}

// Discover probes the USB address plus every host on the machine's local /24
// networks, concurrently. Sub-second on a quiet Wi-Fi.
func Discover(timeout time.Duration) []*Probe {
	addrs := append([]string{DefaultUSBAddr}, lanHosts()...)
	seen := map[string]bool{}
	var (
		mu      sync.Mutex
		results []*Probe
		wg      sync.WaitGroup
	)
	sem := make(chan struct{}, 192)
	for _, a := range addrs {
		if seen[a] {
			continue
		}
		seen[a] = true
		wg.Add(1)
		sem <- struct{}{}
		go func(a string) {
			defer wg.Done()
			defer func() { <-sem }()
			p, err := ProbeAddr(a, timeout)
			if err != nil || !p.IsDropbear() {
				return
			}
			mu.Lock()
			results = append(results, p)
			mu.Unlock()
		}(a)
	}
	wg.Wait()
	sort.Slice(results, func(i, j int) bool { return results[i].Addr < results[j].Addr })
	return results
}

// lanHosts enumerates the hosts of each up, non-loopback IPv4 interface,
// clamped to at most a /24 so a corporate /16 never triggers a 65k scan.
func lanHosts() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipn.IP.To4()
			if ip4 == nil || ip4.IsLinkLocalUnicast() {
				continue
			}
			ones, _ := ipn.Mask.Size()
			if ones < 24 {
				ones = 24
			}
			mask := net.CIDRMask(ones, 32)
			base := binary.BigEndian.Uint32(ip4.Mask(mask))
			self := binary.BigEndian.Uint32(ip4)
			n := uint32(1) << (32 - ones)
			for i := uint32(1); i < n-1; i++ {
				host := base + i
				if host == self {
					continue
				}
				ip := make(net.IP, 4)
				binary.BigEndian.PutUint32(ip, host)
				out = append(out, ip.String())
			}
		}
	}
	return out
}
