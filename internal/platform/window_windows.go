package platform

import (
	"fmt"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	registerClassEx = user32.NewProc("RegisterClassExW")
	createWindowEx  = user32.NewProc("CreateWindowExW")
	destroyWindow   = user32.NewProc("DestroyWindow")
	defWindowProc   = user32.NewProc("DefWindowProcW")
	translateMsg    = user32.NewProc("TranslateMessage")
	dispatchMsg     = user32.NewProc("DispatchMessageW")
	postQuitMessage = user32.NewProc("PostQuitMessage")
	postMessage     = user32.NewProc("PostMessageW")
	setTimer        = user32.NewProc("SetTimer")
	killTimer       = user32.NewProc("KillTimer")
	getModuleHandle = windows.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
)

const (
	// wmAppWake 由其它协程投递，用于唤醒 UI 线程处理新状态。
	wmAppWake = 0x8000 + 1
	// wmAppTray 是托盘图标的回调消息。
	wmAppTray  = 0x8000 + 2
	timerID    = 1
	tickMillis = 16
)

type wndClassEx struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     uintptr
	hIcon         uintptr
	hCursor       uintptr
	hbrBackground uintptr
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       uintptr
}

type point struct{ x, y int32 }
type msgStruct struct {
	hwnd           uintptr
	message        uint32
	wParam, lParam uintptr
	timestamp      uint32
	pt             point
	private        uint32
}

// Loop 是 UI 线程的消息循环。窗口、定时器与全部显示状态都归它所有。
type Loop struct {
	hwnd   uintptr
	tick   func(time.Time) bool
	timer  atomic.Bool
	dispay atomic.Uint64
}

// 单实例：Win32 的窗口过程是全局回调，无法携带上下文。
var activeLoop atomic.Pointer[Loop]

// onTrayMessage 由托盘注册，收到托盘回调消息时在 UI 线程上被调用。
var onTrayMessage func(lParam uintptr)

var windowClass = windows.StringToUTF16Ptr("KeyViviWindow")

// registerWindowClass 注册供消息窗口与叠加窗口共用的窗口类。
func registerWindowClass() error {
	instance, _, _ := getModuleHandle.Call(0)
	if instance == 0 {
		return fmt.Errorf("GetModuleHandleW failed")
	}
	class := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		lpfnWndProc:   syscall.NewCallback(windowProc),
		hInstance:     instance,
		lpszClassName: windowClass,
	}
	ret, _, err := registerClassEx.Call(uintptr(unsafe.Pointer(&class)))
	if ret == 0 {
		return win32Error("RegisterClassExW", err)
	}
	return nil
}

// Run 在调用线程创建隐藏消息窗口并跑消息循环，直到 Quit。
//
// onReady 在窗口就绪后调用一次，用于创建叠加窗口与托盘图标。
// tick 由 16ms 定时器驱动；返回 false 表示暂停时钟，直到再次 Wake。
func Run(onReady func(*Loop) error, tick func(time.Time) bool) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := registerWindowClass(); err != nil {
		return err
	}
	hwnd, err := createMessageWindow()
	if err != nil {
		return err
	}
	defer destroyWindow.Call(hwnd)

	loop := &Loop{hwnd: hwnd, tick: tick}
	activeLoop.Store(loop)
	defer activeLoop.Store(nil)

	if onReady != nil {
		if err := onReady(loop); err != nil {
			return err
		}
	}
	return loop.pump()
}

// createMessageWindow 创建一个不可见的顶层窗口。
// 不能用 message-only 窗口：它收不到 WM_DISPLAYCHANGE 这类广播。
func createMessageWindow() (uintptr, error) {
	const (
		wsPopup = 0x80000000
	)
	hwnd, _, err := createWindowEx.Call(
		0,
		uintptr(unsafe.Pointer(windowClass)),
		0,
		wsPopup,
		0, 0, 0, 0,
		0, 0, 0, 0,
	)
	if hwnd == 0 {
		return 0, win32Error("CreateWindowExW(message window)", err)
	}
	return hwnd, nil
}

func (l *Loop) pump() error {
	var message msgStruct
	for {
		ret, _, err := getMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
		switch int32(ret) {
		case -1:
			return win32Error("GetMessageW", err)
		case 0:
			return nil
		}
		translateMsg.Call(uintptr(unsafe.Pointer(&message)))
		dispatchMsg.Call(uintptr(unsafe.Pointer(&message)))
	}
}

// Handle 返回消息窗口句柄，供托盘等需要窗口接收回调的组件使用。
func (l *Loop) Handle() uintptr { return l.hwnd }

// Wake 让 UI 线程立即处理一次 tick，并在需要时启动时钟。可从任意协程调用。
func (l *Loop) Wake() {
	postMessage.Call(l.hwnd, wmAppWake, 0, 0)
}

// Quit 结束消息循环。
func (l *Loop) Quit() {
	postMessage.Call(l.hwnd, 0x0012 /* WM_QUIT */, 0, 0)
}

// DisplayGeneration 每次显示器或 DPI 变化时递增，调用方可据此重新定位。
func (l *Loop) DisplayGeneration() uint64 { return l.dispay.Load() }

func (l *Loop) startClock() {
	if l.timer.CompareAndSwap(false, true) {
		setTimer.Call(l.hwnd, timerID, tickMillis, 0)
	}
}

func (l *Loop) stopClock() {
	if l.timer.CompareAndSwap(true, false) {
		killTimer.Call(l.hwnd, timerID)
	}
}

// runTick 执行一次刷新；tick 返回 false 时暂停时钟以保持空闲零占用。
func (l *Loop) runTick() {
	if l.tick == nil {
		return
	}
	if !l.tick(time.Now()) {
		l.stopClock()
	}
}

func windowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	loop := activeLoop.Load()
	switch message {
	case wmAppWake:
		if loop != nil {
			loop.startClock()
			loop.runTick()
		}
		return 0
	case 0x0113: // WM_TIMER
		if loop != nil {
			loop.runTick()
		}
		return 0
	case 0x007E, 0x02E0: // WM_DISPLAYCHANGE, WM_DPICHANGED
		if loop != nil {
			loop.dispay.Add(1)
			loop.startClock()
			loop.runTick()
		}
		return 0
	case wmAppTray:
		if onTrayMessage != nil {
			onTrayMessage(lParam)
		}
		return 0
	}
	ret, _, _ := defWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}
