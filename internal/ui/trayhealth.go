package ui

import (
	"io"
	"log"
	"strings"
	"sync"
)

// trayHealth reports whether the system tray initialised.
//
// Fyne's tray API returns no error when initialisation fails: the failure only
// reaches the standard logger. A missing tray is then completely silent, and
// because the tray is also the only quit entry point, the user is left with no
// window, no icon and no way to exit. Watchers capture the logger so the app can
// tell the user and offer a fallback.
type trayHealth struct {
	mu     sync.Mutex
	failed bool
	detail string
}

// watchTray tees the standard logger so tray failures can be detected. Output is
// forwarded unchanged, so normal logging is unaffected.
func watchTray() *trayHealth {
	health := &trayHealth{}
	log.SetOutput(io.MultiWriter(log.Writer(), health))
	return health
}

func (h *trayHealth) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "systray error") {
		h.mu.Lock()
		h.failed = true
		h.detail = strings.TrimSpace(string(p))
		h.mu.Unlock()
	}
	return len(p), nil
}

// Failed reports whether a tray error was logged, with the first detail line.
func (h *trayHealth) Failed() (bool, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.failed, h.detail
}
