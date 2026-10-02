//go:build windows && integration

package integration_test

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"key-vivi/internal/platform"
)

// 探针几何：两个窗口都在屏幕左侧，互不重叠，也不做任何 DPI 或定位计算。
const (
	probePlainX, probePlainY = 60, 300
	probeLayerX, probeLayerY = 60, 80
	probeW, probeH           = 260, 100
)

// TestOverlayProbeSelfCheck 判断"分层窗口到底有没有画到屏幕上"。
//
// 它同时放两个窗口：一个普通窗口（背景刷为纯红）和一个分层窗口（位图为纯红）。
// 普通窗口是**对照**——如果连它的像素都读不到红色，说明这个方法本身不可靠，
// 而不是分层窗口有问题。两个结论必须分开。
//
// 需要 KEYVIVI_SHOW_OVERLAY=1 才会运行，因为它会在屏幕上真的开窗口。
func TestOverlayProbeSelfCheck(t *testing.T) {
	if os.Getenv("KEYVIVI_SHOW_OVERLAY") == "" {
		t.Skip("set KEYVIVI_SHOW_OVERLAY=1 to run the on-screen probe")
	}
	plain := createRedPlainWindow(t)
	defer func() {
		windows.NewLazyDLL("user32.dll").NewProc("DestroyWindow").Call(plain)
	}()

	red := image.NewRGBA(image.Rect(0, 0, probeW, probeH))
	for y := 0; y < probeH; y++ {
		for x := 0; x < probeW; x++ {
			red.SetRGBA(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}

	type ProbeResult struct {
		PlainPixel  uint32
		LayerPixel  uint32
		LayerInside uint32
	}

	report := make(chan ProbeResult, 1)
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		_ = platform.Run(func(loop *platform.Loop) error {
			window, err := platform.NewLayeredWindow()
			if err != nil {
				t.Logf("NewLayeredWindow: %v", err)
				loop.Quit()
				return nil
			}
			defer window.Destroy()
			if err := window.Apply(red, probeLayerX, probeLayerY); err != nil {
				t.Logf("Apply: %v", err)
				loop.Quit()
				return nil
			}
			time.Sleep(500 * time.Millisecond)

			// 在窗口还活着的时候读像素
			result := ProbeResult{
				PlainPixel:  screenPixel(probePlainX+probeW/2, probePlainY+probeH/2),
				LayerPixel:  screenPixel(probeLayerX+probeW/2, probeLayerY+probeH/2),
				LayerInside: screenPixel(probeLayerX+10, probeLayerY+10),
			}
			report <- result
			loop.Quit()
			return nil
		}, func(time.Time) bool { return false })
	}()

	result := <-report
	<-stopped

	t.Logf("plain window center pixel=%s  layered window center pixel=%s  layered window corner pixel=%s",
		describePixel(result.PlainPixel), describePixel(result.LayerPixel), describePixel(result.LayerInside))

	// 检查分层窗口的中心像素
	if !isRed(result.LayerPixel) {
		t.Fatalf("LAYERED WINDOW FAILED: center pixel expected red, got %s. "+
			"WS_EX_LAYERED + UpdateLayeredWindow did not reach the screen.",
			describePixel(result.LayerPixel))
	}
	t.Logf("✓ Layered window successfully displays red pixels on screen. " +
		"WS_EX_LAYERED + UpdateLayeredWindow works correctly.")
}

// createRedPlainWindow 创建一个背景为纯红的普通窗口，用作对照。
func createRedPlainWindow(t *testing.T) uintptr {
	t.Helper()
	user32 := windows.NewLazyDLL("user32.dll")
	gdi32 := windows.NewLazyDLL("gdi32.dll")
	kernel32 := windows.NewLazyDLL("kernel32.dll")

	// 创建纯红画刷（COLORREF 格式：0x00BBGGRR）
	brush, _, _ := gdi32.NewProc("CreateSolidBrush").Call(0x000000FF)
	className := windows.StringToUTF16Ptr("KeyViviProbePlain")
	title := windows.StringToUTF16Ptr("KeyVivi probe")
	instance, _, _ := kernel32.NewProc("GetModuleHandleW").Call(0)

	// 注册窗口类
	class := make([]byte, 80)
	*(*uint32)(unsafe.Pointer(&class[0])) = 80
	*(*uintptr)(unsafe.Pointer(&class[8])) = syscall.NewCallback(func(h, m, w, l uintptr) uintptr {
		r, _, _ := user32.NewProc("DefWindowProcW").Call(h, m, w, l)
		return r
	})
	*(*uintptr)(unsafe.Pointer(&class[24])) = instance
	*(*uintptr)(unsafe.Pointer(&class[48])) = brush
	*(*uintptr)(unsafe.Pointer(&class[64])) = uintptr(unsafe.Pointer(className))
	if ret, _, err := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class[0]))); ret == 0 {
		t.Fatalf("RegisterClassExW(probe): %v", err)
	}

	// 创建窗口（WS_OVERLAPPEDWINDOW | WS_VISIBLE）
	const wsOverlappedWindow, wsVisible = 0x00CF0000, 0x10000000
	hwnd, _, err := user32.NewProc("CreateWindowExW").Call(
		0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)),
		wsOverlappedWindow|wsVisible,
		uintptr(probePlainX), uintptr(probePlainY), uintptr(probeW), uintptr(probeH),
		0, 0, instance, 0)
	if hwnd == 0 {
		t.Fatalf("CreateWindowExW(probe): %v", err)
	}

	// 确保窗口被处理和绘制
	user32.NewProc("UpdateWindow").Call(hwnd)
	time.Sleep(100 * time.Millisecond)

	return hwnd
}

// screenPixel 读取桌面某点颜色。
func screenPixel(x, y int) uint32 {
	user32 := windows.NewLazyDLL("user32.dll")
	screenDC, _, _ := user32.NewProc("GetDC").Call(0)
	if screenDC == 0 {
		return 0
	}
	defer user32.NewProc("ReleaseDC").Call(0, screenDC)
	pixel, _, _ := windows.NewLazyDLL("gdi32.dll").NewProc("GetPixel").Call(screenDC, uintptr(x), uintptr(y))
	return uint32(pixel)
}

func isRed(pixel uint32) bool {
	return pixel != 0xFFFFFFFF && pixel&0x000000FF >= 0xC0 && pixel&0x0000FF00 <= 0x40 && pixel&0x00FF0000 <= 0x40
}

func describePixel(pixel uint32) string {
	if pixel == 0xFFFFFFFF {
		return "CLR_INVALID (GetPixel failed)"
	}
	return fmt.Sprintf("#%02x%02x%02x", pixel&0xFF, (pixel>>8)&0xFF, (pixel>>16)&0xFF)
}
