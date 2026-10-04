package render

import (
	"image"
	"image/color"

	"golang.org/x/image/vector"
)

// TrayIcon 按目标像素尺寸绘制黑白 KV 字标，暂停态反色并保留暂停标记。
// 图形使用路径生成，避免大图缩小后字母笔画模糊，也不依赖系统字体。
func TrayIcon(size int, paused bool) *image.RGBA {
	if size < 16 {
		size = 16
	}
	s := newSurface(size, size)
	scale := float64(size) / 32
	background := color.RGBA{22, 22, 24, 255}
	foreground := color.RGBA{255, 255, 255, 255}
	if paused {
		background = color.RGBA{210, 214, 220, 255}
		foreground = color.RGBA{66, 70, 78, 255}
	}
	s.fillRounded(scale, scale, 31*scale, 31*scale, 6*scale, background)

	// 两个字母共享斜线节奏，保留窄缝以免小尺寸下连成无法辨认的实块。
	paths := [][][2]float32{
		{{4.8, 8.65}, {7.78, 8.65}, {7.78, 13.33}, {12.23, 8.65}, {16.18, 8.65},
			{9.8, 14.73}, {18.6, 23.15}, {14.25, 23.15}, {7.78, 16.08}, {7.78, 23.15}, {4.8, 23.15}},
		{{11.73, 13.45}, {12.98, 12.25}, {15.58, 12.25}, {20.2, 19.8},
			{24.48, 11.88}, {28.35, 11.88}, {21.3, 23.15}, {19.33, 23.15}},
	}
	z := vector.NewRasterizer(size, size)
	for _, path := range paths {
		z.MoveTo(path[0][0]*float32(scale), path[0][1]*float32(scale))
		for _, p := range path[1:] {
			z.LineTo(p[0]*float32(scale), p[1]*float32(scale))
		}
		z.ClosePath()
	}
	z.Draw(s.image(), s.image().Bounds(), image.NewUniform(foreground), image.Point{})
	if paused {
		s.fillRounded(23*scale, 4*scale, 25*scale, 8*scale, 0, foreground)
		s.fillRounded(27*scale, 4*scale, 29*scale, 8*scale, 0, foreground)
	}
	return s.image()
}
