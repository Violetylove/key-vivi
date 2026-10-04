package render

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// TextImage 生成预乘文字快照；动画帧只合成快照，不反复光栅化字体。
// 左右各保留 2px 字形边界，摆放时应从文字原点减去 2px。
func TextImage(text string, th Theme, size, scale float64) (*image.RGBA, error) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	face, err := faceFor(size, 96*scale)
	if err != nil {
		return nil, err
	}
	w, h := textSize(face, text)
	s := newSurface(int(math.Ceil(w))+4, int(math.Ceil(h)))
	drawText(s.image(), face, text, 2, 0, premultiply(th.TextColor, 1))
	return s.image(), nil
}

// KeyImage 缓存一个完整键帽及计数；动画只合成图片，不重复绘制边框和字体。
func KeyImage(token Token, th Theme, plan Plan) (*image.RGBA, error) {
	var badge *image.RGBA
	if token.Count != "" {
		badgeTheme := th
		badgeTheme.TextColor = th.CountColor
		var err error
		badge, err = TextImage(token.Count, badgeTheme, countSize(plan.FontSize), plan.Scale)
		if err != nil {
			return nil, err
		}
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	face, err := faceFor(plan.FontSize, 96*plan.Scale)
	if err != nil {
		return nil, err
	}
	left, top := float64(plan.Inset), float64(plan.Inset)
	height := float64(plan.Height) - 2*top - 2*plan.Scale
	s := newSurface(int(math.Ceil(token.Width))+2*plan.Inset, plan.Height)
	right, bottom := left+token.Width, top+height
	s.fillRounded(left, top+2*plan.Scale, right, bottom+2*plan.Scale, th.Radius*plan.Scale, premultiply(th.ShadowColor, 1))
	s.fillRounded(left, top, right, bottom, th.Radius*plan.Scale, premultiply(th.KeyColor, 1))
	s.strokeRounded(left, top, right, bottom, th.Radius*plan.Scale, plan.Scale, premultiply(th.BorderColor, 1))
	w, h := textSize(face, token.Text)
	contentWidth := w
	if badge != nil {
		contentWidth += token.CountWidth + 8*plan.Scale
	}
	x := left + (token.Width-contentWidth)/2
	drawText(s.image(), face, token.Text, x, top+(height-h)/2, premultiply(th.TextColor, 1))
	if badge != nil {
		badgeX := x + w + 8*plan.Scale
		badgeHeight := float64(badge.Bounds().Dy()) + 6*plan.Scale
		badgeY := top + (height-badgeHeight)/2
		s.fillRounded(badgeX, badgeY, badgeX+token.CountWidth, badgeY+badgeHeight, 4*plan.Scale, premultiply(th.CountBackground, 1))
		origin := image.Pt(int(math.Round(badgeX+(token.CountWidth-float64(badge.Bounds().Dx()))/2)),
			int(math.Round(badgeY+3*plan.Scale)))
		draw.Draw(s.image(), badge.Bounds().Add(origin), badge, badge.Bounds().Min, draw.Over)
	}
	return s.image(), nil
}

// Layer 的透明度衰减整个键帽，包含文字、背景、描边和计数。
type Layer struct {
	Image *image.RGBA
	X, Y  int
	Alpha float64
}

// Frame 在透明画布上合成独立键帽，间距区域不绘制衬底。
func Frame(width, height int, layers []Layer) *image.RGBA {
	return FrameInto(nil, width, height, layers)
}

// FrameInto 在尺寸不变时复用画布；调用方需在下一帧之前消费或复制返回位图。
func FrameInto(dst *image.RGBA, width, height int, layers []Layer) *image.RGBA {
	var s *surface
	if dst == nil || dst.Bounds() != image.Rect(0, 0, width, height) {
		s = newSurface(width, height)
	} else {
		clear(dst.Pix)
		s = &surface{img: dst}
	}
	for _, layer := range layers {
		if layer.Image == nil {
			continue
		}
		opacity := math.Max(0, math.Min(1, layer.Alpha))
		mask := image.NewUniform(color.Alpha{A: uint8(math.Round(opacity * 255))})
		draw.DrawMask(s.image(), layer.Image.Bounds().Add(image.Pt(layer.X, layer.Y)), layer.Image, layer.Image.Bounds().Min,
			mask, image.Point{}, draw.Over)
	}
	return s.image()
}
