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

// TestHookSelfCheck 覆盖应用使用的导出自检路径与回调计数，避免
// TestLiveKeyboardHook 通过、而计数器始终不涨的情况被漏掉。
func TestHookSelfCheck(t *testing.T) {
	events := make(chan keyboard.Event, 64)
	stop, err := platform.StartKeyboardHook(events)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	go func() {
		for range events {
		}
	}()
	before := platform.HookEventCount()
	platform.SendTestKey()
	deadline := time.Now().Add(2 * time.Second)
	for platform.HookEventCount() == before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if after := platform.HookEventCount(); after == before {
		t.Fatalf("SendTestKey produced no hook events (before=%d after=%d)", before, after)
	}
}
