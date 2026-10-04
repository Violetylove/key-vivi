//go:build windows

package integration_test

import (
	"testing"

	"key-vivi/internal/platform"
)

func TestPlatformRejectsInvalidArguments(t *testing.T) {
	if stop, err := platform.StartKeyboardHook(nil); err == nil || stop != nil {
		t.Fatal("nil keyboard event channel must be rejected")
	}
	if stop, err := platform.StartPauseHotkey(nil); err == nil || stop != nil {
		t.Fatal("nil hotkey callback must be rejected")
	}
	if _, _, err := platform.OverlayPosition(0); err == nil {
		t.Fatal("OverlayPosition accepted zero HWND")
	}
}
