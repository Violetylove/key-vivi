package platform

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	sendMessage         = user32.NewProc("SendMessageW")
	setWindowText       = user32.NewProc("SetWindowTextW")
	getWindowText       = user32.NewProc("GetWindowTextW")
	getWindowTextLength = user32.NewProc("GetWindowTextLengthW")
	getClientRect       = user32.NewProc("GetClientRect")
	adjustWindowRect    = user32.NewProc("AdjustWindowRectEx")
	moveWindow          = user32.NewProc("MoveWindow")
	isDialogMessage     = user32.NewProc("IsDialogMessageW")
	invalidateRect      = user32.NewProc("InvalidateRect")
	beginPaint          = user32.NewProc("BeginPaint")
	endPaint            = user32.NewProc("EndPaint")
	setScrollInfo       = user32.NewProc("SetScrollInfo")
	enableWindow        = user32.NewProc("EnableWindow")
	loadCursor          = user32.NewProc("LoadCursorW")
	createFont          = gdi32.NewProc("CreateFontW")
	setDIBitsToDevice   = gdi32.NewProc("SetDIBitsToDevice")
)

// SettingField 定义原生设置控件，平台层不依赖应用配置模型。
type SettingField struct {
	Key, Label, Value string
	Group             int
	// Kind 为空或text时使用文本框；bool为开关，position为九宫格。
	Kind string
}

// SettingsOptions 使草稿预览与保存共用调用方校验，保存失败不关闭窗口。
type SettingsOptions struct {
	Fields   []SettingField
	Path     string
	Defaults func() map[string]string
	Preview  func(map[string]string, float64, int, int) (*image.RGBA, error)
	Save     func(map[string]string) error
}

type settingsControl struct {
	hwnd                uintptr
	x, y, width, height int
}
type scrollInfo struct {
	size, mask uint32
	min, max   int32
	page       uint32
	pos, track int32
}
type paintInfo struct {
	dc              uintptr
	erase           int32
	rect            windowRect
	restore, update int32
	reserved        [32]byte
}

// Settings 持有普通交互窗口与草稿，所有操作均在创建它的UI线程执行。
type Settings struct {
	hwnd                                                                   uintptr
	loop                                                                   *Loop
	options                                                                SettingsOptions
	font                                                                   uintptr
	scale                                                                  float64
	controls                                                               []settingsControl
	inputs                                                                 map[string]uintptr
	positionButtons                                                        [9]uintptr
	position                                                               int
	status, saveButton                                                     uintptr
	loading                                                                bool
	scroll, virtualHeight, previewX, previewY, previewWidth, previewHeight int
	preview                                                                []byte
	previewInfo                                                            bitmapInfo
}

var settingsClass = windows.StringToUTF16Ptr("KeyViviSettings")
var settingsClassOnce sync.Once
var settingsClassError error
var settingsCallback = syscall.NewCallback(settingsProc)

// settingsWindows 只在固定UI线程访问，销毁时移除，避免保留过期窗口。
var settingsWindows = make(map[uintptr]*Settings)

// NewSettings 创建按三个区域分组的设置窗口，初始草稿取调用方活动配置。
func NewSettings(loop *Loop, options SettingsOptions) (*Settings, error) {
	if loop == nil || loop.closed.Load() || options.Defaults == nil || options.Preview == nil || options.Save == nil {
		return nil, fmt.Errorf("settings require a live loop and callbacks")
	}
	settingsClassOnce.Do(func() {
		instance, _, _ := getModuleHandle.Call(0)
		cursor, _, _ := loadCursor.Call(0, 32512)
		class := wndClassEx{cbSize: uint32(unsafe.Sizeof(wndClassEx{})), lpfnWndProc: settingsCallback,
			hInstance: instance, hCursor: cursor, hbrBackground: 6, lpszClassName: settingsClass}
		if ret, _, err := registerClassEx.Call(uintptr(unsafe.Pointer(&class))); ret == 0 {
			settingsClassError = win32Error("RegisterClassExW(settings)", err)
		}
	})
	if settingsClassError != nil {
		return nil, settingsClassError
	}
	left, top, workWidth, workHeight, scale, err := PrimaryWorkArea(loop.hwnd)
	if err != nil {
		return nil, err
	}
	s := &Settings{loop: loop, options: options, scale: scale, inputs: make(map[string]uintptr)}
	const style = 0x00C00000 | 0x00080000 | 0x00020000 | 0x00200000 | 0x02000000 // 标题、关闭、最小化、滚动与子窗口裁剪。
	clientWidth := min(s.px(846), max(1, workWidth-s.px(32)))
	clientHeight := min(s.px(756), max(1, workHeight-s.px(72)))
	box := windowRect{right: int32(clientWidth), bottom: int32(clientHeight)}
	if ret, _, err := adjustWindowRect.Call(uintptr(unsafe.Pointer(&box)), style, 0, 0x10000); ret == 0 {
		return nil, win32Error("AdjustWindowRectEx(settings)", err)
	}
	width, height := min(workWidth, int(box.right-box.left)), min(workHeight, int(box.bottom-box.top))
	title := windows.StringToUTF16Ptr("KeyVivi 设置")
	hwnd, _, err := createWindowEx.Call(0x10000, uintptr(unsafe.Pointer(settingsClass)), uintptr(unsafe.Pointer(title)), style,
		uintptr(left+(workWidth-width)/2), uintptr(top+(workHeight-height)/2), uintptr(width), uintptr(height), loop.hwnd, 0, 0, 0)
	if hwnd == 0 {
		return nil, win32Error("CreateWindowExW(settings)", err)
	}
	s.hwnd = hwnd
	settingsWindows[hwnd] = s
	loop.dialog = hwnd
	if err := s.build(); err != nil {
		s.Destroy()
		return nil, err
	}
	values := make(map[string]string)
	for _, f := range options.Fields {
		values[f.Key] = f.Value
	}
	s.fill(values)
	s.Show()
	return s, nil
}

func (s *Settings) px(value int) int { return int(math.Round(float64(value) * s.scale)) }

func (s *Settings) control(class, title string, style uintptr, id, x, y, width, height int) (uintptr, error) {
	c, t := windows.StringToUTF16Ptr(class), windows.StringToUTF16Ptr(title)
	hwnd, _, err := createWindowEx.Call(0, uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(t)), style|0x50000000,
		uintptr(s.px(x)), uintptr(s.px(y)), uintptr(s.px(width)), uintptr(s.px(height)), s.hwnd, uintptr(id), 0, 0)
	if hwnd == 0 {
		return 0, win32Error("CreateWindowExW(settings control)", err)
	}
	s.controls = append(s.controls, settingsControl{hwnd, x, y, width, height})
	sendMessage.Call(hwnd, 0x30, s.font, 1)
	return hwnd, nil
}

func (s *Settings) build() error {
	fontName := windows.StringToUTF16Ptr("Segoe UI")
	s.font, _, _ = createFont.Call(uintptr(-int32(s.px(14))), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(fontName)))
	if s.font == 0 {
		return fmt.Errorf("CreateFontW(settings) failed")
	}
	var client windowRect
	getClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	logicalWidth := int(float64(client.right) / s.scale)
	columns := 3
	if logicalWidth < 800 {
		columns = 1
	}
	groupWidth := (logicalWidth - 36 - (columns-1)*16) / columns
	add := func(class, title string, style uintptr, id, x, y, width, height int) (uintptr, error) {
		return s.control(class, title, style, id, x, y, width, height)
	}
	if _, err := add("STATIC", "调整后查看预览，保存后立即生效。", 0, 0, 18, 16, logicalWidth-36, 24); err != nil {
		return err
	}
	for group, title := range []string{"显示区域", "外观与布局", "行为与动画"} {
		x, y := 18+(group%columns)*(groupWidth+16), 50+(group/columns)*346
		if _, err := add("BUTTON", title, 7, 0, x, y, groupWidth, 330); err != nil {
			return err
		}
		row := 0
		for index, field := range s.options.Fields {
			if field.Group != group {
				continue
			}
			if field.Kind == "position" {
				for p, text := range []string{"左上", "上中", "右上", "左中", "居中", "右中", "左下", "下中", "右下"} {
					hwnd, err := add("BUTTON", text, 4|0x10000, 2000+p, x+12+(p%3)*((groupWidth-24)/3), y+28+(p/3)*34, (groupWidth-24)/3-4, 28)
					if err != nil {
						return err
					}
					s.positionButtons[p] = hwnd
				}
				row = 3
				continue
			}
			fy := y + 28 + row*46
			var hwnd uintptr
			var err error
			if field.Kind == "bool" {
				hwnd, err = add("BUTTON", field.Label, 3|0x10000, 100+index, x+12, fy, groupWidth-24, 28)
			} else {
				if _, err := add("STATIC", field.Label, 0, 0, x+12, fy+3, groupWidth-24, 20); err != nil {
					return err
				}
				hwnd, err = add("EDIT", "", 0x00800000|0x10000|0x80, 100+index, x+12, fy+22, groupWidth-24, 23)
			}
			if err != nil {
				return err
			}
			s.inputs[field.Key] = hwnd
			row++
		}
	}
	below := 50 + ((3+columns-1)/columns)*346
	if _, err := add("STATIC", "预览：左侧为位置示意，右侧为键帽示例", 0, 0, 18, below, logicalWidth-36, 24); err != nil {
		return err
	}
	s.previewX, s.previewY = 18, below+30
	s.previewWidth, s.previewHeight = max(1, logicalWidth-36), 190
	if _, err := add("STATIC", "配置文件", 0, 0, 18, below+230, 80, 24); err != nil {
		return err
	}
	if _, err := add("EDIT", s.options.Path, 0x00800000|0x10000|0x80|0x800, 0, 100, below+228, logicalWidth-118, 25); err != nil {
		return err
	}
	var err error
	s.status, err = add("STATIC", "", 0, 0, 18, below+265, logicalWidth-36, 44)
	if err != nil {
		return err
	}
	s.saveButton, err = add("BUTTON", "保存", 1|0x10000, 1, logicalWidth-218, below+312, 92, 30)
	if err != nil {
		return err
	}
	if _, err := add("BUTTON", "取消", 0x10000, 2, logicalWidth-110, below+312, 92, 30); err != nil {
		return err
	}
	if _, err := add("BUTTON", "恢复默认", 0x10000, 3, 18, below+312, 108, 30); err != nil {
		return err
	}
	s.virtualHeight = s.px(below + 360)
	s.setScroll(0)
	return nil
}

func settingsText(hwnd uintptr, text string) {
	p := windows.StringToUTF16Ptr(text)
	setWindowText.Call(hwnd, uintptr(unsafe.Pointer(p)))
}

func (s *Settings) fill(values map[string]string) {
	s.loading = true
	for _, field := range s.options.Fields {
		if field.Kind == "position" {
			s.position, _ = strconv.Atoi(values[field.Key])
		} else if field.Kind == "bool" {
			checked := uintptr(0)
			if values[field.Key] == "true" {
				checked = 1
			}
			sendMessage.Call(s.inputs[field.Key], 0xf1, checked, 0)
		} else {
			settingsText(s.inputs[field.Key], values[field.Key])
		}
	}
	for i, hwnd := range s.positionButtons {
		checked := uintptr(0)
		if i == s.position {
			checked = 1
		}
		sendMessage.Call(hwnd, 0xf1, checked, 0)
	}
	s.loading = false
	s.updatePreview()
}

func (s *Settings) values() map[string]string {
	values := make(map[string]string)
	for _, field := range s.options.Fields {
		hwnd := s.inputs[field.Key]
		switch field.Kind {
		case "position":
			values[field.Key] = strconv.Itoa(s.position)
		case "bool":
			checked, _, _ := sendMessage.Call(hwnd, 0xf0, 0, 0)
			values[field.Key] = strconv.FormatBool(checked == 1)
		default:
			length, _, _ := getWindowTextLength.Call(hwnd)
			buffer := make([]uint16, int(length)+1)
			getWindowText.Call(hwnd, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
			values[field.Key] = windows.UTF16ToString(buffer)
		}
	}
	return values
}

func (s *Settings) updatePreview() {
	if s.loading {
		return
	}
	img, err := s.options.Preview(s.values(), s.scale, s.px(s.previewWidth), s.px(s.previewHeight))
	if err == nil && (img == nil || img.Bounds().Empty()) {
		err = fmt.Errorf("settings preview is empty")
	}
	valid := uintptr(1)
	if err != nil {
		settingsText(s.status, err.Error())
		valid = 0
		s.preview = nil
	} else {
		settingsText(s.status, "预览使用示例按键；取消或关闭窗口会放弃未保存的修改。")
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		s.preview = make([]byte, w*h*4)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i, j := y*img.Stride+x*4, (y*w+x)*4
				s.preview[j], s.preview[j+1], s.preview[j+2], s.preview[j+3] = img.Pix[i+2], img.Pix[i+1], img.Pix[i], 255
			}
		}
		s.previewInfo = bitmapInfo{header: bitmapInfoHeader{size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), width: int32(w), height: -int32(h), planes: 1, bitCount: 32}}
	}
	enableWindow.Call(s.saveButton, valid)
	invalidateRect.Call(s.hwnd, 0, 1)
}

func (s *Settings) setScroll(position int) {
	var client windowRect
	getClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	s.scroll = min(max(0, position), max(0, s.virtualHeight-int(client.bottom)))
	info := scrollInfo{size: uint32(unsafe.Sizeof(scrollInfo{})), mask: 7, max: int32(s.virtualHeight - 1), page: uint32(client.bottom), pos: int32(s.scroll)}
	setScrollInfo.Call(s.hwnd, 1, uintptr(unsafe.Pointer(&info)), 1)
	for _, c := range s.controls {
		moveWindow.Call(c.hwnd, uintptr(s.px(c.x)), uintptr(s.px(c.y)-s.scroll), uintptr(s.px(c.width)), uintptr(s.px(c.height)), 1)
	}
	invalidateRect.Call(s.hwnd, 0, 1)
}

func (s *Settings) reflow(scale float64) error {
	values := s.values()
	var bounds windowRect
	if ret, _, err := getWindowRect.Call(s.hwnd, uintptr(unsafe.Pointer(&bounds))); ret == 0 {
		return win32Error("GetWindowRect(settings)", err)
	}
	work, err := workArea(s.hwnd)
	if err != nil {
		return err
	}
	ratio := scale / s.scale
	width := min(int(work.right-work.left), int(math.Round(float64(bounds.right-bounds.left)*ratio)))
	height := min(int(work.bottom-work.top), int(math.Round(float64(bounds.bottom-bounds.top)*ratio)))
	x := max(int(work.left), min(int(bounds.left), int(work.right)-width))
	y := max(int(work.top), min(int(bounds.top), int(work.bottom)-height))
	s.loading = true
	for _, control := range s.controls {
		destroyWindow.Call(control.hwnd)
	}
	s.controls = nil
	clear(s.inputs)
	s.positionButtons = [9]uintptr{}
	if s.font != 0 {
		deleteObject.Call(s.font)
		s.font = 0
	}
	s.scale = scale
	if ret, _, err := setWindowPos.Call(s.hwnd, 0, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0x14); ret == 0 {
		return win32Error("SetWindowPos(settings DPI)", err)
	}
	if err := s.build(); err != nil {
		return err
	}
	s.fill(values)
	return nil
}

// Show 激活已存在的设置窗口，不创建重复草稿。
func (s *Settings) Show() {
	if s.hwnd != 0 {
		showWindow.Call(s.hwnd, 9)
		setForegroundWindow.Call(s.hwnd)
	}
}

// Handle 返回设置窗口句柄，便于原生集成验证。
func (s *Settings) Handle() uintptr { return s.hwnd }

// Destroy 关闭草稿窗口并清理控件、字体与消息路由，可重复调用。
func (s *Settings) Destroy() {
	if s.hwnd != 0 {
		destroyWindow.Call(s.hwnd)
	}
}

func settingsProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	s := settingsWindows[hwnd]
	if s != nil {
		switch message {
		case 0x400: // IsDialogMessage 查询默认按钮，用于回车保存。
			return 0x534b0001
		case 0x2e0:
			scale := float64(uint16(wParam)) / 96
			if scale > 0 && scale != s.scale {
				if err := s.reflow(scale); err != nil {
					ShowMessage(hwnd, "设置窗口缩放失败", err.Error())
					s.Destroy()
				}
			}
			return 0
		case 0x5:
			if !s.loading && len(s.controls) > 0 && wParam != 1 {
				s.setScroll(s.scroll)
			}
			return 0
		case 0x111:
			id := int(uint16(wParam))
			switch {
			case id == 1:
				if err := s.options.Save(s.values()); err != nil {
					settingsText(s.status, "保存失败："+err.Error())
				} else {
					s.Destroy()
				}
			case id == 2:
				s.Destroy()
			case id == 3:
				s.fill(s.options.Defaults())
			case id >= 2000 && id < 2009:
				s.position = id - 2000
				for i, button := range s.positionButtons {
					checked := uintptr(0)
					if i == s.position {
						checked = 1
					}
					sendMessage.Call(button, 0xf1, checked, 0)
				}
				s.updatePreview()
			case id >= 100 && id < 100+len(s.options.Fields):
				if uint16(wParam>>16) == 0 || uint16(wParam>>16) == 0x300 {
					s.updatePreview()
				}
			}
			return 0
		case 0x10:
			s.Destroy()
			return 0
		case 0x82: // 子控件销毁后再释放其字体，并保留默认非客户区清理。
			delete(settingsWindows, hwnd)
			if s.loop.dialog == hwnd {
				s.loop.dialog = 0
			}
			s.hwnd = 0
			if s.font != 0 {
				deleteObject.Call(s.font)
				s.font = 0
			}
			s.preview = nil
		case 0x115:
			position := s.scroll
			switch uint16(wParam) {
			case 0:
				position -= s.px(40)
			case 1:
				position += s.px(40)
			case 2:
				position -= s.px(200)
			case 3:
				position += s.px(200)
			case 4, 5:
				position = int(uint16(wParam >> 16))
			case 6:
				position = 0
			case 7:
				position = s.virtualHeight
			}
			s.setScroll(position)
			return 0
		case 0x20a:
			s.setScroll(s.scroll - int(int16(wParam>>16))*s.px(40)/120)
			return 0
		case 0xf:
			var paint paintInfo
			dc, _, _ := beginPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))
			if dc != 0 && len(s.preview) > 0 {
				h := int(-s.previewInfo.header.height)
				setDIBitsToDevice.Call(dc, uintptr(s.px(s.previewX)), uintptr(s.px(s.previewY)-s.scroll), uintptr(s.previewInfo.header.width), uintptr(h), 0, 0, 0, uintptr(h), uintptr(unsafe.Pointer(&s.preview[0])), uintptr(unsafe.Pointer(&s.previewInfo)), 0)
			}
			endPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))
			return 0
		}
	}
	ret, _, _ := defWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}
