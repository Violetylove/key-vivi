package render

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// 系统字体失败时使用内嵌轮廓字体，降级后仍保持相同的 DPI 缩放规则。
var fontCandidates = []string{"Deng.ttf", "msyh.ttc", "simhei.ttf", "simsun.ttc"}

var (
	fontOnce sync.Once
	fontData []byte
	fontErr  error
)

// systemFontData 读取并解析第一个可用的系统字体，只做一次。
func systemFontData() ([]byte, error) {
	fontOnce.Do(func() {
		dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
		for _, name := range fontCandidates {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				continue
			}
			// 解析一次即可确认可用；失败就试下一个。
			if _, err := parseFont(data); err != nil {
				continue
			}
			fontData = data
			return
		}
		fontErr = errors.New("no usable system font found")
	})
	return fontData, fontErr
}

// parseFont 兼容单体 TTF 与 TTC 集合（集合取第一个字体）。
func parseFont(data []byte) (*opentype.Font, error) {
	if f, err := opentype.Parse(data); err == nil {
		return f, nil
	}
	collection, err := opentype.ParseCollection(data)
	if err != nil {
		return nil, err
	}
	if collection.NumFonts() == 0 {
		return nil, errors.New("empty font collection")
	}
	return collection.Font(0)
}

type cachedFace struct {
	size, dpi float64
	face      font.Face
}

// faceCache 保留最近的字体，键名与小计数交替绘制时不反复解析字体。
type faceCache struct {
	mu     sync.Mutex
	parsed *opentype.Font
	faces  []cachedFace
}

var cache faceCache

// faceFor 只在持有 cache.mu 时调用，锁必须覆盖测量、绘制与关闭整个过程。
func faceFor(size, dpi float64) (font.Face, error) {
	for i, item := range cache.faces {
		if item.dpi == dpi && item.size == size {
			copy(cache.faces[i:], cache.faces[i+1:])
			cache.faces[len(cache.faces)-1] = item
			return item.face, nil
		}
	}
	if cache.parsed == nil {
		data, err := systemFontData()
		if err != nil {
			data = goregular.TTF
		}
		parsed, err := parseFont(data)
		if err != nil {
			return nil, fmt.Errorf("parse font: %w", err)
		}
		cache.parsed = parsed
	}
	face, err := opentype.NewFace(cache.parsed, &opentype.FaceOptions{
		// 逻辑像素转为点，DPI 只在这里缩放一次。
		Size:    size * 72 / 96,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("new face: %w", err)
	}
	// 限制超宽输入/DPI 变化产生的字号缓存，避免长时间使用持续增长。
	if len(cache.faces) == 16 {
		cache.faces[0].face.Close()
		cache.faces = cache.faces[1:]
	}
	cache.faces = append(cache.faces, cachedFace{size: size, dpi: dpi, face: face})
	return face, nil
}

// CloseFonts 释放字体 face，退出时与测量、绘制串行。
func CloseFonts() {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	for _, item := range cache.faces {
		item.face.Close()
	}
	cache.faces = nil
	cache.parsed = nil
}

// HintText 在所用字体缺少提示字形时返回英文提示，避免降级后显示方框。
func HintText(primary, fallback string) string {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	face, err := faceFor(18, 96)
	if err != nil {
		return fallback
	}
	for _, r := range primary {
		if _, ok := face.GlyphAdvance(r); !ok {
			return fallback
		}
	}
	return primary
}

// textSize 返回文本在给定 face 下的像素尺寸。
func textSize(face font.Face, text string) (width, height float64) {
	advance := font.MeasureString(face, text)
	metrics := face.Metrics()
	return float64(advance) / 64, float64(metrics.Ascent+metrics.Descent) / 64
}

// drawText 以 left 为左边界、top 为行框顶部绘制文本，基线在行框内垂直居中。
func drawText(dst *image.RGBA, face font.Face, text string, left, top float64, c color.RGBA) {
	metrics := face.Metrics()
	lineHeight := float64(metrics.Ascent+metrics.Descent) / 64
	baseline := top + lineHeight/2 + float64(metrics.Ascent-metrics.Descent)/128
	drawer := &font.Drawer{
		Dst:  dst,
		Src:  image.NewUniform(c),
		Face: face,
		Dot:  fixed.Point26_6{X: fixed.Int26_6(left * 64), Y: fixed.Int26_6(baseline * 64)},
	}
	drawer.DrawString(text)
}
