package platform

import (
	"fmt"
	"syscall"
	"unsafe"
)

var setWindowPos = user32.NewProc("SetWindowPos")
var getWindowRect = user32.NewProc("GetWindowRect")
var isWindow = user32.NewProc("IsWindow")
var getDpiForWindow = user32.NewProc("GetDpiForWindow")
var monitorFromWindow = user32.NewProc("MonitorFromWindow")
var getMonitorInfo = user32.NewProc("GetMonitorInfoW")
var monitorFromPoint = user32.NewProc("MonitorFromPoint")
var setProcessDpiAwarenessContext = user32.NewProc("SetProcessDpiAwarenessContext")

// EnableDPIAwareness 在创建窗口前启用逐显示器 DPI；宿主已设置时保留其上下文。
func EnableDPIAwareness() error {
	if err := setProcessDpiAwarenessContext.Find(); err != nil {
		return err
	}
	if ret, _, err := setProcessDpiAwarenessContext.Call(^uintptr(3)); ret == 0 && err != syscall.Errno(5) {
		return win32Error("SetProcessDpiAwarenessContext", err)
	}
	return nil
}

// PrimaryWorkArea 返回主屏工作区及窗口的 DPI 缩放，供首次位图提交前定位。
func PrimaryWorkArea(hwnd uintptr) (x, y, width, height int, scale float64, err error) {
	monitor, _, _ := monitorFromPoint.Call(0, 1)
	if monitor == 0 {
		err = fmt.Errorf("MonitorFromPoint returned no primary monitor")
		return
	}
	info := monitorInfo{cbSize: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ret, _, callErr := getMonitorInfo.Call(monitor, uintptr(unsafe.Pointer(&info))); ret == 0 {
		err = win32Error("GetMonitorInfoW(primary)", callErr)
		return
	}
	x, y = int(info.rcWork.left), int(info.rcWork.top)
	width, height = int(info.rcWork.right-info.rcWork.left), int(info.rcWork.bottom-info.rcWork.top)
	scale = deviceScale(hwnd)
	return
}

const (
	overlayBottom = 80 // 距工作区底部的逻辑像素，按 DPI 缩放。
	swpNoActivate = 0x0010
	swpShowWindow = 0x0040
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

// deviceScale 返回窗口 DPI 除以 96，取不到时按 1 处理。
func deviceScale(hwnd uintptr) float64 {
	dpi, _, _ := getDpiForWindow.Call(hwnd)
	if dpi == 0 {
		return 1
	}
	return float64(dpi) / 96.0
}

// workArea 返回窗口所在显示器的工作区，与窗口尺寸使用同一 DPI 坐标系。
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
