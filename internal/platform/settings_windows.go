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
	beginPaint          = user32.NewProc("BeginPaint")
	endPaint            = user32.NewProc("EndPaint")
	redrawWindow        = user32.NewProc("RedrawWindow")
	fillRect            = user32.NewProc("FillRect")
	drawText            = user32.NewProc("DrawTextW")
	getFocus            = user32.NewProc("GetFocus")
	setFocus            = user32.NewProc("SetFocus")
	getControlID        = user32.NewProc("GetDlgCtrlID")
	setTextColor        = gdi32.NewProc("SetTextColor")
	setBkMode           = gdi32.NewProc("SetBkMode")
	getStockObject      = gdi32.NewProc("GetStockObject")
	setDCBrushColor     = gdi32.NewProc("SetDCBrushColor")
	enableWindow        = user32.NewProc("EnableWindow")
	loadCursor          = user32.NewProc("LoadCursorW")
	createFont          = gdi32.NewProc("CreateFontW")
	setDIBitsToDevice   = gdi32.NewProc("SetDIBitsToDevice")
	shellExecute        = windows.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")
)

const (
	settingsPathID     = 4000
	settingsOpenPathID = 4001
)

// SettingField 定义原生设置控件，平台层不依赖应用配置模型。
type SettingField struct {
	Key, Label, Value string
	Description       string
	Group             int
	// 原生下拉框保留键盘导航，绘制由面板主题统一控制。
	Kind    string
	Choices []string
}

// SettingsOptions 使草稿预览与保存共用调用方校验，保存失败不关闭窗口。
type SettingsOptions struct {
	Fields   []SettingField
	Path     string
	Defaults func() map[string]string
	Preview  func(map[string]string, float64, int, int) (*image.RGBA, error)
	Save     func(map[string]string) error
	Icon     func(int) *image.RGBA
}

type settingsControl struct {
	hwnd                uintptr
	x, y, width, height int
	fixed               bool
	edit                bool
	choice              bool
}
type settingsLabel struct {
	text                string
	x, y, width, height int
	heading             bool
	muted               bool
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
	headingFont                                                            uintptr
	viewport, content                                                      uintptr
	viewportHeight, clientWidth, clientHeight, footerTop                   int
	labels                                                                 []settingsLabel
	scale                                                                  float64
	controls                                                               []settingsControl
	inputs                                                                 map[string]uintptr
	palette                                                                settingsPalette
	toggles                                                                map[string]bool
	icons                                                                  [2]uintptr
	status, saveButton                                                     uintptr
	loading                                                                bool
	scroll, virtualHeight, previewX, previewY, previewWidth, previewHeight int
	preview                                                                []byte
	previewInfo                                                            bitmapInfo
	wheelRemainder                                                         int
	customColors                                                           [16]uint32
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
	s := &Settings{loop: loop, options: options, scale: scale, inputs: make(map[string]uintptr), toggles: make(map[string]bool), palette: mochaPalette}
	s.customColors = settingsCustomColors()
	const style = 0x00C00000 | 0x00080000 | 0x00020000 | 0x02000000 // 标题、关闭、最小化与子窗口裁剪；滚动由内容视口接管。
	clientWidth := min(s.px(820), max(1, workWidth-s.px(32)))
	clientHeight := min(s.px(780), max(1, workHeight-s.px(72)))
	box := windowRect{right: int32(clientWidth), bottom: int32(clientHeight)}
	const extendedStyle = 0x50000 // APPWINDOW独立进入任务栏，CONTROLPARENT保留键盘导航。
	if ret, _, err := adjustWindowRect.Call(uintptr(unsafe.Pointer(&box)), style, 0, extendedStyle); ret == 0 {
		return nil, win32Error("AdjustWindowRectEx(settings)", err)
	}
	width, height := min(workWidth, int(box.right-box.left)), min(workHeight, int(box.bottom-box.top))
	title := windows.StringToUTF16Ptr("KeyVivi 设置")
	hwnd, _, err := createWindowEx.Call(extendedStyle, uintptr(unsafe.Pointer(settingsClass)), uintptr(unsafe.Pointer(title)), style,
		uintptr(left+(workWidth-width)/2), uintptr(top+(workHeight-height)/2), uintptr(width), uintptr(height), 0, 0, 0, 0)
	if hwnd == 0 {
		return nil, win32Error("CreateWindowExW(settings)", err)
	}
	s.hwnd = hwnd
	settingsWindows[hwnd] = s
	loop.dialog = hwnd
	s.loading = true
	s.applyWindowStyle()
	if err := s.refreshIcons(); err != nil {
		s.Destroy()
		return nil, err
	}
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
	parent := s.content
	fixed := id < 100
	if fixed {
		parent = s.hwnd
	}
	c, t := windows.StringToUTF16Ptr(class), windows.StringToUTF16Ptr(title)
	hwnd, _, err := createWindowEx.Call(0, uintptr(unsafe.Pointer(c)), uintptr(unsafe.Pointer(t)), style|0x54000000,
		uintptr(s.px(x)), uintptr(s.px(y)), uintptr(s.px(width)), uintptr(s.px(height)), parent, uintptr(id), 0, 0)
	if hwnd == 0 {
		return 0, win32Error("CreateWindowExW(settings control)", err)
	}
	s.controls = append(s.controls, settingsControl{hwnd: hwnd, x: x, y: y, width: width, height: height, fixed: fixed, edit: class == "EDIT", choice: class == "COMBOBOX"})
	if class == "COMBOBOX" {
		if err := s.subclassChoice(hwnd); err != nil {
			return 0, err
		}
	}
	sendMessage.Call(hwnd, 0x30, s.font, 1)
	return hwnd, nil
}

func (s *Settings) build() error {
	fontName := windows.StringToUTF16Ptr("Microsoft YaHei UI")
	s.font, _, _ = createFont.Call(uintptr(-int32(s.px(15))), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(fontName)))
	if s.font == 0 {
		return fmt.Errorf("CreateFontW(settings) failed")
	}
	s.headingFont, _, _ = createFont.Call(uintptr(-int32(s.px(18))), 0, 0, 0, 600, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(fontName)))
	if s.headingFont == 0 {
		return fmt.Errorf("CreateFontW(settings heading) failed")
	}
	var client windowRect
	getClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	logicalWidth := int(float64(client.right) / s.scale)
	// 独立视口裁剪整块内容，避免分组边框与同层控件在滚动重绘时互相覆盖。
	createPane := func(parent uintptr, id int) (uintptr, error) {
		h, _, err := createWindowEx.Call(0x10000, uintptr(unsafe.Pointer(settingsClass)), 0, 0x56000000,
			0, 0, 1, 1, parent, uintptr(id), 0, 0)
		if h == 0 {
			return 0, win32Error("CreateWindowExW(settings pane)", err)
		}
		settingsWindows[h] = s
		return h, nil
	}
	var err error
	s.viewport, err = createPane(s.hwnd, 3000)
	if err != nil {
		return err
	}
	s.content, err = createPane(s.viewport, 3001)
	if err != nil {
		return err
	}
	controlWidth := min(264, max(150, logicalWidth/3))
	controlX := logicalWidth - 28 - controlWidth
	labelWidth := max(60, controlX-52)
	add := func(class, title string, style uintptr, id, x, y, width, height int) (uintptr, error) {
		return s.control(class, title, style, id, x, y, width, height)
	}
	y := 20
	for group, title := range []string{"显示区域", "外观与布局", "行为与动画"} {
		s.labels = append(s.labels, settingsLabel{title, 24, y, logicalWidth - 48, 28, true, false})
		y += 48
		for index, field := range s.options.Fields {
			if field.Group != group {
				continue
			}
			rowHeight := 72
			s.labels = append(s.labels, settingsLabel{field.Label, 36, y + 6, labelWidth, 24, false, false},
				settingsLabel{field.Description, 36, y + 34, labelWidth, 32, false, true})
			var hwnd uintptr
			var err error
			if field.Kind == "choice" || field.Kind == "theme" {
				hwnd, err = add("COMBOBOX", "", 0x213|0x10000|0x200000, 100+index, controlX, y+8, controlWidth, 240)
				if err == nil {
					for _, title := range field.Choices {
						p := windows.StringToUTF16Ptr(title)
						sendMessage.Call(hwnd, 0x143, 0, uintptr(unsafe.Pointer(p)))
					}
					sendMessage.Call(hwnd, 0x153, ^uintptr(0), uintptr(s.px(32)))
					sendMessage.Call(hwnd, 0x153, 0, uintptr(s.px(32)))
				}
			} else if field.Kind == "bool" {
				hwnd, err = add("BUTTON", field.Label, 0xB|0x4000|0x10000, 100+index, controlX+controlWidth-56, y+8, 56, 32)
			} else if field.Kind == "color" {
				hwnd, err = add("BUTTON", "", 0xB|0x4000|0x10000, 100+index, controlX, y+8, controlWidth, 40)
			} else {
				hwnd, err = add("EDIT", "", 0x10000|0x80, 100+index, controlX+12, y+15, controlWidth-24, 24)
			}
			if err != nil {
				return err
			}
			s.inputs[field.Key] = hwnd
			y += rowHeight
		}
		y += 24
	}
	// 配置路径属于设置内容的最后一项，使用只读文本并提供打开按钮。
	s.labels = append(s.labels, settingsLabel{"配置文件", 36, y + 6, labelWidth, 24, false, false})
	pathWidth := max(120, controlX+controlWidth-164)
	if _, err := s.control("STATIC", s.options.Path, 0x8000, settingsPathID, 36, y+38, pathWidth, 32); err != nil {
		return err
	}
	if _, err := s.control("BUTTON", "打开配置文件", 0xB|0x10000, settingsOpenPathID, controlX+controlWidth-116, y+8, 116, 32); err != nil {
		return err
	}
	y += 72
	s.virtualHeight = s.px(y)
	s.status, err = add("STATIC", "", 0, 5, 24, 4, logicalWidth-48, 28)
	if err != nil {
		return err
	}
	s.saveButton, err = add("BUTTON", "保存设置", 0xB|0x10000, 1, logicalWidth-236, 40, 108, 36)
	if err != nil {
		return err
	}
	if _, err := add("BUTTON", "取消", 0xB|0x10000, 2, logicalWidth-116, 40, 92, 36); err != nil {
		return err
	}
	if _, err := add("BUTTON", "恢复默认", 0xB|0x10000, 3, 24, 40, 108, 36); err != nil {
		return err
	}
	s.arrange()
	s.styleChoices()
	return nil
}

func (s *Settings) arrange() {
	var client windowRect
	getClientRect.Call(s.hwnd, uintptr(unsafe.Pointer(&client)))
	s.clientWidth, s.clientHeight = int(client.right), int(client.bottom)
	s.footerTop = max(s.px(190), s.clientHeight-s.px(90))
	s.previewX, s.previewY = 24, 50
	s.previewWidth, s.previewHeight = max(1, int(float64(s.clientWidth)/s.scale)-48), 112
	top := s.px(180)
	s.viewportHeight = max(1, s.footerTop-top-s.px(12))
	moveWindow.Call(s.viewport, 0, uintptr(top), uintptr(s.clientWidth), uintptr(s.viewportHeight), 0)
	for _, c := range s.controls {
		if c.fixed {
			moveWindow.Call(c.hwnd, uintptr(s.px(c.x)), uintptr(s.footerTop+s.px(c.y)), uintptr(s.px(c.width)), uintptr(s.px(c.height)), 0)
		}
	}
	s.setScroll(s.scroll)
}

func settingsText(hwnd uintptr, text string) {
	p := windows.StringToUTF16Ptr(text)
	setWindowText.Call(hwnd, uintptr(unsafe.Pointer(p)))
}

func (s *Settings) fill(values map[string]string) {
	s.loading = true
	for _, field := range s.options.Fields {
		if field.Kind == "choice" || field.Kind == "theme" {
			index, _ := strconv.Atoi(values[field.Key])
			if field.Kind == "theme" {
				index = 0
				if values[field.Key] == "latte" {
					index = 1
				}
			}
			sendMessage.Call(s.inputs[field.Key], 0x14e, uintptr(index), 0)
		} else if field.Kind == "bool" {
			s.toggles[field.Key] = values[field.Key] == "true"
			redrawWindow.Call(s.inputs[field.Key], 0, 0, 0x105)
		} else {
			settingsText(s.inputs[field.Key], values[field.Key])
		}
	}
	s.loading = false
	s.updatePreview()
}

func (s *Settings) values() map[string]string {
	values := make(map[string]string)
	for _, field := range s.options.Fields {
		hwnd := s.inputs[field.Key]
		switch field.Kind {
		case "choice", "theme":
			index, _, _ := sendMessage.Call(hwnd, 0x147, 0, 0)
			values[field.Key] = strconv.Itoa(int(index))
			if field.Kind == "theme" {
				values[field.Key] = "mocha"
				if index == 1 {
					values[field.Key] = "latte"
				}
			}
		case "bool":
			values[field.Key] = strconv.FormatBool(s.toggles[field.Key])
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
	values := s.values()
	s.setTheme(values["settings_theme"])
	img, err := s.options.Preview(values, s.scale, s.px(s.previewWidth), s.px(s.previewHeight))
	if err == nil && (img == nil || img.Bounds().Empty()) {
		err = fmt.Errorf("settings preview is empty")
	}
	valid := uintptr(1)
	if err != nil {
		settingsText(s.status, err.Error())
		valid = 0
		s.preview = nil
	} else {
		settingsText(s.status, "")
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
	redrawWindow.Call(s.hwnd, 0, 0, 0x185)
}

func (s *Settings) setScroll(position int) {
	s.scroll = min(max(0, position), max(0, s.virtualHeight-s.viewportHeight))
	moveWindow.Call(s.content, 0, uintptr(-s.scroll), uintptr(s.clientWidth), uintptr(max(1, s.virtualHeight)), 0)
	// 整块移动后一次擦底并重绘全部子控件，不能复用滚动前的背景像素。
	redrawWindow.Call(s.viewport, 0, 0, 0x185)
}

func (s *Settings) reveal(hwnd uintptr) {
	for _, c := range s.controls {
		if c.hwnd != hwnd || c.fixed {
			continue
		}
		height := c.height
		if c.choice {
			height = 36
		}
		top, bottom := s.px(c.y)-s.px(8), s.px(c.y+height)+s.px(8)
		if top < s.scroll {
			s.setScroll(top)
		} else if bottom > s.scroll+s.viewportHeight {
			s.setScroll(bottom - s.viewportHeight)
		}
		return
	}
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
	focus, _, _ := getFocus.Call()
	focusID := uintptr(0)
	for _, c := range s.controls {
		if c.hwnd == focus {
			focusID, _, _ = getControlID.Call(focus)
		}
	}
	for _, control := range s.controls {
		destroyWindow.Call(control.hwnd)
	}
	if s.viewport != 0 {
		destroyWindow.Call(s.viewport)
		s.viewport, s.content = 0, 0
	}
	s.controls = nil
	s.labels = nil
	clear(s.inputs)
	if s.font != 0 {
		deleteObject.Call(s.font)
		s.font = 0
	}
	if s.headingFont != 0 {
		deleteObject.Call(s.headingFont)
		s.headingFont = 0
	}
	s.scale = scale
	if err := s.refreshIcons(); err != nil {
		return err
	}
	s.scroll = int(math.Round(float64(s.scroll) * ratio))
	if ret, _, err := setWindowPos.Call(s.hwnd, 0, uintptr(x), uintptr(y), uintptr(width), uintptr(height), 0x14); ret == 0 {
		return win32Error("SetWindowPos(settings DPI)", err)
	}
	if err := s.build(); err != nil {
		return err
	}
	s.fill(values)
	if focusID != 0 {
		for _, c := range s.controls {
			id, _, _ := getControlID.Call(c.hwnd)
			if id == focusID {
				setFocus.Call(c.hwnd)
				s.reveal(c.hwnd)
				break
			}
		}
	}
	return nil
}

func settingsFill(dc uintptr, rect windowRect, color uintptr) {
	brush, _, _ := getStockObject.Call(18)
	setDCBrushColor.Call(dc, color)
	fillRect.Call(dc, uintptr(unsafe.Pointer(&rect)), brush)
}

func (s *Settings) text(dc uintptr, text string, x, y, width, height int, heading, muted bool) {
	f := s.font
	if heading {
		f = s.headingFont
	}
	previous, _, _ := selectObject.Call(dc, f)
	defer selectObject.Call(dc, previous)
	color := s.palette.foreground
	if muted {
		color = s.palette.muted
	}
	setTextColor.Call(dc, color)
	setBkMode.Call(dc, 1)
	p := windows.StringToUTF16Ptr(text)
	r := windowRect{left: int32(s.px(x)), top: int32(s.px(y)), right: int32(s.px(x + width)), bottom: int32(s.px(y + height))}
	drawText.Call(dc, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x810)
}

func (s *Settings) paint(hwnd, dc uintptr) {
	var client windowRect
	getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
	settingsFill(dc, client, s.palette.background)
	if hwnd == s.content {
		for _, c := range s.controls {
			if c.edit && !c.fixed {
				border := s.palette.border
				focus, _, _ := getFocus.Call()
				if focus == c.hwnd {
					border = s.palette.accent
				}
				s.rounded(dc, windowRect{left: int32(s.px(c.x - 12)), top: int32(s.px(c.y - 8)), right: int32(s.px(c.x + c.width + 12)), bottom: int32(s.px(c.y + c.height + 8))}, s.palette.input, border, 8)
			}
		}
		for _, label := range s.labels {
			s.text(dc, label.text, label.x, label.y, label.width, label.height, label.heading, label.muted)
			if label.heading {
				settingsFill(dc, windowRect{left: int32(s.px(24)), top: int32(s.px(label.y + 34)), right: int32(s.clientWidth - s.px(24)), bottom: int32(s.px(label.y + 35))}, s.palette.border)
			}
		}
		return
	}
	if hwnd != s.hwnd {
		return
	}
	w := int(float64(s.clientWidth) / s.scale)
	s.text(dc, "实时预览", 24, 14, 120, 26, true, false)
	s.text(dc, "位置示意与示例按键 · 保存后立即生效", 148, 18, w-172, 24, false, true)
	settingsFill(dc, windowRect{left: 0, top: int32(s.footerTop - s.px(8)), right: int32(s.clientWidth), bottom: int32(s.footerTop - s.px(7))}, s.palette.border)
	if len(s.preview) > 0 {
		h := int(-s.previewInfo.header.height)
		setDIBitsToDevice.Call(dc, uintptr(s.px(s.previewX)), uintptr(s.px(s.previewY)), uintptr(s.previewInfo.header.width), uintptr(h), 0, 0, 0, uintptr(h), uintptr(unsafe.Pointer(&s.preview[0])), uintptr(unsafe.Pointer(&s.previewInfo)), 0)
	}
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
		case 0x14:
			var client windowRect
			getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client)))
			settingsFill(wParam, client, s.palette.background)
			return 1
		case 0x133, 0x134, 0x135, 0x138:
			background := s.palette.background
			if message == 0x133 || message == 0x134 {
				background = s.palette.input
			}
			setBkMode.Call(wParam, 2)
			setBkColor.Call(wParam, background)
			setTextColor.Call(wParam, s.palette.foreground)
			brush, _, _ := getStockObject.Call(18)
			setDCBrushColor.Call(wParam, background)
			return brush
		case 0x2b:
			if lParam != 0 {
				// lParam也可能是小整数，不能把所有消息参数声明为Go指针；只复制绘制消息的结构。
				var item settingsDrawItem
				if err := windows.ReadProcessMemory(windows.CurrentProcess(), lParam, (*byte)(unsafe.Pointer(&item)), unsafe.Sizeof(item), nil); err != nil {
					break
				}
				if item.kind == 3 {
					s.drawChoice(&item)
				} else {
					s.drawButton(&item)
				}
				return 1
			}
		case 0x318:
			s.paint(hwnd, wParam)
			return 0
		case 0x400: // IsDialogMessage 查询默认按钮，用于回车保存。
			return 0x534b0001
		case 0x2e0:
			if hwnd != s.hwnd {
				break
			}
			scale := float64(uint16(wParam)) / 96
			if scale > 0 && scale != s.scale {
				if err := s.reflow(scale); err != nil {
					ShowMessage(hwnd, "设置窗口缩放失败", err.Error())
					s.Destroy()
				}
			}
			return 0
		case 0x5:
			if hwnd == s.hwnd && !s.loading && len(s.controls) > 0 && wParam != 1 {
				s.arrange()
				s.updatePreview()
			}
			return 0
		case 0x111:
			if s.loading {
				return 0
			}
			id := int(uint16(wParam))
			notification := uint16(wParam >> 16)
			choice := false
			if id >= 100 && id < 100+len(s.options.Fields) {
				kind := s.options.Fields[id-100].Kind
				choice = kind == "choice" || kind == "theme"
			}
			if notification == 0x100 || (notification == 6 && id >= 100) || (choice && notification == 3) {
				s.reveal(lParam)
				redrawWindow.Call(s.content, 0, 0, 0x85)
				return 0
			}
			if notification == 0x200 || notification == 7 || (choice && notification == 4) {
				redrawWindow.Call(s.content, 0, 0, 0x85)
				return 0
			}
			switch {
			case id == 1:
				if err := s.options.Save(s.values()); err != nil {
					settingsText(s.status, "保存失败："+err.Error())
				} else {
					settingsText(s.status, "设置已保存")
				}
			case id == 2:
				s.Destroy()
			case id == 3:
				s.fill(s.options.Defaults())
			case id == settingsOpenPathID:
				path := windows.StringToUTF16Ptr(s.options.Path)
				verb := windows.StringToUTF16Ptr("open")
				if result, _, _ := shellExecute.Call(s.hwnd, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(path)), 0, 0, 1); result <= 32 {
					settingsText(s.status, "无法打开配置文件，请检查文件关联。")
				}
			case id >= 100 && id < 100+len(s.options.Fields):
				if notification == 0 || notification == 1 || notification == 0x300 {
					field := s.options.Fields[id-100]
					if field.Kind == "bool" && notification == 0 {
						s.toggles[field.Key] = !s.toggles[field.Key]
						redrawWindow.Call(lParam, 0, 0, 0x105)
					}
					if field.Kind == "color" && notification == 0 {
						s.reveal(lParam)
						value, accepted, err := chooseSettingsColor(s.hwnd, s.values()[field.Key], &s.customColors)
						if err != nil {
							settingsText(s.status, "选色器无法打开："+err.Error())
							return 0
						}
						if !accepted {
							return 0
						}
						settingsText(s.inputs[field.Key], value)
					}
					s.updatePreview()
				}
			}
			return 0
		case 0x10:
			if hwnd == s.hwnd {
				s.Destroy()
			}
			return 0
		case 0x82: // 子控件销毁后再释放其字体，并保留默认非客户区清理。
			delete(settingsWindows, hwnd)
			if hwnd != s.hwnd {
				break
			}
			if s.loop.dialog == hwnd {
				s.loop.dialog = 0
			}
			s.hwnd = 0
			if s.font != 0 {
				deleteObject.Call(s.font)
				s.font = 0
			}
			if s.headingFont != 0 {
				deleteObject.Call(s.headingFont)
				s.headingFont = 0
			}
			s.preview = nil
			for i, icon := range s.icons {
				if icon != 0 {
					destroyIcon.Call(icon)
					s.icons[i] = 0
				}
			}
		case 0x115:
			position := s.scroll
			switch uint16(wParam) {
			case 0:
				position -= s.px(40)
			case 1:
				position += s.px(40)
			case 2:
				position -= s.viewportHeight
			case 3:
				position += s.viewportHeight
			case 6:
				position = 0
			case 7:
				position = s.virtualHeight
			}
			s.setScroll(position)
			return 0
		case 0x20a:
			s.wheelRemainder += int(int16(wParam >> 16))
			steps := s.wheelRemainder / 120
			s.wheelRemainder %= 120
			s.setScroll(s.scroll - steps*s.px(64))
			return 0
		case 0xf:
			var paint paintInfo
			dc, _, _ := beginPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))
			if dc != 0 {
				s.paint(hwnd, dc)
			}
			endPaint.Call(hwnd, uintptr(unsafe.Pointer(&paint)))
			return 0
		}
	}
	ret, _, _ := defWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return ret
}
