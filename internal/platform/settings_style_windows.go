package platform

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type settingsPalette struct {
	background, input, border, foreground, muted, accent uintptr
	dark                                                 bool
}

// Catppuccin官方色值转为Win32的BGR；面板主题不改写用户的键帽颜色。
var mochaPalette = settingsPalette{0x2E1E1E, 0x443231, 0x5A4745, 0xF4D6CD, 0xC2ADA6, 0xFEBEB4, true}
var lattePalette = settingsPalette{0xF5F1EF, 0xEFE9E6, 0xDAD0CC, 0x694F4C, 0x806C6C, 0xFD8772, false}

var (
	setBkColor           = gdi32.NewProc("SetBkColor")
	setDCPenColor        = gdi32.NewProc("SetDCPenColor")
	roundRect            = gdi32.NewProc("RoundRect")
	dwmWindowAttribute   = windows.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
	setWindowTheme       = windows.NewLazyDLL("uxtheme.dll").NewProc("SetWindowTheme")
	setWindowSubclass    = windows.NewLazyDLL("comctl32.dll").NewProc("SetWindowSubclass")
	defSubclassProc      = windows.NewLazyDLL("comctl32.dll").NewProc("DefSubclassProc")
	removeWindowSubclass = windows.NewLazyDLL("comctl32.dll").NewProc("RemoveWindowSubclass")
	moveTo               = gdi32.NewProc("MoveToEx")
	lineTo               = gdi32.NewProc("LineTo")
)

var settingsChoiceCallback uintptr

func init() { settingsChoiceCallback = syscall.NewCallback(settingsChoiceProc) }

func (s *Settings) refreshIcons() error {
	if s.options.Icon == nil {
		return nil
	}
	for i, size := range []int{16, 32} {
		icon, err := newTrayIcon(s.options.Icon(s.px(size)))
		if err != nil {
			return err
		}
		sendMessage.Call(s.hwnd, 0x80, uintptr(i), icon)
		if s.icons[i] != 0 {
			destroyIcon.Call(s.icons[i])
		}
		s.icons[i] = icon
	}
	return nil
}

func (s *Settings) subclassChoice(hwnd uintptr) error {
	if ok, _, err := setWindowSubclass.Call(hwnd, settingsChoiceCallback, 1, 0); ok == 0 {
		return fmt.Errorf("SetWindowSubclass(settings choice): %w", err)
	}
	settingsWindows[hwnd] = s
	return nil
}

// 原生下拉框仍处理展开、焦点和键盘，收起后的边框与箭头由面板统一绘制。
func settingsChoiceProc(hwnd uintptr, message uint32, wParam, data, id, ref uintptr) uintptr {
	s := settingsWindows[hwnd]
	if message == 0x82 {
		delete(settingsWindows, hwnd)
		removeWindowSubclass.Call(hwnd, settingsChoiceCallback, 1)
	}
	ret, _, _ := defSubclassProc.Call(hwnd, uintptr(message), wParam, data)
	if s == nil {
		return ret
	}
	switch message {
	case 0xf:
		dc, _, _ := getDC.Call(hwnd)
		if dc != 0 {
			s.paintChoice(hwnd, dc)
			releaseDC.Call(hwnd, dc)
		}
	case 0x317, 0x318:
		if wParam != 0 {
			s.paintChoice(hwnd, wParam)
		}
	}
	return ret
}

func (s *Settings) paintChoice(hwnd, dc uintptr) {
	var r windowRect
	getClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
	settingsFill(dc, r, s.palette.background)
	border := s.palette.border
	focus, _, _ := getFocus.Call()
	if focus == hwnd {
		border = s.palette.accent
	}
	r.right--
	r.bottom--
	s.rounded(dc, r, s.palette.input, border, 8)
	previousFont, _, _ := selectObject.Call(dc, s.font)
	setTextColor.Call(dc, s.palette.foreground)
	setBkMode.Call(dc, 1)
	var label [128]uint16
	getWindowText.Call(hwnd, uintptr(unsafe.Pointer(&label[0])), uintptr(len(label)))
	text := r
	text.left += int32(s.px(12))
	text.right -= int32(s.px(32))
	drawText.Call(dc, uintptr(unsafe.Pointer(&label[0])), ^uintptr(0), uintptr(unsafe.Pointer(&text)), 0x8824)
	selectObject.Call(dc, previousFont)
	pen, _, _ := getStockObject.Call(19)
	previousPen, _, _ := selectObject.Call(dc, pen)
	setDCPenColor.Call(dc, s.palette.muted)
	x, y := r.right-int32(s.px(18)), (r.top+r.bottom)/2
	moveTo.Call(dc, uintptr(x-int32(s.px(4))), uintptr(y-int32(s.px(2))), 0)
	lineTo.Call(dc, uintptr(x), uintptr(y+int32(s.px(2))))
	lineTo.Call(dc, uintptr(x+int32(s.px(4))), uintptr(y-int32(s.px(2))))
	selectObject.Call(dc, previousPen)
}

type settingsDrawItem struct {
	kind, id, item, action, state uint32
	hwnd, dc                      uintptr
	rect                          windowRect
	data                          uintptr
}

func (s *Settings) applyWindowStyle() {
	if dwmWindowAttribute.Find() != nil {
		return
	}
	// 较旧Windows不支持的新属性可失败，不能影响原生控件与设置保存。
	dark := uint32(0)
	if s.palette.dark {
		dark = 1
	}
	for _, attr := range []struct{ id, value uint32 }{{20, dark}, {33, 2}, {35, uint32(s.palette.background)}, {36, uint32(s.palette.foreground)}} {
		dwmWindowAttribute.Call(s.hwnd, uintptr(attr.id), uintptr(unsafe.Pointer(&attr.value)), 4)
	}
}

func (s *Settings) setTheme(name string) {
	palette := mochaPalette
	if name == "latte" {
		palette = lattePalette
	}
	if s.palette == palette {
		return
	}
	s.palette = palette
	s.applyWindowStyle()
	s.styleChoices()
}

func (s *Settings) styleChoices() {
	if setWindowTheme.Find() != nil {
		return
	}
	theme := windows.StringToUTF16Ptr("Explorer")
	if s.palette.dark {
		theme = windows.StringToUTF16Ptr("DarkMode_CFD")
	}
	for _, c := range s.controls {
		if c.choice {
			setWindowTheme.Call(c.hwnd, uintptr(unsafe.Pointer(theme)), 0)
		}
	}
}

func (s *Settings) rounded(dc uintptr, r windowRect, fill, stroke uintptr, radius int) {
	brush, _, _ := getStockObject.Call(18)
	pen, _, _ := getStockObject.Call(19)
	previousBrush, _, _ := selectObject.Call(dc, brush)
	previousPen, _, _ := selectObject.Call(dc, pen)
	setDCBrushColor.Call(dc, fill)
	setDCPenColor.Call(dc, stroke)
	roundRect.Call(dc, uintptr(r.left), uintptr(r.top), uintptr(r.right), uintptr(r.bottom), uintptr(s.px(radius*2)), uintptr(s.px(radius*2)))
	selectObject.Call(dc, previousPen)
	selectObject.Call(dc, previousBrush)
}

func (s *Settings) drawButton(item *settingsDrawItem) {
	if item.kind != 4 {
		return
	}
	r := item.rect
	settingsFill(item.dc, r, s.palette.background)
	fill, stroke, text := s.palette.input, s.palette.border, s.palette.foreground
	selected := item.id == 1
	if selected {
		fill, stroke, text = s.palette.accent, s.palette.accent, s.palette.background
	}
	if item.state&4 != 0 {
		fill, stroke, text = s.palette.input, s.palette.border, s.palette.muted
	} else if item.state&1 != 0 {
		stroke = s.palette.accent
		if !selected {
			fill = s.palette.border
		}
	}
	if item.id >= 100 && int(item.id-100) < len(s.options.Fields) {
		field := s.options.Fields[item.id-100]
		if field.Kind == "color" {
			if item.state&0x10 != 0 {
				stroke = s.palette.accent
			}
			s.rounded(item.dc, r, fill, stroke, 8)
			var label [32]uint16
			getWindowText.Call(item.hwnd, uintptr(unsafe.Pointer(&label[0])), uintptr(len(label)))
			value := windows.UTF16ToString(label[:])
			color, err := settingsColorRef(value)
			if err == nil {
				swatch := windowRect{left: r.left + int32(s.px(12)), top: r.top + int32(s.px(8)), right: r.left + int32(s.px(36)), bottom: r.bottom - int32(s.px(8))}
				s.rounded(item.dc, swatch, uintptr(color), s.palette.border, 5)
			}
			previous, _, _ := selectObject.Call(item.dc, s.font)
			defer selectObject.Call(item.dc, previous)
			setTextColor.Call(item.dc, text)
			setBkMode.Call(item.dc, 1)
			textRect := r
			textRect.left += int32(s.px(48))
			textRect.right -= int32(s.px(12))
			p := windows.StringToUTF16Ptr(value + "   选择颜色")
			drawText.Call(item.dc, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&textRect)), 0x8824)
			return
		}
		if field.Kind == "bool" {
			enabled := s.toggles[field.Key]
			track := r
			track.left += int32(s.px(2))
			track.right -= int32(s.px(2))
			track.top += int32(s.px(3))
			track.bottom -= int32(s.px(3))
			fill = s.palette.border
			if enabled {
				fill = s.palette.accent
			}
			stroke = fill
			if item.state&0x10 != 0 {
				stroke = s.palette.foreground
			}
			s.rounded(item.dc, track, fill, stroke, 13)
			knob := windowRect{left: track.left + int32(s.px(3)), top: track.top + int32(s.px(3)), right: track.left + int32(s.px(23)), bottom: track.bottom - int32(s.px(3))}
			if enabled {
				knob.left, knob.right = track.right-int32(s.px(23)), track.right-int32(s.px(3))
			}
			s.rounded(item.dc, knob, s.palette.foreground, s.palette.foreground, 10)
			return
		}
	}
	if item.state&0x10 != 0 {
		stroke = s.palette.foreground
	}
	r.left++
	r.top++
	r.right--
	r.bottom--
	s.rounded(item.dc, r, fill, stroke, 8)
	previous, _, _ := selectObject.Call(item.dc, s.font)
	setTextColor.Call(item.dc, text)
	setBkMode.Call(item.dc, 1)
	var label [128]uint16
	getWindowText.Call(item.hwnd, uintptr(unsafe.Pointer(&label[0])), uintptr(len(label)))
	drawText.Call(item.dc, uintptr(unsafe.Pointer(&label[0])), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x825)
	selectObject.Call(item.dc, previous)
}

func (s *Settings) drawChoice(item *settingsDrawItem) {
	// 收起项也会收到选中绘制消息，必须整框重绘，避免原生高亮覆盖圆角与箭头。
	if item.state&0x1000 != 0 {
		s.paintChoice(item.hwnd, item.dc)
		return
	}
	fill, text := s.palette.input, s.palette.foreground
	if item.state&1 != 0 {
		fill, text = s.palette.accent, s.palette.background
	}
	settingsFill(item.dc, item.rect, fill)
	if item.item == ^uint32(0) || item.id < 100 || int(item.id-100) >= len(s.options.Fields) {
		return
	}
	field := s.options.Fields[item.id-100]
	if int(item.item) >= len(field.Choices) {
		return
	}
	previous, _, _ := selectObject.Call(item.dc, s.font)
	defer selectObject.Call(item.dc, previous)
	setTextColor.Call(item.dc, text)
	setBkMode.Call(item.dc, 1)
	r := item.rect
	r.left += int32(s.px(10))
	p := windows.StringToUTF16Ptr(field.Choices[item.item])
	drawText.Call(item.dc, uintptr(unsafe.Pointer(p)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x8824)
}
