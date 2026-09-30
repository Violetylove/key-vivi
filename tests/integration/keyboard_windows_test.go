//go:build windows && integration

package integration_test

import (
	"testing"
	"time"

	"golang.org/x/sys/windows"
	"key-vivi/internal/keyboard"
	"key-vivi/internal/platform"
)

func TestLiveKeyboardHook(t *testing.T) {
	events := make(chan keyboard.Event, 16)
	stop, err := platform.StartKeyboardHook(events)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	send := windows.NewLazyDLL("user32.dll").NewProc("keybd_event")
	// This opt-in desktop test injects F24 press/release events.
	send.Call(0x87, 0, 0, 0)
	send.Call(0x87, 0, 2, 0)
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	down, up := false, false
	for !down || !up {
		select {
		case e := <-events:
			if e.VKCode == 0x87 {
				if e.IsDown {
					down = true
				} else {
					up = true
				}
			}
		case <-timeout.C:
			t.Fatalf("hook did not deliver both events: down=%v up=%v", down, up)
		}
	}
}
