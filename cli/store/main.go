// The remagic store — an AppLoad app that browses the catalog and installs
// apps right on the tablet: no laptop, no cable. Launched by AppLoad with
// QTFB_KEY set; `store --sync` is a headless diagnostic (catalog + installed
// state to stdout) usable over SSH.
package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/maximerivest/remagic/cli/internal/applocal"
	"github.com/maximerivest/remagic/cli/internal/qtfb"
	"github.com/maximerivest/remagic/cli/internal/registry"
)

//go:embed DejaVuSans.ttf
var fontRegular []byte

//go:embed DejaVuSans-Bold.ttf
var fontBold []byte

const catalogURL = "https://raw.githubusercontent.com/maximerivest/remagic/main/catalog.json"

type item struct {
	app       registry.App
	installed bool
}

type mode int

const (
	modeLoading mode = iota
	modeList
	modeConfirm
	modeBusy
	modeNotice // transient banner over the list
)

type store struct {
	ui       *UI
	fb       *qtfb.Client
	items    []item
	err      string
	mode     mode
	selected int
	notice   string
	quit     bool
	// async results
	catalogCh  chan []item
	progressCh chan string
	doneCh     chan error
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--sync" {
		os.Exit(syncMode())
	}
	key, err := strconv.Atoi(os.Getenv("QTFB_KEY"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "store: no QTFB_KEY — launch me from AppLoad (or use --sync)")
		os.Exit(1)
	}
	fb, err := qtfb.Connect(int32(key))
	if err != nil {
		fmt.Fprintln(os.Stderr, "store:", err)
		os.Exit(1)
	}
	defer fb.Terminate()

	s := &store{
		ui:         NewUI(),
		fb:         fb,
		mode:       modeLoading,
		catalogCh:  make(chan []item, 1),
		progressCh: make(chan string, 8),
		doneCh:     make(chan error, 1),
	}
	go s.fetchCatalog()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)

	s.render()
	var pressed bool
	var pressX, pressY int32
	for {
		select {
		case <-sig:
			return
		default:
		}
		events, err := s.fb.DrainEvents()
		for _, ev := range events {
			switch ev.Type {
			case qtfb.InputTouchPress, qtfb.InputPenPress:
				pressed, pressX, pressY = true, ev.X, ev.Y
			case qtfb.InputTouchUpdate, qtfb.InputPenUpdate:
				if pressed {
					pressX, pressY = ev.X, ev.Y
				}
			case qtfb.InputTouchRelease, qtfb.InputPenRelease:
				if pressed {
					pressed = false
					s.tap(int(pressX), int(pressY))
				}
			}
		}
		if err != nil || s.quit {
			return // window closed, or the ← was tapped
		}

		select {
		case items := <-s.catalogCh:
			s.items = items
			if s.mode == modeLoading {
				s.mode = modeList
			}
			s.render()
		case msg := <-s.progressCh:
			s.notice = msg
			s.render()
		case err := <-s.doneCh:
			if err != nil {
				s.notice = "failed: " + err.Error()
			} else {
				s.notice = "installed ✓ — reload AppLoad to see it"
				s.refreshInstalled()
			}
			s.mode = modeNotice
			s.render()
		default:
		}
		time.Sleep(15 * time.Millisecond)
	}
}

func (s *store) fetchCatalog() {
	cat, err := registry.FetchWith(applocal.HTTPClient(), catalogURL)
	if err != nil {
		s.err = err.Error()
		s.catalogCh <- nil
		return
	}
	installed := applocal.Installed()
	var items []item
	for _, a := range cat.Apps {
		items = append(items, item{app: a, installed: installed[a.ID]})
	}
	s.catalogCh <- items
}

func (s *store) refreshInstalled() {
	installed := applocal.Installed()
	for i := range s.items {
		s.items[i].installed = installed[s.items[i].app.ID]
	}
}

// tap routes a lifted finger/pen to whatever is on screen at that point.
func (s *store) tap(x, y int) {
	// The header's ← : back out of whatever is open; from the list, leave.
	if s.ui.BackAt(x, y) && s.mode != modeBusy {
		if s.mode == modeConfirm {
			s.mode = modeList
			s.render()
			return
		}
		s.quit = true
		return
	}
	switch s.mode {
	case modeList, modeNotice:
		s.notice = ""
		if i := s.ui.RowAt(y, len(s.items)); i >= 0 {
			s.selected = i
			s.mode = modeConfirm
		} else {
			s.mode = modeList
		}
		s.render()
	case modeConfirm:
		switch s.ui.SheetButtonAt(x, y) {
		case 1: // install / update
			it := s.items[s.selected]
			s.mode = modeBusy
			s.notice = "working…"
			go func() {
				app := it.app
				s.doneCh <- applocal.Install(&app, func(p string) { s.progressCh <- p })
			}()
		case 0: // cancel
			s.mode = modeList
		default:
			return // taps outside the sheet are ignored while confirming
		}
		s.render()
	case modeBusy:
		// no interactions while installing
	}
}

func (s *store) render() {
	s.ui.Clear()
	s.ui.Header("remagic store")
	switch {
	case s.mode == modeLoading:
		s.ui.CenterText("fetching the catalog…")
	case s.err != "":
		s.ui.CenterText("catalog unavailable — is Wi-Fi on?")
		s.ui.SmallCenterText(s.err)
	case len(s.items) == 0:
		s.ui.CenterText("the catalog is empty")
	default:
		for i, it := range s.items {
			status := "tap to install"
			if it.installed {
				status = "installed ✓"
				if v := installedVersion(it.app.ID); v != "" && v != it.app.Version {
					status = "update " + v + " → " + it.app.Version
				}
			}
			s.ui.Row(i, it.app.Name, it.app.Version, it.app.Description, status)
		}
	}
	if s.mode == modeConfirm {
		it := s.items[s.selected]
		verb := "Install"
		if it.installed {
			verb = "Reinstall / update"
		}
		s.ui.Sheet(fmt.Sprintf("%s %s %s?", verb, it.app.Name, it.app.Version))
	}
	if s.notice != "" {
		s.ui.Notice(s.notice)
	}
	s.ui.Blit(s.fb)
}

// installedVersion reads the version out of an installed app's manifest.
func installedVersion(id string) string {
	raw, err := os.ReadFile(applocal.ApploadDir + "/" + id + "/external.manifest.json")
	if err != nil {
		return ""
	}
	// tolerant scan (no full JSON dep needed for one field)
	return jsonField(string(raw), "version")
}

func jsonField(s, key string) string {
	pat := `"` + key + `"`
	i := indexOf(s, pat)
	if i < 0 {
		return ""
	}
	rest := s[i+len(pat):]
	q1 := indexOf(rest, `"`)
	if q1 < 0 {
		return ""
	}
	rest = rest[q1+1:]
	q2 := indexOf(rest, `"`)
	if q2 < 0 {
		return ""
	}
	return rest[:q2]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func syncMode() int {
	cat, err := registry.FetchWith(applocal.HTTPClient(), catalogURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "catalog:", err)
		return 1
	}
	installed := applocal.Installed()
	for _, a := range cat.Apps {
		state := "available"
		if installed[a.ID] {
			state = "installed"
		}
		fmt.Printf("%-20s %-8s %-10s %s\n", a.ID, a.Version, state, a.Description)
	}
	return 0
}
