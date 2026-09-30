package platform

import (
	"fmt"
	"key-vivi/internal/keyboard"
	"runtime"
	"sync"
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
			if code >= 0 && (wParam == 0x100 || wParam == 0x104 || wParam == 0x101 || wParam == 0x105) {
				data := (*keyboardData)(unsafe.Pointer(lParam))
				select {
				case events <- keyboard.Event{data.vk, wParam == 0x100 || wParam == 0x104}:
				default:
				}
			}
			ret, _, _ := nextHook.Call(0, uintptr(code), wParam, lParam)
			return ret
		})
		handle, _, err := setHook.Call(13, callback, 0, 0)
		if handle == 0 {
			ready <- result{err: fmt.Errorf("SetWindowsHookExW: %w", err)}
			return
		}
		defer unhook.Call(handle)
		ready <- result{thread: windows.GetCurrentThreadId()}
		for {
			ret, _, _ := getMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
			if int32(ret) <= 0 {
				return
			}
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
