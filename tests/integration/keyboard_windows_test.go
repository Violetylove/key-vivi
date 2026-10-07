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
	// 该测试仅在显式启用时运行，会注入 F24 的按下与抬起。
	send.Call(0x87, 0, 0, 0)
	send.Call(0x87, 0, 2, 0)
	timeout := time.NewTimer(2 * time.Second)
	defer timeout.Stop()
	down, up := false, false
	for !down || !up {
		select {
		case e := <-events:
			if e.When.IsZero() {
				t.Fatal("hook event has no capture timestamp")
			}
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

func TestHookOverflowStillWakesAndStops(t *testing.T) {
	events := make(chan keyboard.Event, 1)
	wakes := make(chan struct{}, 32)
	before := platform.HookDroppedCount()
	stop, err := platform.StartKeyboardHookWithWake(events, func() {
		select {
		case wakes <- struct{}{}:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	send := windows.NewLazyDLL("user32.dll").NewProc("keybd_event")
	for i := 0; i < 8; i++ {
		send.Call(0x87, 0, 0, 0)
		send.Call(0x87, 0, 2, 0)
	}
	deadline := time.After(2 * time.Second)
	for platform.HookDroppedCount() == before {
		select {
		case <-wakes:
		case <-deadline:
			t.Fatal("full buffer did not wake or report overflow")
		}
	}
	stopped := make(chan struct{})
	go func() { stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("overflow prevented hook shutdown")
	}
}
