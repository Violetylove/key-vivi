package render

import (
	"math"
	"strconv"
	"strings"

	"golang.org/x/image/font"
)

// Token 的 Index 指向原输入，省略与丢项只影响画面，不改变逻辑输入。
type Token struct {
	Index, Part int
	Text        string
	Count       string
	CountWidth  float64
	X, Width    float64
}

// Plan 保存物理像素布局，字号仍用逻辑像素表示。
type Plan struct {
	Width, Height   int
	Inset           int
	FontSize, Scale float64
	Tokens          []Token
}

// Layout 优先保留新项，最新单项超宽时缩小至 12px，再按完整 Unicode 字符省略。
// maxWidth <= 0 表示无上限；窄工作区同时降低最小宽度与留白。
func Layout(items []string, th Theme, scale, maxWidth float64) (Plan, error) {
	if len(items) == 0 {
		return Plan{}, ErrNoItems
	}
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		scale = 1
	}
	size := th.FontSize
	if size <= 0 || math.IsNaN(size) || math.IsInf(size, 0) {
		size = 18
	}
	limit := math.Inf(1)
	if maxWidth > 0 && !math.IsInf(maxWidth, 0) {
		limit = math.Max(1, math.Floor(maxWidth))
	}
	inset := math.Min(math.Ceil(2*scale), math.Max(0, (limit-1)/2))
	inset = math.Floor(inset)
	available := limit - 2*inset
	padX := math.Max(0, th.PaddingX*scale)
	padY := math.Max(0, th.PaddingY*scale)

	cache.mu.Lock()
	defer cache.mu.Unlock()
	groups := make([][]Token, len(items))
	for i, text := range items {
		label, count := splitCount(text)
		groups[i] = []Token{{Index: i, Text: label, Count: count}}
	}
	groupGap := math.Max(0, th.GroupGap*scale)
	var lineHeight float64
	measure := func() error {
		face, err := faceFor(size, 96*scale)
		if err != nil {
			return err
		}
		_, lineHeight = textSize(face, "Mg")
		for _, group := range groups {
			for j := range group {
				token := &group[j]
				token.Text = readableKeys(face, token.Text)
				w, _ := textSize(face, token.Text)
				token.Width = w + 2*padX
				token.CountWidth = 0
			}
		}
		badgeFace, err := faceFor(countSize(size), 96*scale)
		if err != nil {
			return err
		}
		for _, group := range groups {
			for j := range group {
				token := &group[j]
				if token.Count != "" {
					w, _ := textSize(badgeFace, token.Count)
					token.CountWidth = math.Ceil(w) + 10*scale
					token.Width += token.CountWidth + 8*scale
				}
				token.Width = math.Ceil(math.Max(th.MinWidth*scale, token.Width))
			}
		}
		return nil
	}
	if err := measure(); err != nil {
		return Plan{}, err
	}
	groupWidth := func(group []Token) float64 {
		w := 0.0
		for _, token := range group {
			w += token.Width
		}
		return w
	}
	start := 0
	rowWidth := func() float64 {
		w := float64(len(groups)-start-1) * groupGap
		for _, group := range groups[start:] {
			w += groupWidth(group)
		}
		return w
	}
	for start < len(groups)-1 && rowWidth() > available {
		start++
	}
	if rowWidth() > available {
		low, high := math.Min(12, size), size
		// 字体 hinting 会使像素宽度阶梯变化，最终仍需省略检查，不能只靠比例估算。
		for i := 0; i < 10; i++ {
			mid := (low + high) / 2
			size = mid
			if err := measure(); err != nil {
				return Plan{}, err
			}
			if rowWidth() <= available {
				low = mid
			} else {
				high = mid
			}
		}
		size = low
		if err := measure(); err != nil {
			return Plan{}, err
		}
		if rowWidth() > available {
			// 极窄时整组降为一个摘要键帽，不能单独丢掉组合中的修饰键。
			face, err := faceFor(size, 96*scale)
			if err != nil {
				return Plan{}, err
			}
			padding := math.Min(padX, math.Max(0, (available-1)/2))
			text := ellipsize(face, readableKeys(face, items[start]), available-2*padding)
			w, _ := textSize(face, text)
			groups[start] = []Token{{Index: start, Text: text,
				Width: math.Min(available, math.Ceil(math.Max(math.Min(th.MinWidth*scale, available), w+2*padding)))}}
		}
	}
	used := rowWidth()
	width := math.Min(limit, used+2*inset)
	plan := Plan{Width: int(math.Ceil(width)), Height: int(math.Ceil(lineHeight + 2*padY + 2*inset + 2*scale)),
		Inset: int(inset), FontSize: size, Scale: scale}
	if plan.Width < 1 {
		plan.Width = 1
	}
	if plan.Height < 1 {
		plan.Height = 1
	}
	x := (float64(plan.Width) - used) / 2
	for i := start; i < len(groups); i++ {
		if i != start {
			x += groupGap
		}
		for _, token := range groups[i] {
			token.X = x
			plan.Tokens = append(plan.Tokens, token)
			x += token.Width
		}
	}
	return plan, nil
}

func countSize(size float64) float64 { return math.Max(9, size*.72) }

func splitCount(text string) (string, string) {
	count := ""
	if at := strings.LastIndex(text, " ×"); at >= 0 {
		if n, err := strconv.Atoi(text[at+len(" ×"):]); err == nil && n > 2 {
			count, text = text[at+1:], text[:at]
		}
	}
	// 组合名称整体保留，只把计数提取为徽标，Num+ 和字面量 + 无需特殊拆分。
	return text, count
}

func readableKeys(face font.Face, text string) string {
	for _, pair := range [][2]string{{"←", "Left"}, {"↑", "Up"}, {"→", "Right"}, {"↓", "Down"}} {
		for _, r := range pair[0] {
			if _, ok := face.GlyphAdvance(r); !ok {
				text = strings.ReplaceAll(text, pair[0], pair[1])
			}
		}
	}
	return text
}

func ellipsize(face font.Face, text string, width float64) string {
	if w, _ := textSize(face, text); w <= width {
		return text
	}
	suffix := "…"
	if _, ok := face.GlyphAdvance('…'); !ok {
		suffix = "..."
	}
	if w, _ := textSize(face, suffix); w > width {
		return ""
	}
	runes := []rune(text)
	low, high := 0, len(runes)
	for low < high {
		mid := (low + high + 1) / 2
		if w, _ := textSize(face, string(runes[:mid])+suffix); w <= width {
			low = mid
		} else {
			high = mid - 1
		}
	}
	return string(runes[:low]) + suffix
}
