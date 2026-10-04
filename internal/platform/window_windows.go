package platform

import (
	"fmt"
	"runtime"
	"sync"
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
	getModuleHandle = windows.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW")
)

const (
	// wmAppWake 由其它协程投递，用于唤醒 UI 线程处理新状态。
	wmAppWake = 0x8000 + 1
	// wmAppTray 是托盘图标的回调消息。
	wmAppTray  = 0x8000 + 2
	wmAppQuit  = 0x8000 + 3
	wmAppFrame = 0x8000 + 4
	tickPeriod = 16 * time.Millisecond
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
	hwnd            uintptr
	tick            func(time.Time) bool
	clockStop       chan struct{}
	clockDone       chan struct{}
	clockGeneration uintptr
	framePending    atomic.Bool
	clockError      atomic.Pointer[error]
	display         atomic.Uint64
	pending         atomic.Bool
	closed          atomic.Bool
	quitting        atomic.Bool
	err             error
	cleanup         []func()
	onMessage       func(uint32, uintptr, uintptr) bool
	dialog          uintptr
}

// 单实例：Win32 的窗口过程是全局回调，无法携带上下文。
var activeLoop atomic.Pointer[Loop]

var windowClass = windows.StringToUTF16Ptr("KeyViviWindow")
var classOnce sync.Once
var classError error
var windowCallback = syscall.NewCallback(windowProc)

// registerWindowClass 注册供消息窗口与叠加窗口共用的窗口类。
func registerWindowClass() error {
	classOnce.Do(func() { classError = registerWindowClassOnce() })
	return classError
}

func registerWindowClassOnce() error {
	instance, _, _ := getModuleHandle.Call(0)
	if instance == 0 {
		return fmt.Errorf("GetModuleHandleW failed")
	}
	class := wndClassEx{
		cbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		lpfnWndProc:   windowCallback,
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
// tick 由 16ms 时钟唤醒 UI 线程；返回 false 表示停表，直到再次 Wake。
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
	if !activeLoop.CompareAndSwap(nil, loop) {
		return fmt.Errorf("a UI loop is already running")
	}
	defer func() {
		loop.closed.Store(true)
		loop.stopClock()
		for i := len(loop.cleanup) - 1; i >= 0; i-- {
			loop.cleanup[i]()
		}
		activeLoop.Store(nil)
	}()

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
			if l.err == nil {
				if err := l.clockError.Load(); err != nil {
					return *err
				}
			}
			return l.err
		}
		if l.dialog != 0 {
			if handled, _, _ := isDialogMessage.Call(l.dialog, uintptr(unsafe.Pointer(&message))); handled != 0 {
				continue
			}
		}
		translateMsg.Call(uintptr(unsafe.Pointer(&message)))
		dispatchMsg.Call(uintptr(unsafe.Pointer(&message)))
	}
}

// Handle 返回消息窗口句柄，供托盘等需要窗口接收回调的组件使用。
func (l *Loop) Handle() uintptr { return l.hwnd }

// Wake 让 UI 线程立即处理一次 tick，并在需要时启动时钟。可从任意协程调用。
func (l *Loop) Wake() error {
	if l.closed.Load() || l.quitting.Load() || !l.pending.CompareAndSwap(false, true) {
		return nil
	}
	if ret, _, err := postMessage.Call(l.hwnd, wmAppWake, 0, 0); ret == 0 {
		l.pending.Store(false)
		return win32Error("PostMessageW(wake)", err)
	}
	return nil
}

// Quit 结束消息循环。
func (l *Loop) Quit() {
	if l.closed.Load() || !l.quitting.CompareAndSwap(false, true) {
		return
	}
	if ret, _, _ := postMessage.Call(l.hwnd, wmAppQuit, 0, 0); ret == 0 {
		l.quitting.Store(false)
	}
}

// OnCleanup 在循环退出或初始化失败时，于 UI 线程逆序释放资源。
func (l *Loop) OnCleanup(cleanup func()) { l.cleanup = append(l.cleanup, cleanup) }

// Fail 在 UI 线程记录错误并请求退出，保留正常资源清理路径。
func (l *Loop) Fail(err error) {
	if l.err == nil {
		l.err = err
	}
	l.Quit()
}

// DisplayGeneration 在显示器、DPI 或工作区设置变化时递增，供调用方重新定位。
func (l *Loop) DisplayGeneration() uint64 { return l.display.Load() }

func (l *Loop) startClock() {
	if l.clockStop != nil {
		return
	}
	l.clockGeneration++
	stop := make(chan struct{})
	l.clockStop = stop
	done := make(chan struct{})
	l.clockDone = done
	// WM_TIMER 实测帧间隔抖动明显；Go 运行时使用高精度时钟，协程只投递消息。
	go func(generation uintptr) {
		defer close(done)
		ticker := time.NewTicker(tickPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if !l.framePending.CompareAndSwap(false, true) {
					continue
				}
				if ret, _, err := postMessage.Call(l.hwnd, wmAppFrame, generation, 0); ret == 0 {
					l.framePending.Store(false)
					failure := win32Error("PostMessageW(frame)", err)
					l.clockError.Store(&failure)
					l.Quit()
					return
				}
			}
		}
	}(l.clockGeneration)
}

func (l *Loop) stopClock() {
	if l.clockStop != nil {
		close(l.clockStop)
		// 窗口销毁前等投递协程退出，避免消息打到被复用的 HWND。
		<-l.clockDone
		l.clockStop = nil
		l.clockDone = nil
	}
}

// runTick 在没有待推进状态时停表，避免空闲时持续刷新。
func (l *Loop) runTick() {
	if l.quitting.Load() || l.tick == nil {
		l.stopClock()
		return
	}
	if l.tick(time.Now()) && !l.quitting.Load() {
		l.startClock()
	} else {
		l.stopClock()
	}
}

func windowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	loop := activeLoop.Load()
	switch message {
	case wmAppWake:
		if loop != nil && hwnd == loop.hwnd {
			loop.pending.Store(false)
			loop.runTick()
		}
		return 0
	case wmAppFrame:
		if loop != nil && hwnd == loop.hwnd {
			loop.framePending.Store(false)
			// 已停表或来自上一轮时钟的消息，不能重启空闲动画。
			if loop.clockStop != nil && wParam == loop.clockGeneration {
				loop.runTick()
			}
		}
		return 0
	case 0x007E, 0x02E0, 0x001A: // 显示器、DPI 与工作区设置变化后重新定位。
		if loop != nil {
			loop.display.Add(1)
			loop.Wake()
		}
		return 0
	case wmAppQuit:
		if loop != nil && hwnd == loop.hwnd {
			postQuitMessage.Call(0)
		}
		return 0
	}
	if loop != nil && hwnd == loop.hwnd && loop.onMessage != nil && loop.onMessage(message, wParam, lParam) {
		return 0
	}
	ret, _, _ := defWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}
