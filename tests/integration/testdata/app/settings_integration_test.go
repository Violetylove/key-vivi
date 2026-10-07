//go:build windows && integration

package app

// 同包集成测试使用真实配置模型驱动原生控件，覆盖草稿、保存失败与窗口清理。
import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"key-vivi/internal/platform"
	"key-vivi/internal/render"
)

func TestNativeSettingsDraftSaveCancelDefaultsAndDPI(t *testing.T) {
	dll := windows.NewLazyDLL("user32.dll")
	send := dll.NewProc("SendMessageW")
	post := dll.NewProc("PostMessageW")
	setText := dll.NewProc("SetWindowTextW")
	getText := dll.NewProc("GetWindowTextW")
	getItem := dll.NewProc("GetDlgItem")
	isEnabled := dll.NewProc("IsWindowEnabled")
	guiResources := dll.NewProc("GetGuiResources")
	updateWindow := dll.NewProc("UpdateWindow")
	store := &configStore{path: filepath.Join(t.TempDir(), "keyvivi.yaml")}
	active := defaultConfig()
	active.Appearance.FontSize = 24
	if err := store.save(active); err != nil {
		t.Fatal(err)
	}
	var settings *platform.Settings
	var owner uint32
	var previews int
	var ui *platform.Loop
	var fieldHandle func(int) uintptr
	phase := 0
	started := time.Now()
	err := platform.Run(func(loop *platform.Loop) error {
		ui = loop
		owner = windows.GetCurrentThreadId()
		loop.OnCleanup(func() {
			if settings != nil {
				settings.Destroy()
			}
		})
		open := func() error {
			var err error
			settings, err = platform.NewSettings(loop, platform.SettingsOptions{
				Fields: configFields(active), Path: store.path,
				Icon:     func(size int) *image.RGBA { return render.TrayIcon(size, false) },
				Defaults: func() map[string]string { return configValues(defaultConfig()) },
				Preview: func(values map[string]string, scale float64, width, height int) (*image.RGBA, error) {
					if windows.GetCurrentThreadId() != owner {
						return nil, fmt.Errorf("preview ran off UI thread")
					}
					previews++
					c, err := configFromValues(values)
					if err != nil {
						return nil, err
					}
					_, _, ww, wh, _, err := platform.PrimaryWorkArea(loop.Handle())
					if err != nil {
						return nil, err
					}
					return configPreview(c, scale, width, height, ww, wh)
				},
				Save: func(values map[string]string) error {
					if windows.GetCurrentThreadId() != owner {
						return fmt.Errorf("save ran off UI thread")
					}
					c, err := configFromValues(values)
					if err != nil {
						return err
					}
					if err := store.save(c); err != nil {
						return err
					}
					active = c
					return nil
				},
			})
			return err
		}
		item := func(id int) uintptr {
			parent := settings.Handle()
			if id >= 100 {
				parent, _, _ = getItem.Call(parent, 3000)
				parent, _, _ = getItem.Call(parent, 3001)
			}
			h, _, _ := getItem.Call(parent, uintptr(id))
			return h
		}
		fieldHandle = item
		edit := func(id int, text string) {
			p := windows.StringToUTF16Ptr(text)
			setText.Call(item(id), uintptr(unsafe.Pointer(p)))
		}
		read := func(id int) string {
			buffer := make([]uint16, 128)
			getText.Call(item(id), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
			return windows.UTF16ToString(buffer)
		}
		click := func(id int) { send.Call(item(id), 0xf5, 0, 0) }
		choose := func(id, index int) {
			hwnd := item(id)
			send.Call(hwnd, 0x14e, uintptr(index), 0)
			parent, _, _ := dll.NewProc("GetParent").Call(hwnd)
			send.Call(parent, 0x111, uintptr(id)|1<<16, hwnd)
		}
		if err := open(); err != nil {
			return err
		}
		style, _, _ := dll.NewProc("GetWindowLongPtrW").Call(settings.Handle(), ^uintptr(19))
		ownerWindow, _, _ := dll.NewProc("GetWindow").Call(settings.Handle(), 4)
		if style&0x40000 == 0 || style&0x80 != 0 || ownerWindow != 0 {
			return fmt.Errorf("settings has no independent taskbar window: style=%#x owner=%#x", style, ownerWindow)
		}
		windowStyle, _, _ := dll.NewProc("GetWindowLongPtrW").Call(settings.Handle(), ^uintptr(15))
		if windowStyle&0x200000 != 0 {
			return fmt.Errorf("settings exposes a system scrollbar")
		}
		if item(4000) != 0 || item(4001) != 0 || read(6) != "打开配置文件" || item(4) != 0 || read(5) != "" {
			return fmt.Errorf("config opener is not in the footer or config path remains visible")
		}
		count, _, _ := send.Call(item(100), 0x146, 0, 0)
		if count != 6 || item(2000) != 0 {
			return fmt.Errorf("settings does not expose exactly six dropdown positions")
		}
		for i, want := range []string{"左上", "上中", "右上", "左下", "下中", "右下"} {
			var text [64]uint16
			send.Call(item(100), 0x148, uintptr(i), uintptr(unsafe.Pointer(&text[0])))
			if windows.UTF16ToString(text[:]) != want {
				return fmt.Errorf("position dropdown order is incorrect")
			}
		}
		send.Call(item(100), 0x14f, 1, 0)
		if expanded, _, _ := send.Call(item(100), 0x157, 0, 0); expanded == 0 {
			return fmt.Errorf("position dropdown did not expand")
		}
		send.Call(item(100), 0x14f, 0, 0)
		edit(101, "36")
		edit(102, "48")
		for i := range positions {
			choose(100, i)
			if read(101) != "36" || read(102) != "48" {
				return fmt.Errorf("position switch changed margin values")
			}
		}
		if read(104) != "24" {
			return fmt.Errorf("active config not loaded into controls")
		}
		updateWindow.Call(settings.Handle())
		edit(104, "30")
		choose(115, 1)
		for _, id := range []int{105, 106} {
			var class [32]uint16
			dll.NewProc("GetClassNameW").Call(item(id), uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
			if windows.UTF16ToString(class[:]) != "Button" {
				return fmt.Errorf("color setting is not a swatch button")
			}
		}
		if err := driveSettingsColorDialog(settings.Handle(), func() { click(105) }, "#FFFFFF", "#123456", false); err != nil {
			return err
		}
		if read(105) != "#123456" || active.Appearance.TextColor != "#FFFFFF" {
			return fmt.Errorf("color picker did not isolate accepted draft")
		}
		if err := driveSettingsColorDialog(settings.Handle(), func() { click(105) }, "#123456", "#AABBCC", true); err != nil {
			return err
		}
		if read(105) != "#123456" {
			return fmt.Errorf("cancelled color picker changed draft")
		}
		before, _ := os.ReadFile(store.path)
		click(2)
		after, _ := os.ReadFile(store.path)
		if settings.Handle() != 0 || active.Appearance.FontSize != 24 || string(before) != string(after) {
			return fmt.Errorf("cancel applied draft")
		}
		baselineGDI, _, _ := guiResources.Call(^uintptr(0), 0)
		baselineUser, _, _ := guiResources.Call(^uintptr(0), 1)
		if baselineUser == 0 {
			return fmt.Errorf("GetGuiResources returned no resources for live UI loop")
		}
		if err := open(); err != nil {
			return err
		}
		click(3)
		if read(104) != "18" || active.Appearance.FontSize != 24 {
			return fmt.Errorf("defaults persisted before save")
		}
		click(1)
		if settings.Handle() == 0 || active != defaultConfig() || read(5) != "设置已保存" {
			return fmt.Errorf("defaults save failed")
		}
		click(2)
		if err := open(); err != nil {
			return err
		}
		if err := checkSettingsScrollPaint(settings.Handle(), item, "mocha", t); err != nil {
			return err
		}
		choose(115, 1)
		if active.Appearance.SettingsTheme != "mocha" {
			return fmt.Errorf("theme preview changed active config")
		}
		if err := checkSettingsScrollPaint(settings.Handle(), item, "latte", t); err != nil {
			return err
		}
		if err := driveSettingsColorDialog(settings.Handle(), func() { click(106) }, "#14181F", "#112233", false); err != nil {
			return err
		}
		edit(104, "32")
		choose(100, 2)
		click(113)
		store.replace = func(string, string) error { return errors.New("replacement denied") }
		before, _ = os.ReadFile(store.path)
		click(1)
		after, _ = os.ReadFile(store.path)
		if settings.Handle() == 0 || active != defaultConfig() || string(before) != string(after) {
			return fmt.Errorf("failed save changed active config or closed draft")
		}
		store.replace = nil
		click(1)
		if settings.Handle() == 0 || active.Appearance.FontSize != 32 || active.Region.Position != "top_right" || active.Behavior.Animation || active.Appearance.SettingsTheme != "latte" || active.Appearance.BackgroundColor != "#112233" {
			return fmt.Errorf("save retry failed")
		}
		if saved, err := (&configStore{path: store.path}).load(); err != nil || saved != active {
			return fmt.Errorf("save with open panel did not persist config: %v", err)
		}
		edit(104, "200")
		if enabled, _, _ := isEnabled.Call(item(1)); enabled != 0 {
			return fmt.Errorf("invalid draft left save enabled")
		}
		send.Call(settings.Handle(), 0x10, 0, 0)
		if active.Appearance.FontSize != 32 {
			return fmt.Errorf("close applied invalid draft")
		}
		if err := open(); err != nil {
			return err
		}
		edit(104, "28")
		edit(101, "36")
		edit(102, "48")
		choose(100, 5)
		send.Call(settings.Handle(), 0x2e0, 144|(144<<16), 0)
		if read(104) != "28" || read(101) != "36" || read(102) != "48" {
			return fmt.Errorf("DPI reflow lost draft")
		}
		if read(106) != "#112233" {
			return fmt.Errorf("DPI reflow lost chosen color")
		}
		click(1)
		if active.Region.Position != "bottom_right" || active.Region.MarginX != 36 || active.Region.MarginY != 48 || active.Behavior.Animation || active.Appearance.SettingsTheme != "latte" {
			return fmt.Errorf("DPI reflow lost position selection or toggle state")
		}
		if settings.Handle() == 0 {
			return fmt.Errorf("DPI save closed settings")
		}
		click(2)
		// 两种主题先各初始化一次，再比较六次交替重开，排除原生主题缓存的首次增长。
		for cycle := 0; cycle < 8; cycle++ {
			if err := open(); err != nil {
				return err
			}
			choose(115, cycle%2)
			settings.Show()
			settings.Destroy()
			settings.Destroy()
			gdi, _, _ := guiResources.Call(^uintptr(0), 0)
			user, _, _ := guiResources.Call(^uintptr(0), 1)
			if cycle == 1 {
				baselineGDI, baselineUser = gdi, user
			}
			if cycle > 1 && (gdi > baselineGDI+2 || user > baselineUser+2) {
				return fmt.Errorf("settings resource growth in reopen cycle %d: GDI %d->%d USER %d->%d", cycle, baselineGDI, gdi, baselineUser, user)
			}
		}
		gdi, _, _ := guiResources.Call(^uintptr(0), 0)
		user, _, _ := guiResources.Call(^uintptr(0), 1)
		if gdi > baselineGDI+2 || user > baselineUser+2 {
			return fmt.Errorf("settings leaked GUI resources: GDI %d->%d USER %d->%d", baselineGDI, gdi, baselineUser, user)
		}
		t.Logf("settings reopen resources: GDI %d -> %d, USER %d -> %d", baselineGDI, gdi, baselineUser, user)
		if err := open(); err != nil {
			return err
		}
		// 键盘消息经过真实消息泵，跨容器Tab后回车保存，再验证Escape取消。
		edit(104, "26")
		dll.NewProc("SetFocus").Call(item(109))
		phase = -5
		started = time.Now()
		post.Call(item(109), 0x100, 9, 0)
		return loop.Wake()
	}, func(time.Time) bool {
		if phase == -5 {
			focus, _, _ := dll.NewProc("GetFocus").Call()
			if focus != fieldHandle(115) {
				if time.Since(started) > 15*time.Second {
					ui.Fail(fmt.Errorf("Tab did not focus theme dropdown"))
					return false
				}
				return true
			}
			var field, viewport [4]int32
			dll.NewProc("GetWindowRect").Call(focus, uintptr(unsafe.Pointer(&field)))
			parent, _, _ := dll.NewProc("GetDlgItem").Call(settings.Handle(), 3000)
			dll.NewProc("GetWindowRect").Call(parent, uintptr(unsafe.Pointer(&viewport)))
			if field[1] < viewport[1] || field[3] > viewport[3] {
				ui.Fail(fmt.Errorf("focused theme dropdown remained outside viewport"))
				return false
			}
			send.Call(fieldHandle(100), 0x14e, 3, 0)
			dll.NewProc("SetFocus").Call(fieldHandle(100))
			phase = -3
			post.Call(fieldHandle(100), 0x100, 0x28, 0)
			return true
		}
		if phase == -3 {
			if index, _, _ := send.Call(fieldHandle(100), 0x147, 0, 0); index != 4 {
				if time.Since(started) > 15*time.Second {
					ui.Fail(fmt.Errorf("Down did not select next dropdown position"))
					return false
				}
				return true
			}
			phase = -2
			post.Call(fieldHandle(100), 0x100, 0x73, 0)
			return true
		}
		if phase == -2 {
			if expanded, _, _ := send.Call(fieldHandle(100), 0x157, 0, 0); expanded == 0 {
				if time.Since(started) > 15*time.Second {
					ui.Fail(fmt.Errorf("F4 did not open dropdown"))
					return false
				}
				return true
			}
			phase = -4
			post.Call(fieldHandle(100), 0x100, 27, 0)
			return true
		}
		if phase == -4 {
			if settings.Handle() == 0 {
				ui.Fail(fmt.Errorf("Escape closed settings while dropdown was expanded"))
				return false
			}
			if expanded, _, _ := send.Call(fieldHandle(100), 0x157, 0, 0); expanded != 0 {
				return true
			}
			phase = -1
			dll.NewProc("SetFocus").Call(fieldHandle(104))
			post.Call(fieldHandle(104), 0x100, 9, 0)
			return true
		}
		if phase == -1 {
			focus, _, _ := dll.NewProc("GetFocus").Call()
			if focus == fieldHandle(105) {
				phase = 0
				post.Call(focus, 0x100, 13, 0)
			} else if time.Since(started) > 15*time.Second {
				ui.Fail(fmt.Errorf("Tab did not navigate nested settings content"))
				return false
			}
			return true
		}
		if phase == 0 && active.Appearance.FontSize == 26 && active.Region.Position == "bottom_center" {
			if settings.Handle() == 0 {
				ui.Fail(fmt.Errorf("Enter saved but closed settings"))
				return false
			}
			// 保存后继续编辑再取消，不能回滚已保存值或写入新草稿。
			p := windows.StringToUTF16Ptr("29")
			setText.Call(fieldHandle(104), uintptr(unsafe.Pointer(p)))
			phase = 1
			post.Call(fieldHandle(104), 0x100, 27, 0)
			return true
		}
		if settings.Handle() == 0 {
			if phase != 1 || active.Appearance.FontSize != 26 {
				ui.Fail(fmt.Errorf("save/cancel did not retain saved config"))
				return false
			}
			ui.Quit()
			return false
		}
		if time.Since(started) > 15*time.Second {
			ui.Fail(fmt.Errorf("Enter did not save or Escape did not close settings"))
			return false
		}
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if previews < 12 {
		t.Fatal("live preview notifications missing", previews)
	}
	reloaded, err := (&configStore{path: store.path}).load()
	if err != nil || reloaded != active {
		t.Fatal("saved config did not survive reload", err)
	}
}

// 只操作本测试设置窗口拥有的系统选色器，验证真实确认与取消，不发送物理按键。
func driveSettingsColorDialog(owner uintptr, click func(), initial, selected string, cancel bool) error {
	u := windows.NewLazyDLL("user32.dll")
	finished := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		class := windows.StringToUTF16Ptr("#32770")
		for time.Now().Before(deadline) {
			var previous uintptr
			for {
				hwnd, _, _ := u.NewProc("FindWindowExW").Call(0, previous, uintptr(unsafe.Pointer(class)), 0)
				if hwnd == 0 {
					break
				}
				previous = hwnd
				parent, _, _ := u.NewProc("GetWindow").Call(hwnd, 4)
				if parent != owner {
					continue
				}
				ready, _, _ := u.NewProc("GetDlgItem").Call(hwnd, 706)
				if ready == 0 {
					continue
				}
				var failure error
				for i, id := range []uintptr{706, 707, 708} {
					wantInitial, _ := strconv.ParseUint(initial[1+i*2:3+i*2], 16, 8)
					var translated uint32
					got, _, _ := u.NewProc("GetDlgItemInt").Call(hwnd, id, uintptr(unsafe.Pointer(&translated)), 0)
					if translated == 0 || got != uintptr(wantInitial) {
						failure = fmt.Errorf("color dialog did not initialize from swatch")
					}
					value, _ := strconv.ParseUint(selected[1+i*2:3+i*2], 16, 8)
					u.NewProc("SetDlgItemInt").Call(hwnd, id, uintptr(value), 0)
				}
				button := uintptr(1)
				if cancel || failure != nil {
					button = 2
				}
				u.NewProc("SendMessageW").Call(hwnd, 0x111, button, 0)
				finished <- failure
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		finished <- fmt.Errorf("owned native color picker did not appear")
	}()
	click()
	return <-finished
}

// checkSettingsScrollPaint 读取窗口主动绘入DIB的结果，不以桌面截图替代人工可见性验收。
func checkSettingsScrollPaint(hwnd uintptr, item func(int) uintptr, theme string, t *testing.T) error {
	u := windows.NewLazyDLL("user32.dll")
	g := windows.NewLazyDLL("gdi32.dll")
	send := u.NewProc("SendMessageW")
	var rect [4]int32
	getRect := u.NewProc("GetWindowRect")
	getRect.Call(item(1), uintptr(unsafe.Pointer(&rect)))
	footer := rect
	getRect.Call(item(104), uintptr(unsafe.Pointer(&rect)))
	fontTop := rect[1]
	// 控件没有重建，滚动只移动视口内容，底栏位置与草稿必须稳定。
	send.Call(hwnd, 0x20a, uintptr(uint32(0xff88)<<16), 0)
	getRect.Call(item(104), uintptr(unsafe.Pointer(&rect)))
	if rect[1] >= fontTop {
		return fmt.Errorf("mouse wheel did not move settings content")
	}
	getRect.Call(item(1), uintptr(unsafe.Pointer(&rect)))
	if rect != footer {
		return fmt.Errorf("scroll moved fixed settings footer")
	}
	send.Call(hwnd, 0x115, 6, 0)
	u.NewProc("GetClientRect").Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	w, h := int(rect[2]), int(rect[3])
	dc, _, _ := g.NewProc("CreateCompatibleDC").Call(0)
	if dc == 0 {
		return fmt.Errorf("settings paint DC creation failed")
	}
	defer g.NewProc("DeleteDC").Call(dc)
	info := struct {
		Size                   uint32
		Width, Height          int32
		Planes, BitCount       uint16
		Compression, SizeImage uint32
		XPels, YPels           int32
		ClrUsed, ClrImportant  uint32
	}{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	var bits unsafe.Pointer
	bitmap, _, _ := g.NewProc("CreateDIBSection").Call(dc, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 || bits == nil {
		return fmt.Errorf("settings paint DIB creation failed")
	}
	defer g.NewProc("DeleteObject").Call(bitmap)
	previous, _, _ := g.NewProc("SelectObject").Call(dc, bitmap)
	defer g.NewProc("SelectObject").Call(dc, previous)
	pixels := unsafe.Slice((*byte)(bits), w*h*4)
	capture := func() ([]byte, error) {
		for i := range pixels {
			pixels[i] = 0
		}
		if ret, _, _ := u.NewProc("PrintWindow").Call(hwnd, dc, 1); ret == 0 {
			return nil, fmt.Errorf("settings PrintWindow failed")
		}
		g.NewProc("GdiFlush").Call()
		got := append([]byte(nil), pixels...)
		// 普通GDI不维护32位DIB的alpha，比较实际可见RGB并统一无意义的alpha字节。
		for i := 3; i < len(got); i += 4 {
			got[i] = 255
		}
		background := [3]byte{46, 30, 30}
		if theme == "latte" {
			background = [3]byte{245, 241, 239}
		}
		if got[0] != background[0] || got[1] != background[1] || got[2] != background[2] {
			return nil, fmt.Errorf("settings background was not erased")
		}
		return got, nil
	}
	u.NewProc("SetFocus").Call(item(100))
	first, err := capture()
	if err != nil {
		return err
	}
	// 聚焦后的收起项保持输入背景，不能留下原生选中矩形。
	var choice [4]int32
	getRect.Call(item(100), uintptr(unsafe.Pointer(&choice)))
	u.NewProc("MapWindowPoints").Call(0, hwnd, uintptr(unsafe.Pointer(&choice)), 2)
	x, y := int((choice[0]+choice[2])/2), int(choice[1]+4)
	expected := [3]byte{68, 50, 49}
	if theme == "latte" {
		expected = [3]byte{239, 233, 230}
	}
	if x < 0 || x >= w || y < 0 || y >= h {
		return fmt.Errorf("focused position dropdown is outside paint bounds")
	}
	offset := (y*w + x) * 4
	if first[offset] != expected[0] || first[offset+1] != expected[1] || first[offset+2] != expected[2] {
		return fmt.Errorf("collapsed focused dropdown kept selection highlight")
	}
	send.Call(hwnd, 0x115, 3, 0)
	middle, err := capture()
	if err != nil {
		return err
	}
	if bytes.Equal(first, middle) {
		return fmt.Errorf("scroll did not change settings painting")
	}
	send.Call(hwnd, 0x115, 7, 0)
	if _, err := capture(); err != nil {
		return err
	}
	for cycle := 0; cycle < 6; cycle++ {
		send.Call(hwnd, 0x115, 6, 0)
		send.Call(hwnd, 0x115, 7, 0)
	}
	send.Call(hwnd, 0x115, 6, 0)
	after, err := capture()
	if err != nil {
		return err
	}
	if !bytes.Equal(first, after) {
		return fmt.Errorf("settings painting retained stale pixels after scrolling")
	}
	getRect.Call(item(1), uintptr(unsafe.Pointer(&rect)))
	if rect != footer {
		return fmt.Errorf("page scrolling moved fixed footer")
	}
	t.Log("settings wheel/page scrolling repaints deterministically; fixed footer stays in place")
	return nil
}
