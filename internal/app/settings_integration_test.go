//go:build windows && integration

package app

// 同包集成测试使用真实配置模型驱动原生控件，覆盖草稿、保存失败与窗口清理。
import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"key-vivi/internal/platform"
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
	var reopen func() error
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
		reopen, fieldHandle = open, item
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
		if err := open(); err != nil {
			return err
		}
		if read(104) != "24" {
			return fmt.Errorf("active config not loaded into controls")
		}
		updateWindow.Call(settings.Handle())
		edit(104, "30")
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
		if settings.Handle() != 0 || active != defaultConfig() {
			return fmt.Errorf("defaults save failed")
		}
		if err := open(); err != nil {
			return err
		}
		if err := checkSettingsScrollPaint(settings.Handle(), item, t); err != nil {
			return err
		}
		edit(104, "32")
		click(2002)
		store.replace = func(string, string) error { return errors.New("replacement denied") }
		before, _ = os.ReadFile(store.path)
		click(1)
		after, _ = os.ReadFile(store.path)
		if settings.Handle() == 0 || active != defaultConfig() || string(before) != string(after) {
			return fmt.Errorf("failed save changed active config or closed draft")
		}
		store.replace = nil
		click(1)
		if settings.Handle() != 0 || active.Appearance.FontSize != 32 || active.Region.Position != "top_right" {
			return fmt.Errorf("save retry failed")
		}
		if err := open(); err != nil {
			return err
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
		send.Call(settings.Handle(), 0x2e0, 144|(144<<16), 0)
		if read(104) != "28" {
			return fmt.Errorf("DPI reflow lost draft")
		}
		click(2)
		for cycle := 0; cycle < 6; cycle++ {
			if err := open(); err != nil {
				return err
			}
			settings.Show()
			settings.Destroy()
			settings.Destroy()
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
		dll.NewProc("SetFocus").Call(item(104))
		phase = -1
		post.Call(item(104), 0x100, 9, 0)
		return loop.Wake()
	}, func(time.Time) bool {
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
		if settings.Handle() == 0 {
			if phase == 0 {
				if active.Appearance.FontSize != 26 {
					ui.Fail(fmt.Errorf("Enter did not save draft"))
					return false
				}
				if err := reopen(); err != nil {
					ui.Fail(err)
					return false
				}
				phase = 1
				post.Call(fieldHandle(104), 0x100, 27, 0)
				return true
			}
			ui.Quit()
			return false
		}
		if time.Since(started) > 15*time.Second {
			ui.Fail(fmt.Errorf("Enter or Escape did not close settings"))
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

// checkSettingsScrollPaint 读取窗口主动绘入DIB的结果，不以桌面截图替代人工可见性验收。
func checkSettingsScrollPaint(hwnd uintptr, item func(int) uintptr, t *testing.T) error {
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
	capture := func(name string) ([]byte, error) {
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
		if got[0] != 255 || got[1] != 255 || got[2] != 255 {
			return nil, fmt.Errorf("settings background was not erased")
		}
		if os.Getenv("KEYVIVI_SETTINGS_PREVIEW") != "" {
			img := image.NewRGBA(image.Rect(0, 0, w, h))
			for i := 0; i < len(got); i += 4 {
				img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = got[i+2], got[i+1], got[i], 255
			}
			path := filepath.Join("..", "..", "tests", "artifacts", name+".png")
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return nil, err
			}
			f, err := os.Create(path)
			if err != nil {
				return nil, err
			}
			err = png.Encode(f, img)
			closeErr := f.Close()
			if err != nil {
				return nil, err
			}
			if closeErr != nil {
				return nil, closeErr
			}
		}
		return got, nil
	}
	first, err := capture("settings-window-top")
	if err != nil {
		return err
	}
	send.Call(hwnd, 0x115, 3, 0)
	middle, err := capture("settings-window-middle")
	if err != nil {
		return err
	}
	if bytes.Equal(first, middle) {
		return fmt.Errorf("scroll did not change settings painting")
	}
	send.Call(hwnd, 0x115, 7, 0)
	if _, err := capture("settings-window-bottom"); err != nil {
		return err
	}
	for cycle := 0; cycle < 6; cycle++ {
		send.Call(hwnd, 0x115, 6, 0)
		send.Call(hwnd, 0x115, 7, 0)
	}
	send.Call(hwnd, 0x115, 6, 0)
	after, err := capture("settings-window-restored")
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
