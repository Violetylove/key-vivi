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
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// 按顺序尝试系统字体；全部失败时退回 x/image 自带的位图字体，保证仍有可读输出。
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

// faceCache 按 DPI 缓存 face；DPI 变化（换显示器）时重建。
type faceCache struct {
	mu    sync.Mutex
	dpi   float64
	size  float64
	face  font.Face
	inUse bool
}

var cache faceCache

// faceFor 返回指定字号与 DPI 的字体 face。调用方不要关闭返回的 face。
func faceFor(size, dpi float64) (font.Face, error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.inUse && cache.face != nil && cache.dpi == dpi && cache.size == size {
		return cache.face, nil
	}
	if cache.face != nil {
		cache.face.Close()
		cache.face = nil
		cache.inUse = false
	}
	data, err := systemFontData()
	if err != nil {
		// 位图字体只有一种固定尺寸，仍可保证有输出。
		cache.face = basicfont.Face7x13
		cache.dpi, cache.size, cache.inUse = dpi, size, true
		return cache.face, nil
	}
	parsed, err := parseFont(data)
	if err != nil {
		return nil, fmt.Errorf("parse font: %w", err)
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
		Size:    size,
		DPI:     dpi,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil, fmt.Errorf("new face: %w", err)
	}
	cache.face, cache.dpi, cache.size, cache.inUse = face, dpi, size, true
	return face, nil
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
