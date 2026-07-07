// Package device speaks SSH to a reMarkable in developer mode. Pure Go —
// no ssh binary, no ControlMaster, works the same on macOS/Linux/Windows.
package device

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/term"
)

// The tablet's fixed address on the USB ethernet gadget.
const DefaultUSBAddr = "10.11.99.1"

const ApploadDir = "/home/root/xovi/exthome/appload"

type Device struct {
	Addr   string
	client *ssh.Client
}

// nonInteractiveAuths collects the auth methods that never prompt: the
// ssh-agent plus the usual key files.
func nonInteractiveAuths() []ssh.AuthMethod {
	var auths []ssh.AuthMethod
	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if conn, err := net.Dial("unix", sock); err == nil {
			auths = append(auths, ssh.PublicKeysCallback(agent.NewClient(conn).Signers))
		}
	}
	home, _ := os.UserHomeDir()
	for _, name := range []string{"id_ed25519", "id_rsa"} {
		b, err := os.ReadFile(filepath.Join(home, ".ssh", name))
		if err != nil {
			continue
		}
		if signer, err := ssh.ParsePrivateKey(b); err == nil {
			auths = append(auths, ssh.PublicKeys(signer))
		}
	}
	return auths
}

// ConnectKeyOnly dials with only non-interactive auth (agent + key files) on
// a genuinely fresh connection. Use it to prove the NEXT person to connect
// will get in — e.g. after an installer step that can wedge the SSH server.
func ConnectKeyOnly(addr string) (*Device, error) {
	cfg := &ssh.ClientConfig{
		User:            "root",
		Auth:            nonInteractiveAuths(),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         8 * time.Second,
	}
	client, err := ssh.Dial("tcp", net.JoinHostPort(addr, "22"), cfg)
	if err != nil {
		return nil, fmt.Errorf("connect root@%s: %w", addr, err)
	}
	return &Device{Addr: addr, client: client}, nil
}

// Connect dials root@addr trying, in order: the ssh-agent, the usual key
// files, then an interactive password prompt (the code on the tablet under
// Settings → Help → Copyrights and licenses).
func Connect(addr string) (*Device, error) {
	auths := nonInteractiveAuths()
	auths = append(auths, ssh.RetryableAuthMethod(ssh.PasswordCallback(func() (string, error) {
		fmt.Fprintf(os.Stderr, "password for root@%s (tablet: Settings → Help → Copyrights and licenses): ", addr)
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		return strings.TrimSpace(string(b)), err
	}), 3))
	cfg := &ssh.ClientConfig{
		User: "root",
		Auth: auths,
		// The device regenerates its host key on every factory reset, and
		// first contact is exactly when this tool runs — pinning would strand
		// users at the moment they need it. Trust-on-connect, like the
		// installer always has.
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         8 * time.Second,
	}
	client, err := ssh.Dial("tcp", net.JoinHostPort(addr, "22"), cfg)
	if err != nil {
		return nil, fmt.Errorf("connect root@%s: %w", addr, err)
	}
	return &Device{Addr: addr, client: client}, nil
}

func (d *Device) Close() {
	if d.client != nil {
		d.client.Close()
	}
}

// Run executes one command; returns combined output.
func (d *Device) Run(cmd string) (string, error) {
	return d.RunIn(cmd, nil)
}

// RunIn executes one command with stdin streamed from r.
func (d *Device) RunIn(cmd string, r io.Reader) (string, error) {
	sess, err := d.client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	// stdout and stderr are copied by separate goroutines; feeding them the
	// same bare buffer is a data race that intermittently loses output.
	var out bytes.Buffer
	lw := &lockedWriter{w: &out}
	sess.Stdout = lw
	sess.Stderr = lw
	if r != nil {
		sess.Stdin = r
	}
	err = sess.Run(cmd)
	lw.mu.Lock()
	defer lw.mu.Unlock()
	return out.String(), err
}

type lockedWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// Push writes content to a remote path with the given chmod mode. Streams
// through cat — the BusyBox device needs no sftp subsystem.
func (d *Device) Push(content []byte, remote, mode string) error {
	cmd := fmt.Sprintf("mkdir -p %s && cat > %s && chmod %s %s",
		shq(filepath.Dir(remote)), shq(remote), mode, shq(remote))
	if out, err := d.RunIn(cmd, bytes.NewReader(content)); err != nil {
		return fmt.Errorf("push %s: %w: %s", remote, err, strings.TrimSpace(out))
	}
	return nil
}

// PushDir mirrors a local directory to remoteDir (created if needed) by
// streaming a tar.gz — one round-trip regardless of file count.
func (d *Device) PushDir(localDir, remoteDir string) error {
	pr, pw := io.Pipe()
	go func() {
		gz := gzip.NewWriter(pw)
		tw := tar.NewWriter(gz)
		err := filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(localDir, path)
			if err != nil || rel == "." {
				return err
			}
			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			hdr.Name = filepath.ToSlash(rel)
			// Everything on the device is root's; don't leak local uids.
			hdr.Uid, hdr.Gid = 0, 0
			hdr.Uname, hdr.Gname = "root", "root"
			if err := tw.WriteHeader(hdr); err != nil {
				return err
			}
			if info.Mode().IsRegular() {
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				defer f.Close()
				if _, err := io.Copy(tw, f); err != nil {
					return err
				}
			}
			return nil
		})
		if err == nil {
			err = tw.Close()
		} else {
			tw.Close()
		}
		if e := gz.Close(); err == nil {
			err = e
		}
		pw.CloseWithError(err)
	}()
	cmd := fmt.Sprintf("mkdir -p %s && tar -xzf - -C %s", shq(remoteDir), shq(remoteDir))
	if out, err := d.RunIn(cmd, pr); err != nil {
		return fmt.Errorf("push dir %s: %w: %s", remoteDir, err, strings.TrimSpace(out))
	}
	return nil
}

// shq single-quotes s for a POSIX shell.
func shq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
