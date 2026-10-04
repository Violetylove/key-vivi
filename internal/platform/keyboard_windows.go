package platform

import (
	"fmt"
	"key-vivi/internal/keyboard"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
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
var getAsyncKeyState = user32.NewProc("GetAsyncKeyState")

// HeldKeys 返回当前物理按下的键，供暂停和丢事件恢复时阻止残留输入。
func HeldKeys() []uint32 {
	var held []uint32
	for vk := uint32(8); vk < 256; vk++ {
		if vk == 0x10 || vk == 0x11 || vk == 0x12 {
			continue
		}
		if state, _, _ := getAsyncKeyState.Call(uintptr(vk)); state&0x8000 != 0 {
			held = append(held, vk)
		}
	}
	return held
}

// hookEvents 是供 KEYVIVI_DEBUG 计数的回调次数。
var hookEvents atomic.Uint64
var hookDropped atomic.Uint64

// HookEventCount 返回钩子回调见过的事件总数；只给数量，不给按了哪些键。
func HookEventCount() uint64 { return hookEvents.Load() }

// HookDroppedCount 返回缓冲溢出的累计次数，不记录键码或文本。
func HookDroppedCount() uint64 { return hookDropped.Load() }

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
	return StartKeyboardHookWithWake(events, nil)
}

// StartKeyboardHookWithWake 投递带采集时间的事件，随后调用非阻塞的唤醒函数。
func StartKeyboardHookWithWake(events chan<- keyboard.Event, wake func()) (func(), error) {
	if events == nil {
		return nil, fmt.Errorf("keyboard event channel must not be nil")
	}
	stopEvent, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateEvent(keyboard stop): %w", err)
	}
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
		// Go 回调支持指针参数，直接接收系统指针，避免 uintptr 往返破坏指针检查。
		callback := syscall.NewCallback(func(code int32, wParam uintptr, lParam unsafe.Pointer) uintptr {
			hookEvents.Add(1)
			if code >= 0 && (wParam == 0x100 || wParam == 0x104 || wParam == 0x101 || wParam == 0x105) {
				data := (*keyboardData)(lParam)
				select {
				case events <- keyboard.Event{VKCode: data.vk, IsDown: wParam == 0x100 || wParam == 0x104, When: time.Now()}:
				default:
					hookDropped.Add(1)
				}
				if wake != nil {
					wake()
				}
			}
			ret, _, _ := nextHook.Call(0, uintptr(code), wParam, uintptr(lParam))
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
			ret, _, waitErr := msgWaitForMultipleObjects.Call(1, uintptr(unsafe.Pointer(&stopEvent)), 0, 0xffffffff, 0x04ff)
			if ret == 0 {
				return
			}
			if ret != 1 {
				traceHook("wait failed err=%v", waitErr)
				return
			}
			for {
				ret, _, _ := peekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
				if ret == 0 {
					break
				}
				if msg.message == 0x12 {
					return
				}
			}
		}
	}()
	r := <-ready
	if r.err != nil {
		<-finished
		windows.CloseHandle(stopEvent)
		return nil, r.err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			windows.SetEvent(stopEvent)
			<-finished
			windows.CloseHandle(stopEvent)
		})
	}, nil
}
