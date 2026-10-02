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

// hookEvents counts callback invocations for KEYVIVI_DEBUG diagnostics.
var hookEvents atomic.Uint64

// HookEventCount reports how many keyboard events the hook callback has seen.
// It exposes a total only, never which keys were pressed.
func HookEventCount() uint64 { return hookEvents.Load() }

// SendKey injects one press/release pair for a virtual key code. Diagnostic use
// only: injection from this same process still reaches the hook when the desktop
// delivers no other input.
func SendKey(vk uint32) {
	keybdEvent.Call(uintptr(vk), 0, 0, 0)
	keybdEvent.Call(uintptr(vk), 0, 2, 0)
}

// SendTestKey injects F24. It has no display name, so the startup self-check
// never shows up as a keystroke.
func SendTestKey() { SendKey(0x87) }

// traceHook reports hook lifecycle detail when KEYVIVI_DEBUG is set. It records
// handles and message-loop outcomes, never which keys were pressed.
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

// Low-level hooks run on the installing thread and require its message pump.
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
		callback := syscall.NewCallback(func(code int32, wParam, lParam uintptr) uintptr {
			hookEvents.Add(1)
			if code >= 0 && (wParam == 0x100 || wParam == 0x104 || wParam == 0x101 || wParam == 0x105) {
				data := (*keyboardData)(unsafe.Pointer(lParam))
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
