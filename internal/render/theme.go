package render

import "image/color"

// Theme 描述字幕外观。所有尺寸都是逻辑像素，绘制时按 DPI 缩放。
// 颜色按直通 alpha 书写（如 rgba(10,13,18,0.75)），内部再转成预乘格式。
type Theme struct {
	TextColor color.RGBA
	BarColor  color.RGBA
	FontSize  float64
	PaddingX  float64
	PaddingY  float64
	Radius    float64
	Separator string
	// MinWidth 是逻辑像素的宽度下限；宽度上限由调用方通过 Fit 施加。
	MinWidth float64
}

// DefaultTheme 返回规格约定的默认外观。
func DefaultTheme() Theme {
	return Theme{
		TextColor: color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		BarColor:  color.RGBA{R: 10, G: 13, B: 18, A: 191}, // 0.75 × 255
		FontSize:  18,
		PaddingX:  24,
		PaddingY:  12,
		Radius:    8,
		Separator: "  |  ",
		MinWidth:  200,
	}
}

// premultiply 把直通 alpha 的颜色按给定倍率转成 image.RGBA 使用的预乘格式。
// alpha 用于整体淡入淡出，1 表示不额外衰减。
func premultiply(c color.RGBA, alpha float64) color.RGBA {
	a := float64(c.A) / 255 * alpha
	if a <= 0 {
		return color.RGBA{}
	}
	if a > 1 {
		a = 1
	}
	scale := func(v uint8) uint8 {
		return uint8(float64(v) * a)
	}
	return color.RGBA{R: scale(c.R), G: scale(c.G), B: scale(c.B), A: uint8(a * 255)}
}
