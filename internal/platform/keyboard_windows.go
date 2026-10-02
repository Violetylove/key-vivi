package platform

import (
	"fmt"
	"key-vivi/internal/keyboard"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var user32 = windows.NewLazyDLL("user32.dll")
var setHook = user32.NewProc("SetWindowsHookExW")
var nextHook = user32.NewProc("CallNextHookEx")
var unhook = user32.NewProc("UnhookWindowsHookEx")
var getMessage = user32.NewProc("GetMessageW")
var peekMessage = user32.NewProc("PeekMessageW")
var postThreadMessage = user32.NewProc("PostThreadMessageW")
var keybdEvent = user32.NewProc("keybd_event")

// hookEvents 是供 KEYVIVI_DEBUG 计数的回调次数。
var hookEvents atomic.Uint64

// HookEventCount 返回钩子回调见过的事件总数；只给数量，不给按了哪些键。
func HookEventCount() uint64 { return hookEvents.Load() }

// SendKey 注入一次按键的按下与抬起。仅用于诊断：同进程注入在桌面不投递任何
// 输入时仍能到达钩子。
func SendKey(vk uint32) {
	keybdEvent.Call(uintptr(vk), 0, 0, 0)
	keybdEvent.Call(uintptr(vk), 0, 2, 0)
}

// SendTestKey 注入 F24；它没有显示名，自检不会表现为一次按键。
func SendTestKey() { SendKey(0x87) }

// traceHook 在 KEYVIVI_DEBUG 设置时输出钩子生命周期；只记句柄与消息循环结果。
func traceHook(format string, args ...any) {
	if os.Getenv("KEYVIVI_DEBUG") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "[keyvivi/hook] "+format+"\n", args...)
}

type keyboardData struct {
	vk, scan, flags, timestamp uint32
	extra                      uintptr
}

type winMessage struct {
	hwnd           uintptr
	message        uint32
	wParam, lParam uintptr
	timestamp      uint32
	x, y           int32
	private        uint32
}

// 低级钩子在安装它的线程上执行，需要该线程自己跑消息循环。
func StartKeyboardHook(events chan<- keyboard.Event) (func(), error) {
	type result struct {
		thread uint32
		err    error
	}
	ready := make(chan result, 1)
	finished := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(finished)
		var msg winMessage
		peekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0)
		// go vet 会报告闭包内的 unsafe.Pointer 转换，但这是 Windows 钩子回调的标准模式：
		// lParam 由系统保证在回调期间有效，转换是安全的
		callback := syscall.NewCallback(func(code int32, wParam, lParam uintptr) uintptr {
			hookEvents.Add(1)
			if code >= 0 && (wParam == 0x100 || wParam == 0x104 || wParam == 0x101 || wParam == 0x105) {
				// lParam 是 KBDLLHOOKSTRUCT 指针，通过 unsafe.Pointer 访问结构
				p := unsafe.Pointer(lParam)
				data := (*keyboardData)(p)
				select {
				case events <- keyboard.Event{VKCode: data.vk, IsDown: wParam == 0x100 || wParam == 0x104}:
				default:
				}
			}
			ret, _, _ := nextHook.Call(0, uintptr(code), wParam, lParam)
			return ret
		})
		handle, _, err := setHook.Call(13, callback, 0, 0)
		if handle == 0 {
			traceHook("install failed err=%v", err)
			ready <- result{err: fmt.Errorf("SetWindowsHookExW: %w", err)}
			return
		}
		defer unhook.Call(handle)
		traceHook("installed handle=%#x thread=%d", handle, windows.GetCurrentThreadId())
		ready <- result{thread: windows.GetCurrentThreadId()}
		for {
			ret, _, getErr := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(ret) <= 0 {
				traceHook("pump exit ret=%d err=%v msg=%#x", int32(ret), getErr, msg.message)
				return
			}
			traceHook("pump message=%#x", msg.message)
		}
	}()
	r := <-ready
	if r.err != nil {
		<-finished
		return nil, r.err
	}
	var once sync.Once
	return func() {
		once.Do(func() { postThreadMessage.Call(uintptr(r.thread), 0x12, 0, 0); <-finished })
	}, nil
}
