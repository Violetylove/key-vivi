package app

import (
	"image"
	"math"
	"slices"
	"time"

	"key-vivi/internal/display"
	"key-vivi/internal/render"
)

// motion 在新布局抢占旧移动时从当前值续接，不从过期起点重新播放。
type motion struct {
	from, to float64
	start    time.Time
}

func (m motion) value(now time.Time) float64 {
	t := math.Max(0, math.Min(1, float64(now.Sub(m.start))/float64(display.Move)))
	return m.from + (m.to-m.from)*t*t*(3-2*t)
}

func (m *motion) target(value float64, now time.Time, immediate bool) {
	if immediate {
		*m = motion{from: value, to: value, start: now}
		return
	}
	if m.to == value {
		return
	}
	*m = motion{from: m.value(now), to: value, start: now}
}

type tokenKey struct {
	id   uint64
	part int
}
type contentKey struct {
	id    uint64
	group uint64
	text  string
}
type snapshot struct {
	x               motion
	y               motion
	text, count     string
	width, height   int
	fontSize, scale float64
	image           *image.RGBA
}
type paintStamp struct {
	key   tokenKey
	image *image.RGBA
	x, y  int
	alpha uint8
}

// scene 固定行宽和左侧锚点；纵坐标相对底部，画布伸缩不改变键帽的屏幕位置。
type scene struct {
	widthLogical          float64
	options               *display.Options
	content               []contentKey
	theme                 render.Theme
	maxWidth, scale       float64
	canvasWidth           int
	snapshots             map[tokenKey]*snapshot
	lastPaint             []paintStamp
	lastWidth, lastHeight int
	image                 *image.RGBA
	tokens                []render.Token
}

func (s *scene) reset() { *s = scene{widthLogical: s.widthLogical, options: s.options} }

// widthLimit 为窄工作区先减边距，避免固定边距把可读字幕挤成一条竖线。
func widthLimit(workWidth int, scale, minimum float64) float64 {
	margin := math.Min(120*scale, math.Max(0, (float64(workWidth)-minimum*scale)/2))
	return math.Max(1, float64(workWidth)-2*margin)
}

func rowWidth(scale, maxWidth float64) float64 {
	return configuredRowWidth(420, scale, maxWidth)
}

func configuredRowWidth(logical, scale, maxWidth float64) float64 {
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		scale = 1
	}
	if logical <= 0 {
		logical = 420
	}
	width := logical * scale
	if maxWidth > 0 {
		width = math.Min(width, maxWidth)
	}
	return math.Max(1, math.Floor(width))
}

type rowLayout struct {
	id      uint64
	indexes []int
	plan    render.Plan
}

func (s *scene) draw(visuals []display.Visual, th render.Theme, scale, maxWidth float64, now time.Time) (*image.RGBA, bool, error) {
	if len(visuals) == 0 {
		changed := s.image != nil
		s.reset()
		return nil, changed, nil
	}
	content := make([]contentKey, len(visuals))
	texts := make([]string, len(visuals))
	for i, visual := range visuals {
		content[i] = contentKey{visual.ID, visual.GroupID, visual.Text}
		texts[i] = visual.Text
	}
	coordinatesChanged := s.scale != scale || s.maxWidth != maxWidth
	themeChanged := s.theme != th
	first := s.snapshots == nil || coordinatesChanged
	if first {
		s.snapshots = make(map[tokenKey]*snapshot)
	}
	if first || !slices.Equal(content, s.content) || themeChanged {
		var rows []rowLayout
		for i, visual := range visuals {
			id := visual.GroupID
			if id == 0 {
				id = visual.ID
			}
			if len(rows) == 0 || rows[len(rows)-1].id != id {
				rows = append(rows, rowLayout{id: id})
			}
			rows[len(rows)-1].indexes = append(rows[len(rows)-1].indexes, i)
		}
		if len(rows) > s.options.GroupLimit() {
			rows = rows[len(rows)-s.options.GroupLimit():]
		}
		limit := configuredRowWidth(s.widthLogical, scale, maxWidth)
		for i := range rows {
			var items []string
			for _, index := range rows[i].indexes {
				items = append(items, texts[index])
			}
			plan, err := render.Layout(items, th, scale, limit)
			if err != nil {
				return nil, false, err
			}
			for j := range plan.Tokens {
				plan.Tokens[j].Index = rows[i].indexes[plan.Tokens[j].Index]
			}
			rows[i].plan = plan
		}
		s.canvasWidth = int(limit)
		s.content, s.theme, s.scale, s.maxWidth = content, th, scale, maxWidth
		if first || themeChanged {
			s.image = nil
		}
		retained := make(map[tokenKey]*snapshot)
		gap := math.Max(0, th.RowGap*scale)
		goals := make([]float64, len(rows))
		bottom := 0.0
		for i := len(rows) - 1; i >= 0; i-- {
			goals[i] = bottom - float64(rows[i].plan.Height)
			bottom = goals[i] - gap
		}
		// 批量新组从已有组下方依次入场，移动抢占时仍保持组间距。
		edge := math.Inf(-1)
		for _, row := range rows {
			for _, token := range row.plan.Tokens {
				if old := s.snapshots[tokenKey{visuals[token.Index].ID, token.Part}]; old != nil {
					edge = math.Max(edge, old.y.value(now)+float64(old.height))
				}
			}
		}
		s.tokens = nil
		for i, row := range rows {
			plan := row.plan
			movement := motion{}
			found := false
			for _, token := range plan.Tokens {
				if old := s.snapshots[tokenKey{visuals[token.Index].ID, token.Part}]; old != nil {
					movement, found = old.y, true
					break
				}
			}
			if !found {
				origin := goals[i]
				if !first && !math.IsInf(edge, -1) {
					origin = edge + gap
				}
				movement.target(origin, now, true)
				edge = math.Max(edge, origin+float64(plan.Height))
			}
			movement.target(goals[i], now, first || !s.options.Animated())
			// Layout 的行内留白仅用于像素取整，不能随追加输入重新居中。
			shift := float64(plan.Inset) - plan.Tokens[0].X
			for _, token := range plan.Tokens {
				key := tokenKey{visuals[token.Index].ID, token.Part}
				item := s.snapshots[key]
				if item == nil {
					item = &snapshot{}
				}
				item.x.target(token.X+shift-float64(plan.Inset), now, true)
				item.y = movement
				w := int(math.Ceil(token.Width)) + 2*plan.Inset
				if item.image == nil || item.text != token.Text || item.count != token.Count || item.width != w || item.height != plan.Height || item.fontSize != plan.FontSize || item.scale != scale || themeChanged {
					img, err := render.KeyImage(token, th, plan)
					if err != nil {
						return nil, false, err
					}
					item.text, item.count, item.width, item.height = token.Text, token.Count, w, plan.Height
					item.fontSize, item.scale, item.image = plan.FontSize, scale, img
				}
				retained[key] = item
				s.tokens = append(s.tokens, token)
			}
		}
		s.snapshots = retained
	}
	width := s.canvasWidth
	top := 0.0
	for _, item := range s.snapshots {
		y := item.y.value(now)
		top = math.Min(top, y)
	}
	// 新组从底部裁剪边界滑入，不能为入场画布向下扩展而越过工作区。
	height := max(1, -int(math.Floor(top)))
	var stamps []paintStamp
	var layers []render.Layer
	for _, token := range s.tokens {
		visual := visuals[token.Index]
		alpha := visual.Alpha
		quantized := uint8(math.Round(math.Max(0, math.Min(1, alpha)) * 255))
		key := tokenKey{visual.ID, token.Part}
		item := s.snapshots[key]
		x := int(math.Round(item.x.value(now)))
		y := height + int(math.Round(item.y.value(now)))
		stamps = append(stamps, paintStamp{key, item.image, x, y, quantized})
		layers = append(layers, render.Layer{Image: item.image, X: x, Y: y, Alpha: float64(quantized) / 255})
	}
	if s.image != nil && s.lastWidth == width && s.lastHeight == height && slices.Equal(s.lastPaint, stamps) {
		return s.image, false, nil
	}
	s.image = render.FrameInto(s.image, width, height, layers)
	s.lastPaint, s.lastWidth, s.lastHeight = stamps, width, height
	return s.image, true, nil
}
