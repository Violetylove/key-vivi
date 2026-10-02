//go:build windows

package integration_test

import (
	"testing"

	"key-vivi/internal/platform"
)

func TestPlatformRejectsInvalidArguments(t *testing.T) {
	if stop, err := platform.StartPauseHotkey(nil); err == nil || stop != nil {
		t.Fatal("nil hotkey callback must be rejected")
	}
	if err := platform.ConfigureOverlay(0); err == nil {
		t.Fatal("ConfigureOverlay accepted zero HWND")
	}
}
