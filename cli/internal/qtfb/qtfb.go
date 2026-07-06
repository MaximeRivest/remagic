// Package qtfb is a native client for AppLoad's framebuffer protocol:
// SOCK_SEQPACKET on /tmp/qtfb.sock + a shared-memory framebuffer.
// Wire format per rm-appload src/qtfb/common.h:
//
//	ClientMessage = 24 bytes, type:u8 @0, payload @4
//	ServerMessage = 32 bytes, type:u8 @0, payload @8
package qtfb

import (
	"encoding/binary"
	"fmt"
	"io"
	"time"

	"golang.org/x/sys/unix"
)

const (
	msgInitialize     = 0
	msgUpdate         = 1
	msgTerminate      = 3
	msgUserInput      = 4
	msgSetRefreshMode = 5

	updateAll     = 0
	updatePartial = 1

	// FBFMT_RMPP_RGB565: native 1620x2160, 2 bytes/pixel.
	FormatRGB565 = 3

	Width  = 1620
	Height = 2160
	BPP    = 2

	InputTouchPress   = 0x10
	InputTouchRelease = 0x11
	InputTouchUpdate  = 0x12
	InputPenPress     = 0x20
	InputPenRelease   = 0x21
	InputPenUpdate    = 0x22

	socketPath = "/tmp/qtfb.sock"
)

type InputEvent struct {
	Type  int32
	DevID int32
	X, Y  int32
	D     int32
}

type Client struct {
	fd  int
	shm []byte
}

// Connect initializes the framebuffer AppLoad allocated for us under `key`
// (the QTFB_KEY environment variable).
func Connect(key int32) (*Client, error) {
	fd, err := unix.Socket(unix.AF_UNIX, unix.SOCK_SEQPACKET, 0)
	if err != nil {
		return nil, fmt.Errorf("socket: %w", err)
	}
	if err := unix.Connect(fd, &unix.SockaddrUnix{Name: socketPath}); err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("connect %s: %w (is this running under AppLoad?)", socketPath, err)
	}

	var msg [24]byte
	msg[0] = msgInitialize
	binary.LittleEndian.PutUint32(msg[4:8], uint32(key))
	msg[8] = FormatRGB565
	if err := sendAll(fd, msg[:]); err != nil {
		unix.Close(fd)
		return nil, err
	}

	// Init reply: shmKey i32 @8, shmSize u64 @16. The server closing without
	// replying means the key was rejected.
	var reply [32]byte
	n, _, err := unix.Recvfrom(fd, reply[:], 0)
	if err != nil || n <= 0 {
		unix.Close(fd)
		return nil, fmt.Errorf("qtfb init rejected (err=%v n=%d)", err, n)
	}
	shmKey := int32(binary.LittleEndian.Uint32(reply[8:12]))
	shmSize := binary.LittleEndian.Uint64(reply[16:24])

	shmFd, err := unix.Open(fmt.Sprintf("/dev/shm/qtfb_%d", shmKey), unix.O_RDWR, 0)
	if err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("open shm: %w", err)
	}
	shm, err := unix.Mmap(shmFd, 0, int(shmSize), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	unix.Close(shmFd)
	if err != nil {
		unix.Close(fd)
		return nil, fmt.Errorf("mmap shm: %w", err)
	}
	if len(shm) < Width*Height*BPP {
		unix.Close(fd)
		return nil, fmt.Errorf("shm too small: %d", len(shm))
	}
	if err := unix.SetNonblock(fd, true); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return &Client{fd: fd, shm: shm}, nil
}

func (c *Client) Framebuffer() []byte { return c.shm }

func (c *Client) send(msg [24]byte) error { return sendAll(c.fd, msg[:]) }

func (c *Client) UpdateAll() error {
	var m [24]byte
	m[0] = msgUpdate
	binary.LittleEndian.PutUint32(m[4:8], updateAll)
	return c.send(m)
}

func (c *Client) UpdatePartial(x, y, w, h int32) error {
	var m [24]byte
	m[0] = msgUpdate
	binary.LittleEndian.PutUint32(m[4:8], updatePartial)
	binary.LittleEndian.PutUint32(m[8:12], uint32(x))
	binary.LittleEndian.PutUint32(m[12:16], uint32(y))
	binary.LittleEndian.PutUint32(m[16:20], uint32(w))
	binary.LittleEndian.PutUint32(m[20:24], uint32(h))
	return c.send(m)
}

func (c *Client) Terminate() {
	var m [24]byte
	m[0] = msgTerminate
	_ = c.send(m)
	unix.Munmap(c.shm)
	unix.Close(c.fd)
}

// DrainEvents returns pending input events; io.EOF means the window was
// closed server-side and the app must exit.
func (c *Client) DrainEvents() ([]InputEvent, error) {
	var out []InputEvent
	for {
		var buf [32]byte
		n, _, err := unix.Recvfrom(c.fd, buf[:], 0)
		if n == 0 && err == nil {
			return out, io.EOF
		}
		if err != nil {
			if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
				return out, nil
			}
			if err == unix.EINTR {
				continue
			}
			return out, err
		}
		if buf[0] == msgUserInput && n >= 28 {
			out = append(out, InputEvent{
				Type:  int32(binary.LittleEndian.Uint32(buf[8:12])),
				DevID: int32(binary.LittleEndian.Uint32(buf[12:16])),
				X:     int32(binary.LittleEndian.Uint32(buf[16:20])),
				Y:     int32(binary.LittleEndian.Uint32(buf[20:24])),
				D:     int32(binary.LittleEndian.Uint32(buf[24:28])),
			})
		}
	}
}

func sendAll(fd int, buf []byte) error {
	for {
		err := unix.Send(fd, buf, 0)
		if err == unix.EINTR {
			continue
		}
		// Non-blocking socket: retry briefly rather than dropping a protocol
		// message (they're 24 bytes; the server drains fast).
		if err == unix.EAGAIN || err == unix.EWOULDBLOCK {
			time.Sleep(2 * time.Millisecond)
			continue
		}
		if err != nil {
			return err
		}
		return nil
	}
}
