package app

import (
	"fmt"
	"image"
	"log"
	"os"
	"time"

	"key-vivi/internal/keyboard"
	"key-vivi/internal/platform"
	"key-vivi/internal/render"
)

// Run 装配自绘窗口、输入与托盘，退出和错误均先完成资源清理。
func Run() error {
	level, levelErr := platform.IntegrityLevel()
	if levelErr != nil {
		return fmt.Errorf("integrity check: %w", levelErr)
	}
	// Low文件标签会让普通桌面启动也降权，必须在装配托盘和钩子之前明确失败。
	if err := checkStartupIntegrity(level); err != nil {
		return err
	}
	if os.Getenv("KEYVIVI_DEBUG") != "" {
		log.Printf("integrity level=%#x", level)
	}
	if err := platform.EnableDPIAwareness(); err != nil {
		log.Printf("DPI awareness: %v", err)
	}

	path, err := executableConfigPath()
	if err != nil {
		return fmt.Errorf("config path: %w", err)
	}
	store := &configStore{path: path}
	activeConfig, configErr := store.load()
	state := newSession()
	state.configure(activeConfig, time.Now(), nil)
	state.controller.Paused = activeConfig.Behavior.StartPaused
	theme := activeConfig.theme()
	events := make(chan keyboard.Event, 1024)
	pauses := make(chan struct{}, 64)
	var loop *platform.Loop
	var overlay *platform.LayeredWindow
	var tray *platform.Tray
	var settings *platform.Settings
	var exitRegistered bool
	picture := scene{widthLogical: float64(activeConfig.Region.Width), options: state.controller.Queue.Options}
	var lastGeneration uint64
	var workLeft, workTop, workWidth, workHeight int
	var workScale float64
	var regionPixels int
	var shown bool
	lastDropped := platform.HookDroppedCount()
	selftest := os.Getenv("KEYVIVI_SELFTEST") != ""
	var testFire, testCheck time.Time
	var testCount uint64

	return platform.Run(func(ui *platform.Loop) error {
		loop = ui
		var err error
		overlay, err = platform.NewLayeredWindow()
		if err != nil {
			return fmt.Errorf("overlay: %w", err)
		}
		loop.OnCleanup(overlay.Destroy)
		loop.OnCleanup(render.CloseFonts)
		stopHook, err := platform.StartKeyboardHookWithWake(events, func() { _ = loop.Wake() })
		if err != nil {
			return fmt.Errorf("keyboard hook: %w", err)
		}
		loop.OnCleanup(stopHook)
		loop.OnCleanup(func() {
			if settings != nil {
				settings.Destroy()
			}
		})
		state.reset(time.Now(), platform.HeldKeys())

		openSettings := func() {
			if settings != nil && settings.Handle() != 0 {
				settings.Show()
				return
			}
			window, err := platform.NewSettings(loop, platform.SettingsOptions{
				Fields: configFields(activeConfig), Path: path,
				Icon:     func(size int) *image.RGBA { return render.TrayIcon(size, false) },
				Defaults: func() map[string]string { return configValues(defaultConfig()) },
				Preview: func(values map[string]string, scale float64, width, height int) (*image.RGBA, error) {
					c, err := configFromValues(values)
					if err != nil {
						return nil, err
					}
					_, _, ww, wh, _, err := platform.PrimaryWorkArea(overlay.Handle())
					if err != nil {
						return nil, err
					}
					return configPreview(c, scale, width, height, ww, wh)
				},
				Save: func(values map[string]string) error {
					c, err := configFromValues(values)
					if err != nil {
						return err
					}
					if err := store.save(c); err != nil {
						return err
					}
					activeConfig, theme = c, c.theme()
					state.configure(c, time.Now(), platform.HeldKeys())
					picture = scene{widthLogical: float64(c.Region.Width), options: state.controller.Queue.Options}
					workWidth = 0
					if err := overlay.Apply(nil, 0, 0); err != nil {
						loop.Fail(err)
						return err
					}
					shown = false
					return loop.Wake()
				},
			})
			if err != nil {
				platform.ShowMessage(loop.Handle(), "设置无法打开", err.Error())
				return
			}
			settings = window
		}

		toggle := func() {
			state.toggle(time.Now(), platform.HeldKeys())
			if tray != nil {
				if err := tray.SetPaused(state.controller.Paused); err != nil {
					log.Printf("tray update: %v", err)
				}
			}
			if err := overlay.Apply(nil, 0, 0); err != nil {
				loop.Fail(err)
			}
			shown = false
			picture.reset()
			_ = loop.Wake()
		}
		stopPause, pauseErr := platform.StartPauseHotkey(func() {
			select {
			case pauses <- struct{}{}:
			default:
			}
			_ = loop.Wake()
		})
		if pauseErr == nil {
			loop.OnCleanup(stopPause)
		}

		ensureExit := func() error {
			if exitRegistered {
				return nil
			}
			stop, err := platform.StartHotkey(platform.ModControl|platform.ModAlt|platform.ModNoRepeat, 'Q', loop.Quit)
			if err != nil {
				return fmt.Errorf("fallback exit hotkey: %w", err)
			}
			loop.OnCleanup(stop)
			exitRegistered = true
			state.controller.FilterExitHotkey = true
			return nil
		}
		trayError := func(err error) {
			log.Printf("tray unavailable: %v", err)
			if err := ensureExit(); err != nil {
				loop.Fail(err)
				return
			}
			state.showHint(render.HintText("托盘不可用，Ctrl+Alt+Q 退出", "Tray unavailable; Ctrl+Alt+Q to exit"), time.Now(), 6*time.Second)
			_ = loop.Wake()
		}
		tray, err = platform.NewTray(loop, platform.TrayActions{
			Settings: openSettings,
			Icon:     render.TrayIcon,
			Toggle:   toggle,
			About: func() {
				platform.ShowMessage(loop.Handle(), "关于 KeyVivi",
					"KeyVivi 自绘开发版\nCtrl+Alt+K：暂停 / 继续\n不记录或上传按键内容")
			},
			Exit:  loop.Quit,
			Error: trayError,
		})
		now := time.Now()
		if err != nil {
			trayError(err)
			if !exitRegistered {
				return fmt.Errorf("tray and exit controls unavailable: %w", err)
			}
		} else {
			loop.OnCleanup(tray.Destroy)
			if state.controller.Paused {
				if err := tray.SetPaused(true); err != nil {
					return err
				}
			}
			if duration := readyHintDuration(); duration > 0 {
				hint := render.HintText("KeyVivi 已启动", "KeyVivi ready")
				state.showHint(hint, now, duration)
			}
		}
		if pauseErr != nil {
			log.Printf("pause hotkey unavailable: %v", pauseErr)
			if tray == nil {
				return fmt.Errorf("pause and tray controls unavailable: %w", pauseErr)
			}
			state.showHint(render.HintText("Ctrl+Alt+K 注册失败，请使用托盘暂停", "Ctrl+Alt+K unavailable; pause from tray"), now, 6*time.Second)
		}
		if selftest {
			testFire = now.Add(700 * time.Millisecond)
		}
		if configErr != nil {
			platform.ShowMessage(loop.Handle(), "KeyVivi 配置提示", "已使用默认设置，原配置未自动覆盖。\n"+configErr.Error()+"\n配置路径："+path)
		}
		if os.Getenv("KEYVIVI_DEBUG") != "" {
			log.Printf("startup ready: tray=%v pause_hotkey=%v", tray != nil, pauseErr == nil)
		}
		_ = loop.Wake()
		return nil
	}, func(now time.Time) bool {
		// 控制通知先处理，重置时间屏障阻止暂停期间排队的输入在恢复后出现。
		for {
			select {
			case <-pauses:
				state.toggle(time.Now(), platform.HeldKeys())
				if tray != nil {
					if err := tray.SetPaused(state.controller.Paused); err != nil {
						log.Printf("tray update: %v", err)
					}
				}
			default:
				goto controlsDone
			}
		}
	controlsDone:
		if dropped := platform.HookDroppedCount(); dropped != lastDropped {
			log.Printf("keyboard buffer overflow: dropped=%d", dropped-lastDropped)
			lastDropped = dropped
			state.reset(time.Now(), platform.HeldKeys())
		}
		generation := loop.DisplayGeneration()
		geometryChanged := workWidth == 0 || generation != lastGeneration
		if geometryChanged {
			var err error
			workLeft, workTop, workWidth, workHeight, workScale, err = platform.PrimaryWorkArea(overlay.Handle())
			if err != nil {
				loop.Fail(err)
				return false
			}
			regionPixels, err = regionHeight(activeConfig, theme, workScale)
			if err != nil {
				loop.Fail(err)
				return false
			}
		}
		maxWidth := widthLimit(workWidth, workScale, max(200, theme.MinWidth))
		if geometryChanged {
			limit := configuredRowWidth(float64(activeConfig.Region.Width), workScale, maxWidth)
			state.controller.Queue.Fits = func(items []string) bool {
				plan, err := render.Layout(items, theme, workScale, 0)
				if err != nil {
					loop.Fail(err)
					return false
				}
				return float64(plan.Width) <= limit
			}
			lastGeneration = generation
		}
		for {
			select {
			case e := <-events:
				state.input(e, now)
			default:
				goto inputsDone
			}
		}
	inputsDone:
		if selftest && !testFire.IsZero() && !now.Before(testFire) {
			testCount = platform.HookEventCount()
			platform.SendTestKey()
			testFire = time.Time{}
			testCheck = now.Add(500 * time.Millisecond)
		}
		if !testCheck.IsZero() && !now.Before(testCheck) {
			after := platform.HookEventCount()
			text := render.HintText("钩子自检失败，请检查运行环境", "Hook self-test failed; check environment")
			if after > testCount {
				text = render.HintText("钩子自检通过", "Hook self-test passed")
			}
			log.Printf("hook self-test before=%d after=%d", testCount, after)
			state.showHint(text, now, 5*time.Second)
			testCheck = time.Time{}
		}
		visuals := state.visuals(now)
		if len(visuals) == 0 {
			if shown {
				if err := overlay.Apply(nil, 0, 0); err != nil {
					loop.Fail(err)
				}
				shown = false
			}
			picture.reset()
			return !testFire.IsZero() || !testCheck.IsZero()
		}
		img, changed, err := picture.draw(visuals, theme, workScale, maxWidth, now)
		if err != nil {
			loop.Fail(err)
			return false
		}
		if !changed && shown && !geometryChanged {
			return true
		}
		if img.Bounds().Dy() > workHeight {
			img = img.SubImage(image.Rect(0, img.Bounds().Dy()-workHeight, img.Bounds().Dx(), img.Bounds().Dy())).(*image.RGBA)
		}
		anchor := positionAnchor(activeConfig.Region.Position)
		ox, oy := activeConfig.regionOffsets(workScale)
		x, y := platform.RegionPosition(workLeft, workTop, workWidth, workHeight, img.Bounds().Dx(), img.Bounds().Dy(), regionPixels, anchor,
			ox, oy)
		if err := overlay.Apply(img, x, y); err != nil {
			loop.Fail(err)
			return false
		}
		shown = true
		lastGeneration = generation
		return true
	})
}
