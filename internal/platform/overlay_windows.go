package platform

import (
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var getWindowLong = user32.NewProc("GetWindowLongW")
var setWindowLong = user32.NewProc("SetWindowLongW")
var setWindowPos = user32.NewProc("SetWindowPos")
var getWindowRect = user32.NewProc("GetWindowRect")
var isWindow = user32.NewProc("IsWindow")
var getDpiForWindow = user32.NewProc("GetDpiForWindow")
var monitorFromWindow = user32.NewProc("MonitorFromWindow")
var getMonitorInfo = user32.NewProc("GetMonitorInfoW")
var clearLastError = windows.NewLazyDLL("kernel32.dll").NewProc("SetLastError")

const (
	styleIndex     = -16
	exStyleIndex   = -20
	overlayBottom  = 80 // Logical pixels above the work-area bottom, scaled by DPI.
	swpNoSize      = 0x0001
	swpNoMove      = 0x0002
	swpNoActivate  = 0x0010
	swpFrameChange = 0x0020
)

type windowRect struct {
	left, top, right, bottom int32
}

type monitorInfo struct {
	cbSize    uint32
	rcMonitor windowRect
	rcWork    windowRect
	dwFlags   uint32
}

func win32Error(operation string, err error) error {
	if err == nil || err == syscall.Errno(0) {
		return fmt.Errorf("%s failed without a Win32 error code", operation)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func validOverlayWindow(hwnd uintptr) error {
	if ret, _, _ := isWindow.Call(hwnd); ret == 0 {
		return fmt.Errorf("invalid overlay HWND %#x", hwnd)
	}
	return nil
}

func windowStyle(hwnd uintptr, index int32, value *uint32) (uint32, error) {
	// Style APIs can succeed with a zero return; clear and inspect LastError
	// on the same OS thread. Styles remain 32-bit even with pointer-sized HWNDs.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	clearLastError.Call(0)
	var ret uintptr
	var err error
	if value == nil {
		ret, _, err = getWindowLong.Call(hwnd, uintptr(index))
	} else {
		ret, _, err = setWindowLong.Call(hwnd, uintptr(index), uintptr(*value))
	}
	if ret == 0 && err != nil && err != syscall.Errno(0) {
		return 0, win32Error("Get/SetWindowLongW", err)
	}
	return uint32(ret), nil
}

// deviceScale returns the window DPI divided by 96, defaulting to 1 on failure.
func deviceScale(hwnd uintptr) float64 {
	dpi, _, _ := getDpiForWindow.Call(hwnd)
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96.0
}

// workArea returns the work area of the monitor nearest the window. Reading it
// from the monitor keeps the result in the same coordinate space as
// GetWindowRect; SystemParametersInfoW reported inconsistently under Fyne's
// per-monitor DPI awareness.
func workArea(hwnd uintptr) (windowRect, error) {
	const monitorDefaultToNearest = 2
	monitor, _, _ := monitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	if monitor == 0 {
		return windowRect{}, fmt.Errorf("MonitorFromWindow returned no monitor")
	}
	info := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ret, _, err := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ret == 0 {
		return windowRect{}, win32Error("GetMonitorInfoW", err)
	}
	return info.rcWork, nil
}

// ConfigureOverlay makes hwnd an undecorated, click-through, non-activating
// toplevel. It deliberately does not move or show the window.
//
// Fyne/GLFW own both visibility and position: a window shown behind their back
// is re-hidden, and Fyne restores its own stored coordinates on every Show, so
// a raw SetWindowPos placement is undone the next time the overlay appears. Use
// OverlayPosition with desktop.Window.RequestPosition instead.
//
// Call on the owning UI thread after the native window exists.
func ConfigureOverlay(hwnd uintptr) error {
	if err := validOverlayWindow(hwnd); err != nil {
		return err
	}
	style, err := windowStyle(hwnd, styleIndex, nil)
	if err != nil {
		return err
	}
	extended, err := windowStyle(hwnd, exStyleIndex, nil)
	if err != nil {
		return err
	}
	style &^= 0x00c00000 | 0x00040000          // WS_CAPTION | WS_THICKFRAME
	extended &^= 0x00040000                    // WS_EX_APPWINDOW
	extended |= 0x80 | 0x08000000 | 0x20 | 0x8 // TOOLWINDOW, NOACTIVATE, TRANSPARENT, TOPMOST
	// WS_EX_LAYERED is deliberately NOT set. A layered window is composited
	// through a redirection surface that GLFW's OpenGL swap does not update, so
	// the window reports itself visible while painting nothing. The subtitle
	// paints its own background, so no per-window alpha is needed.
	if _, err := windowStyle(hwnd, styleIndex, &style); err != nil {
		return err
	}
	if _, err := windowStyle(hwnd, exStyleIndex, &extended); err != nil {
		return err
	}
	if ret, _, err := setWindowPos.Call(hwnd, ^uintptr(0), 0, 0, 0, 0,
		swpNoSize|swpNoMove|swpNoActivate|swpFrameChange); ret == 0 {
		return win32Error("SetWindowPos(apply overlay styles)", err)
	}
	return nil
}

// OverlayPosition returns the bottom-centre position for hwnd on its monitor's
// work area, inset by a DPI-scaled margin.
func OverlayPosition(hwnd uintptr) (int, int, error) {
	if err := validOverlayWindow(hwnd); err != nil {
		return 0, 0, err
	}
	var rect windowRect
	if ret, _, err := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ret == 0 {
		return 0, 0, win32Error("GetWindowRect", err)
	}
	work, err := workArea(hwnd)
	if err != nil {
		return 0, 0, err
	}
	width, height := rect.right-rect.left, rect.bottom-rect.top
	margin := int32(overlayBottom * deviceScale(hwnd))
	x := work.left + (work.right-work.left-width)/2
	y := work.bottom - height - margin
	if x < work.left {
		x = work.left
	}
	if y < work.top {
		y = work.top
	}
	return int(x), int(y), nil
}

// DescribeOverlay reports the geometry the placement maths sees, so a misplaced
// overlay can be diagnosed from a log instead of guessed at from screenshots.
func DescribeOverlay(hwnd uintptr) string {
	if err := validOverlayWindow(hwnd); err != nil {
		return err.Error()
	}
	var rect windowRect
	if ret, _, err := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect))); ret == 0 {
		return win32Error("GetWindowRect", err).Error()
	}
	work, err := workArea(hwnd)
	if err != nil {
		return err.Error()
	}
	dpi, _, _ := getDpiForWindow.Call(hwnd)
	return fmt.Sprintf("rect=%d,%d %dx%d work=%d,%d..%d,%d dpi=%d scale=%.3f",
		rect.left, rect.top, rect.right-rect.left, rect.bottom-rect.top,
		work.left, work.top, work.right, work.bottom, dpi, deviceScale(hwnd))
}
