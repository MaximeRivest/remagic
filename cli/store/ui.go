package main

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"log"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"github.com/maximerivest/remagic/cli/internal/qtfb"
)

const (
	screenW = qtfb.Width
	screenH = qtfb.Height

	marginX   = 90
	headerH   = 170
	rowsTop   = 230
	rowH      = 210
	sheetH    = 420
	noticeBar = 110
)

// UI draws the whole screen into an 8-bit gray canvas, then blits it to the
// RGB565 qtfb framebuffer. E-ink is effectively grayscale; this keeps every
// drawing routine trivial.
type UI struct {
	canvas *image.Gray
	reg    *opentype.Font
	bold   *opentype.Font
	faces  map[[2]int]font.Face
}

func NewUI() *UI {
	reg, err := opentype.Parse(fontRegular)
	if err != nil {
		log.Fatal("font:", err)
	}
	bold, err := opentype.Parse(fontBold)
	if err != nil {
		log.Fatal("font:", err)
	}
	return &UI{
		canvas: image.NewGray(image.Rect(0, 0, screenW, screenH)),
		reg:    reg,
		bold:   bold,
		faces:  map[[2]int]font.Face{},
	}
}

func (u *UI) face(bold bool, size int) font.Face {
	k := [2]int{0, size}
	src := u.reg
	if bold {
		k[0], src = 1, u.bold
	}
	if f, ok := u.faces[k]; ok {
		return f
	}
	f, err := opentype.NewFace(src, &opentype.FaceOptions{
		Size: float64(size), DPI: 72, Hinting: font.HintingFull,
	})
	if err != nil {
		log.Fatal("face:", err)
	}
	u.faces[k] = f
	return f
}

// text draws s with its baseline at (x, y); returns the advance in pixels.
func (u *UI) text(x, y int, bold bool, size int, gray uint8, s string) int {
	d := font.Drawer{
		Dst:  u.canvas,
		Src:  image.NewUniform(color.Gray{gray}),
		Face: u.face(bold, size),
		Dot:  fixed.P(x, y),
	}
	d.DrawString(s)
	return (d.Dot.X - fixed.I(x)).Ceil()
}

func (u *UI) textWidth(bold bool, size int, s string) int {
	d := font.Drawer{Face: u.face(bold, size)}
	return d.MeasureString(s).Ceil()
}

// truncate cuts s so it fits within w pixels, appending an ellipsis.
func (u *UI) truncate(bold bool, size int, s string, w int) string {
	if u.textWidth(bold, size, s) <= w {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && u.textWidth(bold, size, string(r)+"…") > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

func (u *UI) fillRect(x, y, w, h int, gray uint8) {
	draw.Draw(u.canvas, image.Rect(x, y, x+w, y+h), image.NewUniform(color.Gray{gray}), image.Point{}, draw.Src)
}

func (u *UI) frameRect(x, y, w, h, t int, gray uint8) {
	u.fillRect(x, y, w, t, gray)
	u.fillRect(x, y+h-t, w, t, gray)
	u.fillRect(x, y, t, h, gray)
	u.fillRect(x+w-t, y, t, h, gray)
}

func (u *UI) Clear() {
	u.fillRect(0, 0, screenW, screenH, 255)
}

func (u *UI) Header(title string) {
	u.text(marginX, 118, true, 64, 0, title)
	u.fillRect(0, headerH-4, screenW, 4, 0)
}

func (u *UI) Row(i int, name, version, desc, status string) {
	y := rowsTop + i*rowH
	u.text(marginX, y+78, true, 52, 0, u.truncate(true, 52, name, screenW-2*marginX-420))
	sw := u.textWidth(false, 36, status)
	u.text(screenW-marginX-sw, y+78, false, 36, 60, status)
	line := desc
	if version != "" {
		line = version + " — " + desc
	}
	u.text(marginX, y+140, false, 36, 90, u.truncate(false, 36, line, screenW-2*marginX))
	u.fillRect(marginX, y+rowH-10, screenW-2*marginX, 2, 190)
}

// RowAt maps a tap y to a row index, or -1.
func (u *UI) RowAt(y, n int) int {
	if y < rowsTop || y >= screenH-noticeBar {
		return -1
	}
	i := (y - rowsTop) / rowH
	if i < 0 || i >= n {
		return -1
	}
	return i
}

// Sheet draws the bottom confirmation sheet with Cancel / Install zones.
func (u *UI) Sheet(question string) {
	top := screenH - sheetH
	u.fillRect(0, top, screenW, sheetH, 255)
	u.fillRect(0, top, screenW, 6, 0)
	u.text(marginX, top+105, true, 44, 0, u.truncate(true, 44, question, screenW-2*marginX))
	// buttons
	by, bh := screenH-250, 130
	u.frameRect(marginX, by, 660, bh, 4, 0)
	cw := u.textWidth(false, 44, "Cancel")
	u.text(marginX+(660-cw)/2, by+86, false, 44, 0, "Cancel")
	u.fillRect(screenW-marginX-660, by, 660, bh, 0)
	iw := u.textWidth(true, 44, "Install")
	u.text(screenW-marginX-660+(660-iw)/2, by+86, true, 44, 255, "Install")
}

// SheetButtonAt: 0 = cancel, 1 = install, -1 = elsewhere.
func (u *UI) SheetButtonAt(x, y int) int {
	by, bh := screenH-250, 130
	if y >= by-30 && y < by+bh+30 {
		if x >= marginX-30 && x < marginX+660+30 {
			return 0
		}
		if x >= screenW-marginX-660-30 && x < screenW-marginX+30 {
			return 1
		}
	}
	return -1
}

func (u *UI) Notice(s string) {
	y := screenH - noticeBar
	u.fillRect(0, y, screenW, noticeBar, 220)
	w := u.textWidth(false, 38, s)
	u.text((screenW-w)/2, y+72, false, 38, 0, u.truncate(false, 38, s, screenW-2*marginX))
}

func (u *UI) CenterText(s string) {
	w := u.textWidth(false, 48, s)
	u.text((screenW-w)/2, screenH/3, false, 48, 0, s)
}

func (u *UI) SmallCenterText(s string) {
	s = u.truncate(false, 30, s, screenW-2*marginX)
	w := u.textWidth(false, 30, s)
	u.text((screenW-w)/2, screenH/3+70, false, 30, 90, s)
}

// Blit converts the gray canvas to RGB565 in the shared framebuffer and asks
// the server to refresh the full area.
func (u *UI) Blit(fb *qtfb.Client) {
	shm := fb.Framebuffer()
	pix := u.canvas.Pix
	for i, v := range pix {
		p := uint16(v>>3)<<11 | uint16(v>>2)<<5 | uint16(v>>3)
		binary.LittleEndian.PutUint16(shm[i*2:], p)
	}
	_ = fb.UpdatePartial(0, 0, screenW, screenH)
}
