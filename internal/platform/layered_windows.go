package platform

import (
	"fmt"
	"image"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var gdi32 = windows.NewLazyDLL("gdi32.dll")

var (
	createCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	deleteDC            = gdi32.NewProc("DeleteDC")
	createDIBSection    = gdi32.NewProc("CreateDIBSection")
	deleteObject        = gdi32.NewProc("DeleteObject")
	selectObject        = gdi32.NewProc("SelectObject")
	updateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	getDC               = user32.NewProc("GetDC")
	releaseDC           = user32.NewProc("ReleaseDC")
	showWindow          = user32.NewProc("ShowWindow")
)

const (
	wsPopup            = 0x80000000
	wsExLayered        = 0x00080000
	wsExToolWindow     = 0x00000080
	wsExNoActivate     = 0x08000000
	wsExTransparent    = 0x00000020
	wsExTopmost        = 0x00000008
	swShowNoActivate   = 4
	swHide             = 0
	ulwAlpha           = 0x00000002
	dibRGBColors       = 0
	acSrcOver          = 0x00
	acSrcAlpha         = 0x01
	blendAlphaConstant = 255
)

type bitmapInfoHeader struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
}

type bitmapInfo struct {
	header bitmapInfoHeader
	colors [1]uint32
}

type blendFunction struct {
	blendOp             byte
	blendFlags          byte
	sourceConstantAlpha byte
	alphaFormat         byte
}

type winSize struct{ cx, cy int32 }

// LayeredWindow 是一个逐像素 alpha 的置顶叠加窗口，内容由调用方提供的位图决定。
// 这解决了此前 OpenGL 与分层窗口冲突的问题：这里不再有 GL，位图由我们自己提交。
//
// 窗口有线程亲和性，必须在 UI 线程上创建与使用。
type LayeredWindow struct {
	hwnd      uintptr
	screenDC  uintptr
	memDC     uintptr
	bitmap    uintptr
	oldBitmap uintptr
	pixels    unsafe.Pointer
	width     int
	height    int
	shown     bool
}

// NewLayeredWindow 创建叠加窗口。必须在 UI 线程调用。
func NewLayeredWindow() (*LayeredWindow, error) {
	hwnd, _, err := createWindowEx.Call(
		wsExLayered|wsExToolWindow|wsExNoActivate|wsExTransparent|wsExTopmost,
		uintptr(unsafe.Pointer(windowClass)),
		0,
		wsPopup,
		0, 0, 0, 0,
		0, 0, 0, 0,
	)
	if hwnd == 0 {
		return nil, win32Error("CreateWindowExW(layered)", err)
	}
	window := &LayeredWindow{hwnd: hwnd}
	window.screenDC, _, _ = getDC.Call(0)
	if window.screenDC == 0 {
		destroyWindow.Call(hwnd)
		return nil, fmt.Errorf("GetDC failed")
	}
	window.memDC, _, err = createCompatibleDC.Call(window.screenDC)
	if window.memDC == 0 {
		releaseDC.Call(0, window.screenDC)
		destroyWindow.Call(hwnd)
		return nil, win32Error("CreateCompatibleDC", err)
	}
	return window, nil
}

// Handle 返回原生窗口句柄。
func (w *LayeredWindow) Handle() uintptr { return w.hwnd }

// Apply 提交位图、位置与尺寸。传 nil 表示隐藏窗口。
func (w *LayeredWindow) Apply(img *image.RGBA, x, y int) error {
	if img == nil {
		showWindow.Call(w.hwnd, swHide)
		w.shown = false
		return nil
	}
	bounds := img.Bounds()
	if err := w.ensureSurface(bounds.Dx(), bounds.Dy()); err != nil {
		return err
	}
	w.copyPixels(img)

	dst := point{x: int32(x), y: int32(y)}
	size := winSize{cx: int32(w.width), cy: int32(w.height)}
	src := point{}
	blend := blendFunction{blendOp: acSrcOver, alphaFormat: acSrcAlpha, sourceConstantAlpha: blendAlphaConstant}
	ret, _, err := updateLayeredWindow.Call(
		w.hwnd,
		w.screenDC,
		uintptr(unsafe.Pointer(&dst)),
		uintptr(unsafe.Pointer(&size)),
		w.memDC,
		uintptr(unsafe.Pointer(&src)),
		0,
		uintptr(unsafe.Pointer(&blend)),
		ulwAlpha,
	)
	if ret == 0 {
		return win32Error("UpdateLayeredWindow", err)
	}
	if !w.shown {
		showWindow.Call(w.hwnd, swShowNoActivate)
		w.shown = true
	}
	// 仅靠 UpdateLayeredWindow 有时不会让窗口管理器接手分层表面，
	// 再显式要求显示并置顶一次。
	setWindowPos.Call(w.hwnd, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w.width), uintptr(w.height),
		swpNoActivate|swpShowWindow)
	traceLayered("apply hwnd=%#x dc=%#x memdc=%#x bmp=%#x px=%p %dx%d at %d,%d ret=%d err=%v first=%v",
		w.hwnd, w.screenDC, w.memDC, w.bitmap, w.pixels, w.width, w.height, x, y, ret, err, w.samplePixels())
	return nil
}

// samplePixels 回读 DIB 中心像素，用于确认内容真的写进去了。
func (w *LayeredWindow) samplePixels() []byte {
	if w.pixels == nil || w.width < 1 || w.height < 1 {
		return nil
	}
	offset := ((w.height/2)*w.width + w.width/2) * 4
	return unsafe.Slice((*byte)(unsafe.Add(w.pixels, offset)), 4)
}

func traceLayered(format string, args ...any) {
	if os.Getenv("KEYVIVI_DEBUG") == "" {
		return
	}
	fmt.Fprintf(os.Stderr, "[keyvivi/layered] "+format+"\n", args...)
}

// Destroy 释放位图、DC 与窗口。
func (w *LayeredWindow) Destroy() {
	if w.bitmap != 0 {
		selectObject.Call(w.memDC, w.oldBitmap)
		deleteObject.Call(w.bitmap)
		w.bitmap = 0
	}
	if w.memDC != 0 {
		deleteDC.Call(w.memDC)
		w.memDC = 0
	}
	if w.screenDC != 0 {
		releaseDC.Call(0, w.screenDC)
		w.screenDC = 0
	}
	if w.hwnd != 0 {
		destroyWindow.Call(w.hwnd)
		w.hwnd = 0
	}
}

// ensureSurface 按需重建 32 位 top-down DIB。
func (w *LayeredWindow) ensureSurface(width, height int) error {
	if width < 1 || height < 1 {
		return fmt.Errorf("layered window size %dx%d is invalid", width, height)
	}
	if w.bitmap != 0 && w.width == width && w.height == height {
		return nil
	}
	if w.bitmap != 0 {
		selectObject.Call(w.memDC, w.oldBitmap)
		deleteObject.Call(w.bitmap)
		w.bitmap, w.pixels = 0, nil
	}
	info := bitmapInfo{header: bitmapInfoHeader{
		size:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		width:       int32(width),
		height:      -int32(height), // 负高度 = top-down，与 image.RGBA 行序一致
		planes:      1,
		bitCount:    32,
		compression: 0, // BI_RGB
	}}
	var bits unsafe.Pointer
	bitmap, _, err := createDIBSection.Call(
		w.screenDC,
		uintptr(unsafe.Pointer(&info)),
		dibRGBColors,
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	if bitmap == 0 {
		return win32Error("CreateDIBSection", err)
	}
	w.oldBitmap, _, _ = selectObject.Call(w.memDC, bitmap)
	w.bitmap, w.pixels = bitmap, bits
	w.width, w.height = width, height
	return nil
}

// copyPixels 把 Go 的 RGBA（预乘）写成 DIB 需要的 BGRA 字节序。
func (w *LayeredWindow) copyPixels(img *image.RGBA) {
	rowBytes := w.width * 4
	dst := unsafe.Slice((*byte)(w.pixels), rowBytes*w.height)
	for y := 0; y < w.height; y++ {
		src := img.Pix[y*img.Stride : y*img.Stride+rowBytes]
		row := dst[y*rowBytes : (y+1)*rowBytes]
		for i := 0; i < rowBytes; i += 4 {
			row[i+0] = src[i+2] // B
			row[i+1] = src[i+1] // G
			row[i+2] = src[i+0] // R
			row[i+3] = src[i+3] // A
		}
	}
}
