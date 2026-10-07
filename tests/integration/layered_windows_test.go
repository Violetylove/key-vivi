//go:build windows && integration

package integration_test

import (
	"fmt"
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
			loop.OnCleanup(window.Destroy)

			if err := window.Apply(bitmap, wantX, wantY); err != nil {
				problems = append(problems, fmt.Sprintf("Apply: %v", err))
				return nil
			}
			harness := windows.NewLazyDLL("user32.dll")
			hwnd := window.Handle()

			// 逐像素透明依赖分层样式，位图提交成功不能替代真实桌面可见性验收。
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
