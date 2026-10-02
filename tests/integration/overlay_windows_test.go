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
	// 先显示再配置，与 Fyne 一致：隐藏窗口不保留 WS_EX_TOPMOST，
	// 对从未显示的窗口做配置不能代表真实情况。
	// ShowWindow 返回的是之前的可见状态，不是成功标志。
	u.NewProc("ShowWindow").Call(hwnd, 4) // SW_SHOWNOACTIVATE：显示但不激活
	if err := platform.ConfigureOverlay(hwnd); err != nil {
		t.Fatal(err)
	}
	styleIndex, exStyleIndex := int32(-16), int32(-20)
	style, _, _ := u.NewProc("GetWindowLongW").Call(hwnd, uintptr(styleIndex))
	exStyle, _, _ := u.NewProc("GetWindowLongW").Call(hwnd, uintptr(exStyleIndex))
	if style&(0x00c00000|0x00040000) != 0 {
		t.Fatalf("decorations remain: %#x", style)
	}
	// SetWindowLongW 设置的样式是确定的。WS_EX_TOPMOST 由 SetWindowPos 请求，
	// 合成弹窗不保留它，因此只记录不断言；真实 Fyne 窗口实测带该位（0x8000b8）。
	required := uintptr(0x80 | 0x08000000 | 0x20)
	if exStyle&required != required || exStyle&0x00040000 != 0 {
		t.Fatalf("incorrect overlay extended style: %#x", exStyle)
	}
	// 回归保护：WS_EX_LAYERED 会让 GLFW 的 OpenGL 交换不被合成，
	// 窗口自报可见却一个像素都不画。
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
	// ConfigureOverlay 不得移动窗口：位置归 Fyne，它每次 Show 都会恢复自己保存的
	// 坐标，定位必须用 OverlayPosition 的结果走 desktop.Window.RequestPosition。
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
	// ConfigureOverlay 既不能隐藏窗口也不能抢焦点：可见性归 Fyne/GLFW。
	if shown, _, _ := u.NewProc("IsWindowVisible").Call(hwnd); shown == 0 {
		t.Fatal("ConfigureOverlay hid a visible window")
	}
	current, _, _ := u.NewProc("GetForegroundWindow").Call()
	if current != foreground {
		t.Fatal("overlay changed the foreground window")
	}
}
