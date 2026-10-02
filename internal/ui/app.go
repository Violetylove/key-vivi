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

// debugf 在 KEYVIVI_DEBUG 设置时向 stderr 输出排查信息；只记数量，不记按键内容。
func debugf(format string, args ...any) {
	if os.Getenv("KEYVIVI_DEBUG") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "[keyvivi] "+format+"\n", args...)
}

// readyHintDuration 返回启动提示的停留时长；KEYVIVI_READY_HINT（秒）可覆盖，0 为关闭。
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

// 字幕的逻辑尺寸。固定尺寸窗口会收缩到内容最小尺寸，所以内容必须显式声明。
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
	// 空文本会让固定尺寸窗口塌成缩略图，居中后贴到屏幕右侧，故由背景撑开尺寸。
	bg.SetMinSize(fyne.NewSize(overlayWidth, overlayHeight))
	w.SetContent(container.NewStack(bg, container.NewCenter(text)))
	w.Resize(fyne.NewSize(overlayWidth, overlayHeight))
	w.SetCloseIntercept(a.Quit)
	controller := display.NewController()
	// 启动提示用于区分"在运行"与"没启动"；受限令牌只写进提示文字，不弹窗打扰。
	readyHint := "KeyVivi 已启动"
	if restricted, level, err := platform.RestrictedIntegrity(); err != nil {
		debugf("integrity check failed: %v", err)
	} else {
		debugf("integrity level=%#x restricted=%v", level, restricted)
		if restricted {
			readyHint = fmt.Sprintf("KeyVivi 已启动（受限环境 %#x，仅本窗口有效）", level)
		}
		// 调试模式下把完整性级别写进标题，便于从外部确认启动方式。
		if os.Getenv("KEYVIVI_DEBUG") != "" {
			w.SetTitle(fmt.Sprintf("KeyVivi %#x", level))
		}
	}
	readyUntil := time.Time{}
	var hwnd uintptr
	visible := false
	// 通过 Fyne 把窗口放到显示器底部居中。必须走 RequestPosition：Fyne 会保存
	// 给定坐标并在每次 Show 时恢复，直接 SetWindowPos 会被撤销。
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
			// 可见性归 Fyne/GLFW，用 SetWindowPos 显示的窗口会被驱动重新隐藏。
			if wantVisible {
				w.Show()
				// Fyne 每次 Show 都会恢复自己保存的坐标，显示后需重新请求定位。
				placeOverlay()
			} else {
				w.Hide()
			}
			visible = wantVisible
			debugf("visibility -> %v (queue=%d)", wantVisible, len(entries))
		}
	}
	// 提示窗口一律带退出入口：托盘不可用时它是唯一的退出方式。
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
	// KEYVIVI_SELFTEST=1：注入 F24 验证钩子链路，结论显示在屏幕上，无需终端。
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
	// KEYVIVI_DEMO=1：启动提示结束后注入 A、B、C，验证从钩子到队列的完整链路。
	// 会向当前焦点窗口真实输入这三个字母，仅手动排查时使用。
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
	// 托盘初始化异步且无返回值；失败时静默注册 Ctrl+Alt+Q 兜底退出并写日志。
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
	// 先创建原生窗口，之后可见性交给 Fyne。
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
