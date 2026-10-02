//go:build windows && integration

package integration_test

import (
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
	"key-vivi/internal/platform"
)

func TestPauseHotkeyRegistrationCleanup(t *testing.T) {
	stop, err := platform.StartPauseHotkey(func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if duplicate, err := platform.StartPauseHotkey(func() {}); err == nil {
		duplicate()
		t.Fatal("duplicate Ctrl+Alt+K registration succeeded")
	}
	stop()
	stop()
	restarted, err := platform.StartPauseHotkey(func() {})
	if err != nil {
		t.Fatalf("registration was not released: %v", err)
	}
	restarted()
}

func TestNativeOverlayStylesAndPosition(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	u := windows.NewLazyDLL("user32.dll")
	class, err := windows.UTF16PtrFromString("EDIT")
	if err != nil {
		t.Fatal(err)
	}
	hwnd, _, err := u.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class)), 0, 0x80c40000, 0, 0, 320, 100, 0, 0, 0, 0)
	if hwnd == 0 {
		t.Fatalf("CreateWindowExW: %v", err)
	}
	defer u.NewProc("DestroyWindow").Call(hwnd)
	foreground, _, _ := u.NewProc("GetForegroundWindow").Call()
	// Show first, as Fyne does: Windows does not retain WS_EX_TOPMOST for a
	// hidden window, so configuring a never-shown window would not match the app.
	// ShowWindow returns the previous visibility state, not a success flag.
	u.NewProc("ShowWindow").Call(hwnd, 4) // SW_SHOWNOACTIVATE
	if err := platform.ConfigureOverlay(hwnd); err != nil {
		t.Fatal(err)
	}
	styleIndex, exStyleIndex := int32(-16), int32(-20)
	style, _, _ := u.NewProc("GetWindowLongW").Call(hwnd, uintptr(styleIndex))
	exStyle, _, _ := u.NewProc("GetWindowLongW").Call(hwnd, uintptr(exStyleIndex))
	if style&(0x00c00000|0x00040000) != 0 {
		t.Fatalf("decorations remain: %#x", style)
	}
	// Styles applied through SetWindowLongW are deterministic. WS_EX_TOPMOST is
	// requested through SetWindowPos and a synthetic popup does not retain it,
	// so it is reported rather than asserted; the real Fyne window was observed
	// with WS_EX_TOPMOST set (extended style 0x8000b8).
	required := uintptr(0x80 | 0x08000000 | 0x20)
	if exStyle&required != required || exStyle&0x00040000 != 0 {
		t.Fatalf("incorrect overlay extended style: %#x", exStyle)
	}
	// Regression guard: WS_EX_LAYERED stops GLFW's OpenGL swap from being
	// composited, so the window reports itself visible while painting nothing.
	if exStyle&0x00080000 != 0 {
		t.Fatalf("WS_EX_LAYERED must stay clear or the overlay paints nothing: %#x", exStyle)
	}
	if exStyle&0x8 == 0 {
		t.Logf("synthetic window did not retain WS_EX_TOPMOST (%#x); verify Z-order on the real window", exStyle)
	}
	type rect struct{ left, top, right, bottom int32 }
	type monitorInfo struct {
		cbSize            uint32
		rcMonitor, rcWork rect
		dwFlags           uint32
	}
	var bounds rect
	if ret, _, err := u.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&bounds))); ret == 0 {
		t.Fatalf("GetWindowRect: %v", err)
	}
	monitor, _, _ := u.NewProc("MonitorFromWindow").Call(hwnd, 2)
	info := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ret, _, err := u.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info))); ret == 0 {
		t.Fatalf("GetMonitorInfoW: %v", err)
	}
	work := info.rcWork
	dpi, _, _ := u.NewProc("GetDpiForWindow").Call(hwnd)
	if dpi == 0 {
		t.Skip("GetDpiForWindow unavailable; overlay position depends on display DPI")
	}
	margin := int32(80 * float64(dpi) / 96)
	// ConfigureOverlay must not move the window: Fyne owns position and restores
	// its own stored coordinates on every Show, so placement has to go through
	// desktop.Window.RequestPosition using OverlayPosition's result.
	if bounds.left != 0 || bounds.top != 0 {
		t.Fatalf("ConfigureOverlay moved the window: bounds=%+v", bounds)
	}
	x, y, err := platform.OverlayPosition(hwnd)
	if err != nil {
		t.Fatal(err)
	}
	width, height := bounds.right-bounds.left, bounds.bottom-bounds.top
	wantX := int(work.left) + (int(work.right-work.left)-int(width))/2
	wantY := int(work.bottom) - int(height) - int(margin)
	if x != wantX || y != wantY {
		t.Fatalf("OverlayPosition = %d,%d want %d,%d (work=%+v size=%dx%d dpi=%d)", x, y, wantX, wantY, work, width, height, dpi)
	}
	// ConfigureOverlay must neither hide the window nor take focus: Fyne/GLFW own visibility.
	if shown, _, _ := u.NewProc("IsWindowVisible").Call(hwnd); shown == 0 {
		t.Fatal("ConfigureOverlay hid a visible window")
	}
	current, _, _ := u.NewProc("GetForegroundWindow").Call()
	if current != foreground {
		t.Fatal("overlay changed the foreground window")
	}
}
