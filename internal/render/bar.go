package render

import (
	"errors"
	"image"
	"math"
)

// ErrNoItems 表示没有任何可显示的内容。
var ErrNoItems = errors.New("render: no items to draw")

// Bar 把队列内容渲染成一张带 alpha 的键帽位图，尺寸按内容自适应。
// scale 是 DPI 缩放（1.0 对应 96dpi），alpha 用于淡入淡出。
func Bar(items []string, th Theme, alpha, scale float64) (*image.RGBA, error) {
	plan, err := Layout(items, th, scale, 0)
	if err != nil {
		return nil, err
	}
	var layers []Layer
	for _, token := range plan.Tokens {
		img, err := KeyImage(token, th, plan)
		if err != nil {
			return nil, err
		}
		layers = append(layers, Layer{Image: img, X: int(math.Round(token.X)) - plan.Inset,
			Y: (plan.Height - img.Bounds().Dy()) / 2, Alpha: alpha})
	}
	return Frame(plan.Width, plan.Height, layers), nil
}

// Measure 返回字幕位图的自然尺寸（逻辑尺寸乘以 scale）。
func Measure(items []string, th Theme, scale float64) (int, int, error) {
	return measure(items, th, scale)
}

func measure(items []string, th Theme, scale float64) (int, int, error) {
	plan, err := Layout(items, th, scale, 0)
	return plan.Width, plan.Height, err
}

// Fit 返回能在 maxWidth 内放下的队尾子集：超宽时从最旧的一项开始丢弃，
// 因为最新输入最重要。maxWidth <= 0 或本就放得下时原样返回。
func Fit(items []string, th Theme, scale, maxWidth float64) []string {
	if len(items) == 0 || maxWidth <= 0 {
		return items
	}
	start := 0
	for start < len(items)-1 {
		if width, _, err := measure(items[start:], th, scale); err == nil && float64(width) <= maxWidth {
			break
		}
		start++
	}
	return items[start:]
}
