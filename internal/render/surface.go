package render

import (
	"image"
	"image/color"
	"math"
)

// surface 是一张待提交给分层窗口的 ARGB 画布，内部按预乘 alpha 存储。
type surface struct {
	img *image.RGBA
}

func newSurface(width, height int) *surface {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	return &surface{img: image.NewRGBA(image.Rect(0, 0, width, height))}
}

func (s *surface) image() *image.RGBA { return s.img }

// blend 把预乘颜色按 coverage 叠加到目标像素上（source-over）。
func (s *surface) blend(x, y int, c color.RGBA, coverage float64) {
	if coverage <= 0 {
		return
	}
	if coverage > 1 {
		coverage = 1
	}
	bounds := s.img.Bounds()
	if !(image.Point{X: x, Y: y}).In(bounds) {
		return
	}
	dst := s.img.RGBAAt(x, y)
	srcA := float64(c.A) * coverage
	keep := 1 - srcA/255
	blend := func(src, dst uint8) uint8 {
		v := float64(src)*coverage + float64(dst)*keep
		if v > 255 {
			v = 255
		}
		return uint8(v + 0.5)
	}
	s.img.SetRGBA(x, y, color.RGBA{
		R: blend(c.R, dst.R),
		G: blend(c.G, dst.G),
		B: blend(c.B, dst.B),
		A: blend(c.A, dst.A),
	})
}

// fillRounded 填充一个圆角矩形，边缘按解析覆盖率做抗锯齿。
func (s *surface) fillRounded(left, top, right, bottom, radius float64, c color.RGBA) {
	if right <= left || bottom <= top {
		return
	}
	maxRadius := math.Min((right-left)/2, (bottom-top)/2)
	if radius > maxRadius {
		radius = maxRadius
	}
	for y := int(math.Floor(top)) - 1; y <= int(math.Ceil(bottom))+1; y++ {
		for x := int(math.Floor(left)) - 1; x <= int(math.Ceil(right))+1; x++ {
			coverage := roundedCoverage(float64(x)+0.5, float64(y)+0.5, left, top, right, bottom, radius)
			s.blend(x, y, c, coverage)
		}
	}
}

// strokeRounded 只绘制轮廓，避免描边在键帽中央叠加导致背景透明度改变。
func (s *surface) strokeRounded(left, top, right, bottom, radius, thickness float64, c color.RGBA) {
	if right <= left || bottom <= top || thickness <= 0 {
		return
	}
	radius = math.Min(radius, math.Min((right-left)/2, (bottom-top)/2))
	for y := int(math.Floor(top)) - 1; y <= int(math.Ceil(bottom))+1; y++ {
		for x := int(math.Floor(left)) - 1; x <= int(math.Ceil(right))+1; x++ {
			px, py := float64(x)+.5, float64(y)+.5
			outer := roundedCoverage(px, py, left, top, right, bottom, radius)
			inner := 0.0
			if right-left > 2*thickness && bottom-top > 2*thickness {
				inner = roundedCoverage(px, py, left+thickness, top+thickness, right-thickness, bottom-thickness, math.Max(0, radius-thickness))
			}
			s.blend(x, y, c, math.Max(0, outer-inner))
		}
	}
}

// roundedCoverage 返回像素中心落在圆角矩形内的比例，边界处约 1 像素过渡。
func roundedCoverage(px, py, left, top, right, bottom, radius float64) float64 {
	halfW := (right - left) / 2
	halfH := (bottom - top) / 2
	cx := left + halfW
	cy := top + halfH
	// 标准圆角矩形有向距离场。
	qx := math.Abs(px-cx) - (halfW - radius)
	qy := math.Abs(py-cy) - (halfH - radius)
	outside := math.Hypot(math.Max(qx, 0), math.Max(qy, 0))
	dist := outside + math.Min(math.Max(qx, qy), 0) - radius
	// dist < 0 在内部；用 1 像素宽度做线性过渡。
	coverage := 0.5 - dist
	if coverage < 0 {
		return 0
	}
	if coverage > 1 {
		return 1
	}
	return coverage
}
