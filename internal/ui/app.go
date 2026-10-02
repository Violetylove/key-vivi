package ui

import (
	"fmt"
	"image/color"
	"os"
	"strconv"
	"strings"
	"time"

	"key-vivi/internal/display"
	"key-vivi/internal/keyboard"
	"key-vivi/internal/platform"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// debugf writes troubleshooting detail to stderr when KEYVIVI_DEBUG is set.
// Window placement and visibility depend on the desktop, so field reports need
// evidence rather than guesses. It never logs key content.
func debugf(format string, args ...any) {
	if os.Getenv("KEYVIVI_DEBUG") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "[keyvivi] "+format+"\n", args...)
}

// readyHintDuration returns how long the startup hint stays on screen.
// KEYVIVI_READY_HINT overrides it in seconds; 0 disables the hint, which users
// who record their screen may prefer.
func readyHintDuration() time.Duration {
	const fallback = 1500 * time.Millisecond
	raw := os.Getenv("KEYVIVI_READY_HINT")
	if raw == "" {
		return fallback
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || seconds < 0 {
		return fallback
	}
	return time.Duration(seconds * float64(time.Second))
}

// Logical overlay size. The window is fixed-size and Fyne sizes a fixed window
// to its content, so the content must declare this or the overlay collapses.
const (
	overlayWidth  = 1100
	overlayHeight = 64
)

func Run() {
	a := app.NewWithID("keyvivi")
	configureTheme(a)
	w := a.NewWindow("KeyVivi")
	w.SetFixedSize(true)
	text := canvas.NewText("", color.White)
	text.TextSize = 18
	text.Alignment = fyne.TextAlignCenter
	bg := canvas.NewRectangle(color.RGBA{15, 19, 28, 255})
	// A fixed-size window is sized to its content's minimum, so an empty label
	// collapses it to a thumbnail and the centring maths then places that
	// thumbnail against the right edge. The background carries the real size.
	bg.SetMinSize(fyne.NewSize(overlayWidth, overlayHeight))
	w.SetContent(container.NewStack(bg, container.NewCenter(text)))
	w.Resize(fyne.NewSize(overlayWidth, overlayHeight))
	w.SetCloseIntercept(a.Quit)
	controller := display.NewController()
	// The overlay flashes a ready hint so "running but no input yet" is
	// distinguishable from "not running at all". A restricted token is reported
	// here rather than in a dialog: it explains why keystrokes only appear while
	// this window has focus, without interrupting every launch.
	readyHint := "KeyVivi 已启动"
	if restricted, level, err := platform.RestrictedIntegrity(); err != nil {
		debugf("integrity check failed: %v", err)
	} else {
		debugf("integrity level=%#x restricted=%v", level, restricted)
		if restricted {
			readyHint = fmt.Sprintf("KeyVivi 已启动（受限环境 %#x，仅本窗口有效）", level)
		}
		// The title carries the level in debug mode: the overlay drops its
		// caption, so this is what an external probe can read back.
		if os.Getenv("KEYVIVI_DEBUG") != "" {
			w.SetTitle(fmt.Sprintf("KeyVivi %#x", level))
		}
	}
	readyUntil := time.Time{}
	var hwnd uintptr
	visible := false
	// placeOverlay asks Fyne to put the window at the bottom centre of its
	// monitor. It must go through Fyne's RequestPosition: Fyne stores whatever
	// coordinates it is given and restores them on every Show, so a raw
	// SetWindowPos placement is reverted the next time the overlay appears.
	placeOverlay := func() {
		if hwnd == 0 {
			return
		}
		x, y, err := platform.OverlayPosition(hwnd)
		if err != nil {
			debugf("overlay position failed: %v", err)
			return
		}
		desktopWindow, ok := w.(desktop.Window)
		if !ok {
			debugf("window does not implement desktop.Window; position not applied")
			return
		}
		desktopWindow.RequestPosition(x, y)
		debugf("overlay geometry: %s", platform.DescribeOverlay(hwnd))
		debugf("overlay position -> %d,%d", x, y)
	}
	render := func() {
		entries := controller.Queue.Entries()
		parts := make([]string, len(entries))
		for i, e := range entries {
			parts[i] = e.Text
		}
		value := strings.Join(parts, "    |    ")
		if value == "" && time.Now().Before(readyUntil) {
			value = readyHint
		}
		if text.Text != value {
			text.Text = value
			text.Refresh()
		}
		wantVisible := len(entries) > 0 || time.Now().Before(readyUntil)
		if hwnd != 0 && visible != wantVisible {
			// Fyne/GLFW must own visibility: a window shown via SetWindowPos is
			// re-hidden by the driver. GLFW shows without activating.
			if wantVisible {
				w.Show()
				// Fyne restores its own stored coordinates on every Show, so the
				// placement must be re-requested through Fyne after each one.
				placeOverlay()
			} else {
				w.Hide()
			}
			visible = wantVisible
			debugf("visibility -> %v (queue=%d)", wantVisible, len(entries))
		}
	}
	// Every notice offers a quit path: the tray may be unavailable, and it is
	// otherwise the only way to stop the program.
	showNotice := func(title, message string) {
		notice := a.NewWindow(title)
		quit := widget.NewButton("退出 KeyVivi", a.Quit)
		notice.SetContent(container.NewVBox(
			widget.NewLabel(message),
			container.NewHBox(widget.NewButton("关闭", notice.Close), quit),
		))
		notice.Resize(fyne.NewSize(480, 190))
		notice.Show()
	}
	tray := watchTray()
	var updateTray func()
	toggle := func() {
		controller.TogglePause()
		render()
		if updateTray != nil {
			updateTray()
		}
	}
	updateTray = setupTray(a, toggle, func() { showNotice("关于 KeyVivi", "KeyVivi v0.5\nCtrl+Alt+K：暂停 / 继续") }, func() bool { return controller.Paused })
	events := make(chan keyboard.Event, 1024)
	stopHook, err := platform.StartKeyboardHook(events)
	if err != nil {
		showNotice("监听错误", fmt.Sprintf("键盘监听启动失败: %v", err))
	} else {
		defer stopHook()
	}
	// KEYVIVI_SELFTEST=1 proves the hook chain end to end: same-process key
	// injection reaches the hook even when other input never arrives. The
	// result is shown on screen, so a field report needs no terminal.
	if os.Getenv("KEYVIVI_SELFTEST") != "" {
		go func() {
			time.Sleep(700 * time.Millisecond)
			before := platform.HookEventCount()
			platform.SendTestKey()
			time.Sleep(500 * time.Millisecond)
			after := platform.HookEventCount()
			ok := after > before
			debugf("hook self-test ok=%v before=%d after=%d", ok, before, after)
			verdict := "失败：钩子装上了但收不到任何事件"
			if ok {
				verdict = "通过：钩子能收到事件"
			}
			message := fmt.Sprintf("键盘钩子自检%s\n回调计数 %d → %d\n\n若此项通过但按键仍无反应，说明事件到达了钩子但后续处理或显示有问题；若失败，说明系统没有把键盘事件交给本程序。", verdict, before, after)
			fyne.Do(func() { showNotice("钩子自检", message) })
		}()
	}
	// KEYVIVI_DEMO=1 injects A, B, C once the startup hint has expired. It
	// exercises the whole path from hook callback to rendered queue without a
	// keyboard, which is how the display chain is verified here. It types into
	// whatever window has focus, so it stays strictly manual.
	if os.Getenv("KEYVIVI_DEMO") != "" {
		go func() {
			time.Sleep(readyHintDuration() + 1200*time.Millisecond)
			for _, vk := range []uint32{0x41, 0x42, 0x43} {
				platform.SendKey(vk)
				time.Sleep(400 * time.Millisecond)
			}
			debugf("demo keys injected; queue=%d hook=%d seen=%d", len(controller.Queue.Entries()), platform.HookEventCount(), eventsSeen.Load())
		}()
	}
	stopHotkey, err := platform.StartPauseHotkey(func() { fyne.Do(toggle) })
	if err != nil {
		showNotice("快捷键错误", fmt.Sprintf("Ctrl+Alt+K 注册失败，可使用托盘暂停。\n%v", err))
	} else {
		defer stopHotkey()
	}
	// Tray initialisation is asynchronous and unreported. Keep the fallback quit
	// hotkey so a missing tray never leaves the program unstoppable, but report
	// it on stderr instead of interrupting with a dialog.
	go func() {
		time.Sleep(2 * time.Second)
		failed, detail := tray.Failed()
		if !failed {
			return
		}
		stopQuit, err := platform.StartHotkey(platform.ModControl|platform.ModAlt|platform.ModNoRepeat, 'Q', func() { fyne.Do(a.Quit) })
		if err != nil {
			debugf("fallback quit hotkey failed: %v", err)
		} else {
			defer stopQuit()
		}
		debugf("tray unavailable (press Ctrl+Alt+Q to quit): %s", detail)
	}()
	done := make(chan struct{})
	defer close(done)
	go processKeys(events, done, func(e keyboard.Event) { controller.Handle(e, time.Now()); render() })
	go expireQueue(done, func(now time.Time) { controller.Queue.Expire(now); render() })
	go func() {
		for {
			select {
			case <-done:
				return
			case <-time.After(2 * time.Second):
				fyne.Do(func() {
					debugf("queue=%d hook=%d seen=%d paused=%v visible=%v", len(controller.Queue.Entries()), platform.HookEventCount(), eventsSeen.Load(), controller.Paused, visible)
				})
			}
		}
	}()
	// Create the native surface, then hand visibility to Fyne.
	w.Show()
	go func() {
		fyne.Do(func() {
			native, ok := w.(driver.NativeWindow)
			if !ok {
				showNotice("窗口错误", "无法获取原生窗口接口")
				return
			}
			native.RunNative(func(context any) {
				ctx, ok := context.(driver.WindowsWindowContext)
				if !ok || ctx.HWND == 0 {
					showNotice("窗口错误", "无法获取窗口句柄")
					return
				}
				hwnd = ctx.HWND
				debugf("native window acquired hwnd=%#x", hwnd)
				if err := platform.ConfigureOverlay(hwnd); err != nil {
					debugf("ConfigureOverlay err=%v", err)
					showNotice("窗口错误", err.Error())
				}
				w.Hide()
				visible = false
				readyUntil = time.Now().Add(readyHintDuration())
				placeOverlay()
				render()
			})
		})
	}()
	a.Run()
}
