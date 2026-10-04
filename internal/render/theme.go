package render

import "image/color"

// Theme 描述键帽外观。所有尺寸都是逻辑像素，绘制时按 DPI 缩放。
// 颜色按直通 alpha 书写（如 rgba(10,13,18,0.75)），内部再转成预乘格式。
type Theme struct {
	TextColor       color.RGBA
	KeyColor        color.RGBA
	BorderColor     color.RGBA
	ShadowColor     color.RGBA
	CountColor      color.RGBA
	CountBackground color.RGBA
	FontSize        float64
	PaddingX        float64
	PaddingY        float64
	Radius          float64
	GroupGap        float64
	RowGap          float64
	// MinWidth 是单个键帽的逻辑宽度下限，极窄工作区会降低下限。
	MinWidth float64
}

// DefaultTheme 返回规格约定的默认外观。
func DefaultTheme() Theme {
	return Theme{
		TextColor:       color.RGBA{R: 0xFF, G: 0xFF, B: 0xFF, A: 0xFF},
		KeyColor:        color.RGBA{R: 20, G: 24, B: 31, A: 224},
		BorderColor:     color.RGBA{R: 90, G: 99, B: 115, A: 180},
		ShadowColor:     color.RGBA{A: 64},
		CountColor:      color.RGBA{R: 219, G: 226, B: 238, A: 255},
		CountBackground: color.RGBA{R: 255, G: 255, B: 255, A: 24},
		FontSize:        18,
		PaddingX:        14,
		PaddingY:        10,
		Radius:          8,
		GroupGap:        8,
		RowGap:          16,
		MinWidth:        44,
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
