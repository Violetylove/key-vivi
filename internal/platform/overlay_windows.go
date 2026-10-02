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
	overlayBottom  = 80 // 距工作区底部的逻辑像素，按 DPI 缩放。
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
	// 样式 API 成功时也可能返回 0，需在同一线程清空后读 LastError。
	// HWND 是指针宽度，但样式始终是 32 位。
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

// deviceScale 返回窗口 DPI 除以 96，取不到时按 1 处理。
func deviceScale(hwnd uintptr) float64 {
	dpi, _, _ := getDpiForWindow.Call(hwnd)
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96.0
}

// workArea 返回窗口所在显示器的工作区。取显示器信息才能与 GetWindowRect
// 处于同一坐标系；SystemParametersInfoW 在 Fyne 的逐显示器 DPI 下返回值不一致。
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

// ConfigureOverlay 把窗口设为无边框、点击穿透、不激活的顶层窗口，不移动也不显示它。
//
// 可见性和位置都归 Fyne/GLFW：绕过它们显示的窗口会被重新隐藏，而 Fyne 每次 Show
// 都会恢复自己保存的坐标，直接 SetWindowPos 定位下次就会被撤销。定位请用
// OverlayPosition 配合 desktop.Window.RequestPosition。
//
// 需在原生窗口创建后、于 UI 线程调用。
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
	extended |= 0x80 | 0x08000000 | 0x20 | 0x8 // TOOLWINDOW、NOACTIVATE、TRANSPARENT、TOPMOST
	// 刻意不设 WS_EX_LAYERED：分层窗口走重定向表面合成，GLFW 的 OpenGL 交换不更新它，
	// 结果是窗口自报可见却一个像素都不画。字幕自带背景，不需要整窗 alpha。
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

// OverlayPosition 返回窗口在所在显示器工作区内的底部居中坐标，已按 DPI 缩放边距。
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

// DescribeOverlay 输出定位计算用到的几何量，便于从日志排查位置异常，不必靠截图猜。
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
