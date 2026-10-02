//go:build windows && integration

package integration_test

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"key-vivi/internal/platform"
	"key-vivi/internal/render"
)

type rect32 struct{ left, top, right, bottom int32 }

// TestLayeredWindowAcceptsRenderedBar 在 UI 线程上创建叠加窗口、提交一张真实渲染的
// 字幕位图，然后在同一线程内校验窗口样式与几何——窗口有线程亲和性，Destroy 也必须
// 在创建它的线程上执行。
func TestLayeredWindowAcceptsRenderedBar(t *testing.T) {
	const (
		wantX = 120
		wantY = 140
	)
	bitmap, err := render.Bar([]string{"Ctrl+C", "A ×3"}, render.DefaultTheme(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}

	results := make(chan []string, 1)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		_ = platform.Run(func(loop *platform.Loop) error {
			defer loop.Quit()
			var problems []string
			defer func() { results <- problems }()

			window, err := platform.NewLayeredWindow()
			if err != nil {
				problems = append(problems, fmt.Sprintf("NewLayeredWindow: %v", err))
				return nil
			}
			defer window.Destroy()

			if err := window.Apply(bitmap, wantX, wantY); err != nil {
				problems = append(problems, fmt.Sprintf("Apply: %v", err))
				return nil
			}
			harness := windows.NewLazyDLL("user32.dll")
			hwnd := window.Handle()

			// WS_EX_LAYERED 在这里是必需项：内容由 UpdateLayeredWindow 提交，
			// 没有 OpenGL 参与，不会再出现此前"可见却零像素"的情况。
			exIndex := int32(-20)
			exStyle, _, _ := harness.NewProc("GetWindowLongW").Call(hwnd, uintptr(exIndex))
			required := uintptr(0x00080000 | 0x00000080 | 0x08000000 | 0x00000020)
			if exStyle&required != required {
				problems = append(problems, fmt.Sprintf("extended style %#x missing %#x", exStyle, required))
			}
			if exStyle&0x00000008 == 0 {
				problems = append(problems, fmt.Sprintf("WS_EX_TOPMOST not retained (%#x)", exStyle))
			}
			if visible, _, _ := harness.NewProc("IsWindowVisible").Call(hwnd); visible == 0 {
				problems = append(problems, "window is not visible after Apply")
			}
			var bounds rect32
			if ret, _, err := harness.NewProc("GetWindowRect").Call(hwnd, uintptr(unsafe.Pointer(&bounds))); ret == 0 {
				problems = append(problems, fmt.Sprintf("GetWindowRect: %v", err))
			} else {
				if got, want := int(bounds.right-bounds.left), bitmap.Bounds().Dx(); got != want {
					problems = append(problems, fmt.Sprintf("width %d, bitmap %d", got, want))
				}
				if got, want := int(bounds.bottom-bounds.top), bitmap.Bounds().Dy(); got != want {
					problems = append(problems, fmt.Sprintf("height %d, bitmap %d", got, want))
				}
				if bounds.left != wantX || bounds.top != wantY {
					problems = append(problems, fmt.Sprintf("at %d,%d, asked %d,%d", bounds.left, bounds.top, wantX, wantY))
				}
			}
			// 隐藏路径也要可用：空闲时用它把窗口收起来。
			if err := window.Apply(nil, 0, 0); err != nil {
				problems = append(problems, fmt.Sprintf("Apply(nil): %v", err))
			} else if visible, _, _ := harness.NewProc("IsWindowVisible").Call(hwnd); visible != 0 {
				problems = append(problems, "window still visible after Apply(nil)")
			}
			return nil
		}, func(time.Time) bool { return false })
	}()

	problems := <-results
	<-stopped
	for _, problem := range problems {
		t.Error(problem)
	}
}

// TestOverlayPreview 在设置 KEYVIVI_SHOW_OVERLAY 时把字幕显示在屏幕底部居中若干秒，
// 供人工确认真正的半透明、圆角与定位；未设置时跳过。
//
// 注意：本测试只能自证"窗口被创建、位图已提交、几何正确"，不能自证肉眼可见。
// 屏幕上是否真的出现字幕必须由人在真实桌面上确认。
func TestOverlayPreview(t *testing.T) {
	if os.Getenv("KEYVIVI_SHOW_OVERLAY") == "" {
		t.Skip("set KEYVIVI_SHOW_OVERLAY=1 to put the overlay on screen")
	}
	seconds := 8
	if raw := os.Getenv("KEYVIVI_SHOW_OVERLAY"); raw != "1" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			seconds = parsed
		}
	}
	bitmap, err := render.Bar(
		[]string{"Ctrl+C", "A ×3", "Backspace", "↓"},
		render.DefaultTheme(), 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = platform.Run(func(loop *platform.Loop) error {
			go func() {
				time.Sleep(time.Duration(seconds) * time.Second)
				loop.Quit()
			}()
			window, err := platform.NewLayeredWindow()
			if err != nil {
				return err
			}
			defer window.Destroy()
			// 先给窗口一个尺寸，才能问到它所在的显示器。
			if err := window.Apply(bitmap, 0, 0); err != nil {
				return err
			}
			x, y, err := platform.OverlayPosition(window.Handle())
			if err != nil {
				return err
			}
			t.Logf("overlay %dx%d at %d,%d for %ds", bitmap.Bounds().Dx(), bitmap.Bounds().Dy(), x, y, seconds)
			return window.Apply(bitmap, x, y)
		}, func(time.Time) bool { return false })
	}()
	<-done
}
