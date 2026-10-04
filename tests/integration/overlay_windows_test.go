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

func TestOverlayPositionUsesWorkAreaAndDPI(t *testing.T) {
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
	// ShowWindow 返回的是之前的可见状态，不是成功标志。
	u.NewProc("ShowWindow").Call(hwnd, 4) // SW_SHOWNOACTIVATE：显示但不激活
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
	// 几何查询不能改变尺寸、可见性或输入焦点。
	var after rect
	if ret, _, err := u.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&after))); ret == 0 {
		t.Fatalf("GetWindowRect after position query: %v", err)
	}
	if after != bounds {
		t.Fatalf("OverlayPosition changed window bounds: before=%+v after=%+v", bounds, after)
	}
	if shown, _, _ := u.NewProc("IsWindowVisible").Call(hwnd); shown == 0 {
		t.Fatal("OverlayPosition hid a visible window")
	}
	current, _, _ := u.NewProc("GetForegroundWindow").Call()
	if current != foreground {
		t.Fatal("overlay changed the foreground window")
	}
}
