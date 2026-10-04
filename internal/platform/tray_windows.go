package platform

import (
	"fmt"
	"image"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shellNotifyIcon       = windows.NewLazyDLL("shell32.dll").NewProc("Shell_NotifyIconW")
	createIconIndirect    = user32.NewProc("CreateIconIndirect")
	destroyIcon           = user32.NewProc("DestroyIcon")
	createBitmap          = gdi32.NewProc("CreateBitmap")
	createPopupMenu       = user32.NewProc("CreatePopupMenu")
	appendMenu            = user32.NewProc("AppendMenuW")
	destroyMenu           = user32.NewProc("DestroyMenu")
	trackPopupMenu        = user32.NewProc("TrackPopupMenu")
	setForegroundWindow   = user32.NewProc("SetForegroundWindow")
	getCursorPos          = user32.NewProc("GetCursorPos")
	registerWindowMessage = user32.NewProc("RegisterWindowMessageW")
	messageBox            = user32.NewProc("MessageBoxW")
	getSystemMetrics      = user32.NewProc("GetSystemMetrics")
)

type notifyIconData struct {
	size                uint32
	hwnd                uintptr
	id, flags, callback uint32
	icon                uintptr
	tip                 [128]uint16
	state, stateMask    uint32
	info                [256]uint16
	version             uint32
	infoTitle           [64]uint16
	infoFlags           uint32
	guid                windows.GUID
	balloonIcon         uintptr
}

type iconInfo struct {
	icon               uint32
	xHotspot, yHotspot uint32
	mask, color        uintptr
}

// TrayActions 的回调均在 UI 线程执行；Error 用于提示托盘失效并保留退出入口。
type TrayActions struct {
	Toggle, About, Exit func()
	Settings            func()
	Error               func(error)
	// Icon 由调用方按平台要求的像素尺寸提供位图，平台不依赖主题或渲染实现。
	Icon func(size int, paused bool) *image.RGBA
}

// Tray 持有内存图标和原生托盘注册，只能在创建它的 UI 线程操作。
type Tray struct {
	loop           *Loop
	actions        TrayActions
	icons          [2]uintptr
	paused, added  bool
	taskbarMessage uint32
}

// NewTray 添加原生托盘图标，失败时释放已有图标，不写临时文件。
func NewTray(loop *Loop, actions TrayActions) (*Tray, error) {
	if loop == nil || loop.closed.Load() || actions.Toggle == nil || actions.About == nil || actions.Exit == nil || actions.Icon == nil {
		return nil, fmt.Errorf("tray requires a live loop, all menu actions and an icon provider")
	}
	t := &Tray{loop: loop, actions: actions}
	// 直接按系统小图标尺寸绘制，避免固定 32px 再被托盘缩小损失斜线细节。
	metric, _, _ := getSystemMetrics.Call(49)
	size := int(metric)
	if size < 16 {
		size = 16
	}
	for i := range t.icons {
		icon, err := newTrayIcon(actions.Icon(size, i == 1))
		if err != nil {
			t.Destroy()
			return nil, err
		}
		t.icons[i] = icon
	}
	name := windows.StringToUTF16Ptr("TaskbarCreated")
	msg, _, err := registerWindowMessage.Call(uintptr(unsafe.Pointer(name)))
	if msg == 0 {
		t.Destroy()
		return nil, win32Error("RegisterWindowMessageW(TaskbarCreated)", err)
	}
	t.taskbarMessage = uint32(msg)
	if err := t.add(); err != nil {
		t.Destroy()
		return nil, err
	}
	loop.onMessage = t.message
	return t, nil
}

func (t *Tray) data() notifyIconData {
	index := 0
	tip := "KeyVivi — 运行中"
	if t.paused {
		index = 1
		tip = "KeyVivi — 已暂停"
	}
	d := notifyIconData{size: uint32(unsafe.Sizeof(notifyIconData{})), hwnd: t.loop.hwnd, id: 1,
		flags: 1 | 2 | 4 | 0x80, callback: wmAppTray, icon: t.icons[index]}
	copy(d.tip[:], windows.StringToUTF16(tip))
	return d
}

func (t *Tray) notify(command uintptr, data *notifyIconData) error {
	if ret, _, err := shellNotifyIcon.Call(command, uintptr(unsafe.Pointer(data))); ret == 0 {
		return win32Error("Shell_NotifyIconW", err)
	}
	return nil
}

func (t *Tray) add() error {
	d := t.data()
	if err := t.notify(0, &d); err != nil {
		return err
	}
	t.added = true
	d.version = 4
	if err := t.notify(4, &d); err != nil {
		return err
	}
	return nil
}

// SetPaused 同步图标和菜单状态，Explorer 重启后仍保留该状态。
func (t *Tray) SetPaused(paused bool) error {
	t.paused = paused
	var err error
	if !t.added {
		err = t.add()
	} else {
		d := t.data()
		err = t.notify(1, &d)
	}
	if err != nil {
		t.report(err)
	}
	return err
}

func (t *Tray) message(message uint32, wParam, lParam uintptr) bool {
	if message == t.taskbarMessage {
		t.added = false
		if err := t.add(); err != nil {
			t.report(err)
		}
		return true
	}
	if message != wmAppTray {
		return false
	}
	if uint16(lParam>>16) != 1 {
		return true
	}
	switch uint16(lParam) {
	case 0x7b, 0x400, 0x401: // 鼠标菜单、点击或键盘选择。
		if err := t.menu(wParam); err != nil {
			t.report(err)
		}
	}
	return true
}

func (t *Tray) report(err error) {
	if t.actions.Error != nil {
		t.actions.Error(err)
	}
}

func (t *Tray) menu(anchor uintptr) error {
	menu, _, err := createPopupMenu.Call()
	if menu == 0 {
		return win32Error("CreatePopupMenu", err)
	}
	defer destroyMenu.Call(menu)
	label := "暂停"
	if t.paused {
		label = "继续"
	}
	items := []struct {
		title string
		id    uintptr
	}{{label, 1}, {"设置…", 5}, {"关于 KeyVivi", 2}, {"", 3}, {"退出", 4}}
	for _, item := range items {
		if item.id == 5 && t.actions.Settings == nil {
			continue
		}
		title := item.title
		flags := uintptr(0)
		if title == "" {
			flags = 0x800
		}
		text := windows.StringToUTF16Ptr(title)
		if ret, _, err := appendMenu.Call(menu, flags, item.id, uintptr(unsafe.Pointer(text))); ret == 0 {
			return win32Error("AppendMenuW", err)
		}
	}
	x, y := int32(int16(anchor)), int32(int16(anchor>>16))
	if x == -1 && y == -1 {
		var cursor point
		getCursorPos.Call(uintptr(unsafe.Pointer(&cursor)))
		x, y = cursor.x, cursor.y
	}
	// 托盘弹出菜单需前台所有者和 WM_NULL，否则点击外部可能不能关闭菜单。
	setForegroundWindow.Call(t.loop.hwnd)
	command, _, _ := trackPopupMenu.Call(menu, 0x100|0x80|0x2, uintptr(x), uintptr(y), 0, t.loop.hwnd, 0)
	postMessage.Call(t.loop.hwnd, 0, 0, 0)
	switch command {
	case 1:
		t.actions.Toggle()
	case 2:
		t.actions.About()
	case 4:
		t.actions.Exit()
	case 5:
		t.actions.Settings()
	}
	return nil
}

// Destroy 删除托盘并释放图标，可重复调用；必须早于消息窗口销毁。
func (t *Tray) Destroy() {
	if t.added {
		d := t.data()
		t.notify(2, &d)
		t.added = false
	}
	for i, icon := range t.icons {
		if icon != 0 {
			destroyIcon.Call(icon)
			t.icons[i] = 0
		}
	}
	if t.loop != nil {
		t.loop.onMessage = nil
	}
}

func newTrayIcon(img *image.RGBA) (uintptr, error) {
	if img == nil || img.Bounds().Dx() < 1 || img.Bounds().Dx() != img.Bounds().Dy() {
		return 0, fmt.Errorf("tray icon requires a nonempty square bitmap")
	}
	size := img.Bounds().Dx()
	info := bitmapInfo{header: bitmapInfoHeader{size: uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		width: int32(size), height: -int32(size), planes: 1, bitCount: 32}}
	var bits unsafe.Pointer
	bitmap, _, err := createDIBSection.Call(0, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 {
		return 0, win32Error("CreateDIBSection(icon)", err)
	}
	defer deleteObject.Call(bitmap)
	pixels := unsafe.Slice((*byte)(bits), size*size*4)
	for y := 0; y < size; y++ {
		row := img.Pix[y*img.Stride:]
		for x := 0; x < size; x++ {
			i, j := (y*size+x)*4, x*4
			pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = row[j+2], row[j+1], row[j], row[j+3]
		}
	}
	// 单色掩码每行需按 WORD 对齐，24px 等非 16 整倍数尺寸不能紧密拼接。
	maskBits := make([]byte, ((size+15)/16)*2*size)
	mask, _, err := createBitmap.Call(uintptr(size), uintptr(size), 1, 1, uintptr(unsafe.Pointer(&maskBits[0])))
	if mask == 0 {
		return 0, win32Error("CreateBitmap(icon mask)", err)
	}
	defer deleteObject.Call(mask)
	iconData := iconInfo{icon: 1, mask: mask, color: bitmap}
	icon, _, err := createIconIndirect.Call(uintptr(unsafe.Pointer(&iconData)))
	if icon == 0 {
		return 0, win32Error("CreateIconIndirect", err)
	}
	return icon, nil
}

// ShowMessage 显示原生说明窗口，不写入框架设置或缓存。
func ShowMessage(owner uintptr, title, message string) {
	t, m := windows.StringToUTF16Ptr(title), windows.StringToUTF16Ptr(message)
	messageBox.Call(owner, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x40)
}
