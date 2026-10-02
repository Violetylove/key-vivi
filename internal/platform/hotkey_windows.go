package platform

import (
	"fmt"
	"log"
	"runtime"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var registerHotKey = user32.NewProc("RegisterHotKey")
var unregisterHotKey = user32.NewProc("UnregisterHotKey")
var msgWaitForMultipleObjects = user32.NewProc("MsgWaitForMultipleObjects")

// StartHotkey 的修饰键标志。
const (
	ModAlt      = 0x0001
	ModControl  = 0x0002
	ModShift    = 0x0004
	ModWin      = 0x0008
	ModNoRepeat = 0x4000
)

// StartHotkey 注册全局快捷键。回调在后台协程串行执行，UI 操作请用 fyne.Do 转交。
// stop 可重复调用，会释放注册，也可在回调里调用；已在执行的回调可能在 stop 返回后才结束。
func StartHotkey(modifiers, key uint32, callback func()) (stop func(), err error) {
	if callback == nil {
		return nil, fmt.Errorf("hotkey callback must not be nil")
	}
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, fmt.Errorf("CreateEvent: %w", err)
	}
	ready := make(chan error, 1)
	finished := make(chan struct{})
	notifications := make(chan struct{}, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(finished)
		var msg winMessage
		peekMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 0)
		const id = 1
		ret, _, callErr := registerHotKey.Call(0, id, uintptr(modifiers), uintptr(key))
		if ret == 0 {
			ready <- win32Error(fmt.Sprintf("RegisterHotKey(%#x+%#x)", modifiers, key), callErr)
			return
		}
		defer func() {
			if ret, _, err := unregisterHotKey.Call(0, id); ret == 0 {
				log.Print(win32Error("UnregisterHotKey", err))
			}
		}()
		ready <- nil
		for {
			// 等待事件对象，避免停止请求因消息队列已满而丢失。
			ret, _, err := msgWaitForMultipleObjects.Call(1, uintptr(unsafe.Pointer(&event)), 0, 0xffffffff, 0x04ff)
			runtime.KeepAlive(event)
			if ret == 0 {
				return
			}
			if ret != 1 {
				log.Print(win32Error("MsgWaitForMultipleObjects", err))
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
				if msg.message == 0x0312 && msg.wParam == id {
					select {
					case notifications <- struct{}{}:
					default:
					}
				}
			}
		}
	}()
	if err := <-ready; err != nil {
		<-finished
		windows.CloseHandle(event)
		return nil, err
	}
	go func() {
		for {
			select {
			case <-finished:
				return
			case <-notifications:
				select {
				case <-finished:
					return
				default:
					callback()
				}
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			if err := windows.SetEvent(event); err != nil {
				log.Printf("SetEvent(hotkey stop): %v", err)
				return
			}
			<-finished
			if err := windows.CloseHandle(event); err != nil {
				log.Printf("CloseHandle(hotkey stop): %v", err)
			}
		})
	}, nil
}

// StartPauseHotkey 注册 Ctrl+Alt+K 暂停/继续。
func StartPauseHotkey(callback func()) (stop func(), err error) {
	return StartHotkey(ModControl|ModAlt|ModNoRepeat, 'K', callback)
}
