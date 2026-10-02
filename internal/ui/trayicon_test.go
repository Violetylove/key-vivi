package ui

import (
	"bytes"
	"image/png"
	"testing"
)

// 图标构造函数未导出，故测试与代码同包；tests/unit 只覆盖导出 API。
func TestTrayIconIsAValidPNG(t *testing.T) {
	for _, paused := range []bool{false, true} {
		resource := trayIcon(paused)
		if resource == nil {
			t.Fatalf("paused=%v: no icon resource", paused)
		}
		image, err := png.Decode(bytes.NewReader(resource.Content()))
		if err != nil {
			t.Fatalf("paused=%v: not a PNG: %v", paused, err)
		}
		if bounds := image.Bounds(); bounds.Dx() != iconSize || bounds.Dy() != iconSize {
			t.Fatalf("paused=%v: size %v, want %dx%d", paused, bounds, iconSize, iconSize)
		}
	}
	if bytes.Equal(trayIcon(false).Content(), trayIcon(true).Content()) {
		t.Fatal("active and paused icons are identical; the tray cannot show state")
	}
}

func TestRoundedSquareCorners(t *testing.T) {
	if insideRounded(0, 0, iconSize, iconSize, iconRound) {
		t.Fatal("corner pixel should be outside the rounded square")
	}
	if !insideRounded(iconSize/2, iconSize/2, iconSize, iconSize, iconRound) {
		t.Fatal("centre pixel should be inside the rounded square")
	}
}
