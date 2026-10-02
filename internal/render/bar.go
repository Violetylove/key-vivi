package render

import (
	"errors"
	"image"
	"strings"
)

// ErrNoItems 表示没有任何可显示的内容。
var ErrNoItems = errors.New("render: no items to draw")

// Bar 把队列内容渲染成一张带 alpha 的字幕位图，尺寸按内容自适应。
// scale 是 DPI 缩放（1.0 对应 96dpi），alpha 用于淡入淡出。
func Bar(items []string, th Theme, alpha, scale float64) (*image.RGBA, error) {
	if len(items) == 0 {
		return nil, ErrNoItems
	}
	if scale <= 0 {
		scale = 1
	}
	width, height, err := measure(items, th, scale)
	if err != nil {
		return nil, err
	}
	face, err := faceFor(th.FontSize*scale, 96*scale)
	if err != nil {
		return nil, err
	}
	text := strings.Join(items, th.Separator)
	textWidth, _ := textSize(face, text)

	s := newSurface(width, height)
	s.fillRounded(0, 0, float64(width), float64(height), th.Radius*scale, premultiply(th.BarColor, alpha))
	drawText(s.image(), face, text,
		(float64(width)-textWidth)/2, th.PaddingY*scale, premultiply(th.TextColor, alpha))
	return s.image(), nil
}

// Measure 返回字幕位图的自然尺寸（逻辑尺寸乘以 scale）。
func Measure(items []string, th Theme, scale float64) (int, int, error) {
	if len(items) == 0 {
		return 0, 0, ErrNoItems
	}
	if scale <= 0 {
		scale = 1
	}
	return measure(items, th, scale)
}

func measure(items []string, th Theme, scale float64) (int, int, error) {
	face, err := faceFor(th.FontSize*scale, 96*scale)
	if err != nil {
		return 0, 0, err
	}
	textWidth, textHeight := textSize(face, strings.Join(items, th.Separator))
	width := textWidth + 2*th.PaddingX*scale
	if minWidth := th.MinWidth * scale; width < minWidth {
		width = minWidth
	}
	height := textHeight + 2*th.PaddingY*scale
	return int(width + 0.5), int(height + 0.5), nil
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
