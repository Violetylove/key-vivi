package app

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"strings"
	"time"

	"key-vivi/internal/display"
	"key-vivi/internal/platform"
	"key-vivi/internal/render"
)

func configFields(c config) []platform.SettingField {
	position := 0
	for i, p := range positions {
		if p == c.Region.Position {
			position = i
		}
	}
	fields := []platform.SettingField{
		{Key: "position", Label: "位置", Value: strconv.Itoa(position), Group: 0, Kind: "position"},
		{Key: "offset_x", Label: "水平偏移（px）", Value: strconv.Itoa(c.Region.OffsetX), Group: 0},
		{Key: "offset_y", Label: "垂直偏移（px）", Value: strconv.Itoa(c.Region.OffsetY), Group: 0},
		{Key: "width", Label: "最大行宽（px）", Value: strconv.Itoa(c.Region.Width), Group: 0},
		{Key: "font", Label: "字号（px）", Value: strconv.Itoa(c.Appearance.FontSize), Group: 1},
		{Key: "text", Label: "文字颜色（#RRGGBB）", Value: c.Appearance.TextColor, Group: 1},
		{Key: "background", Label: "背景颜色（#RRGGBB）", Value: c.Appearance.BackgroundColor, Group: 1},
		{Key: "opacity", Label: "背景不透明度（%）", Value: strconv.Itoa(c.Appearance.BackgroundOpacity), Group: 1},
		{Key: "element_gap", Label: "元素间距（px）", Value: strconv.Itoa(c.Appearance.ElementGap), Group: 1},
		{Key: "row_gap", Label: "行间距（px）", Value: strconv.Itoa(c.Appearance.RowGap), Group: 1},
		{Key: "groups", Label: "最多组数", Value: strconv.Itoa(c.Behavior.MaxGroups), Group: 2},
		{Key: "elements", Label: "每组元素数", Value: strconv.Itoa(c.Behavior.MaxElements), Group: 2},
		{Key: "pause", Label: "分组停顿（ms）", Value: strconv.Itoa(c.Behavior.GroupPause), Group: 2},
		{Key: "hold", Label: "停留时间（ms）", Value: strconv.Itoa(c.Behavior.Hold), Group: 2},
		{Key: "animation", Label: "启用动画", Value: strconv.FormatBool(c.Behavior.Animation), Group: 2, Kind: "bool"},
		{Key: "start_paused", Label: "启动时暂停", Value: strconv.FormatBool(c.Behavior.StartPaused), Group: 2, Kind: "bool"},
	}
	descriptions := map[string]string{
		"position":     "选择整块按键区域在主屏上的位置。",
		"offset_x":     "正数向右，负数向左。",
		"offset_y":     "正数向下，负数向上。",
		"width":        "一行放不下时另起一组。",
		"font":         "按键文字大小，尺寸随屏幕缩放。",
		"text":         "六位十六进制颜色，如 #FFFFFF。",
		"background":   "键帽底色，如 #14181F。",
		"opacity":      "0 为完全透明，100 为不透明。",
		"element_gap":  "同一组内相邻键帽的间隔。",
		"row_gap":      "上下两组之间的距离。",
		"groups":       "同时保留的输入组数，范围 1–6。",
		"elements":     "每组完整输入数量，范围 2–20。",
		"pause":        "停顿达到此时间后，输入进入新组。",
		"hold":         "最后一次输入后，整组保持可见的时间。",
		"animation":    "控制淡入、淡出和组间移动。",
		"start_paused": "下次启动时生效，当前暂停状态不变。",
	}
	for i := range fields {
		fields[i].Description = descriptions[fields[i].Key]
	}
	return fields
}

func configValues(c config) map[string]string {
	values := make(map[string]string)
	for _, f := range configFields(c) {
		values[f.Key] = f.Value
	}
	return values
}

func configFromValues(values map[string]string) (config, error) {
	c := defaultConfig()
	for _, field := range []struct {
		key    string
		target *int
	}{
		{"offset_x", &c.Region.OffsetX}, {"offset_y", &c.Region.OffsetY}, {"width", &c.Region.Width},
		{"font", &c.Appearance.FontSize}, {"opacity", &c.Appearance.BackgroundOpacity}, {"element_gap", &c.Appearance.ElementGap}, {"row_gap", &c.Appearance.RowGap},
		{"groups", &c.Behavior.MaxGroups}, {"elements", &c.Behavior.MaxElements}, {"pause", &c.Behavior.GroupPause}, {"hold", &c.Behavior.Hold},
	} {
		n, err := strconv.Atoi(strings.TrimSpace(values[field.key]))
		if err != nil {
			return c, fmt.Errorf("请为所有数量、尺寸和时间填写整数")
		}
		*field.target = n
	}
	position, err := strconv.Atoi(values["position"])
	if err != nil || position < 0 || position >= len(positions) {
		return c, fmt.Errorf("请选择显示位置")
	}
	c.Region.Position = positions[position]
	c.Appearance.TextColor = strings.ToUpper(strings.TrimSpace(values["text"]))
	c.Appearance.BackgroundColor = strings.ToUpper(strings.TrimSpace(values["background"]))
	for _, field := range []struct {
		key    string
		target *bool
	}{{"animation", &c.Behavior.Animation}, {"start_paused", &c.Behavior.StartPaused}} {
		value, err := strconv.ParseBool(values[field.key])
		if err != nil {
			return c, fmt.Errorf("开关值无效")
		}
		*field.target = value
	}
	return c, c.validate()
}

func regionHeight(c config, th render.Theme, scale float64) (int, error) {
	plan, err := render.Layout([]string{"Mg"}, th, scale, 0)
	if err != nil {
		return 0, err
	}
	return plan.Height*c.Behavior.MaxGroups + int(math.Ceil(th.RowGap*scale))*max(0, c.Behavior.MaxGroups-1), nil
}

// configPreview 使用示例和独立场景，不能影响正式队列与动画。
func configPreview(c config, scale float64, width, height, workWidth, workHeight int) (*image.RGBA, error) {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.RGBA{R: 238, G: 240, B: 244, A: 255}), image.Point{}, draw.Src)
	pad := max(1, int(12*scale))
	deskWidth := min(width/3, int(210*scale))
	factor := math.Min(float64(deskWidth-2*pad)/float64(workWidth), float64(height-2*pad)/float64(workHeight))
	desk := image.Rect(pad, pad, pad+max(1, int(float64(workWidth)*factor)), pad+max(1, int(float64(workHeight)*factor)))
	draw.Draw(img, desk, image.NewUniform(color.RGBA{R: 200, G: 207, B: 218, A: 255}), image.Point{}, draw.Src)
	th := c.theme()
	rh, err := regionHeight(c, th, scale)
	if err != nil {
		return nil, err
	}
	anchor := 0
	for i, p := range positions {
		if p == c.Region.Position {
			anchor = i
		}
	}
	w := min(workWidth, int(float64(c.Region.Width)*scale))
	x, y := platform.RegionPosition(0, 0, workWidth, workHeight, w, min(rh, workHeight), rh, anchor, int(math.Round(float64(c.Region.OffsetX)*scale)), int(math.Round(float64(c.Region.OffsetY)*scale)))
	box := image.Rect(desk.Min.X+int(float64(x)*factor), desk.Min.Y+int(float64(y)*factor), desk.Min.X+int(float64(x+w)*factor), desk.Min.Y+int(float64(y+min(rh, workHeight))*factor))
	draw.Draw(img, box.Intersect(desk), image.NewUniform(color.RGBA{R: 55, G: 117, B: 224, A: 255}), image.Point{}, draw.Src)
	previewScale := scale * math.Min(1, float64(height-2*pad)/float64(rh))
	picture := scene{widthLogical: float64(c.Region.Width), options: c.displayOptions()}
	var visuals []display.Visual
	id := uint64(0)
	for group := 0; group < c.Behavior.MaxGroups; group++ {
		items := [][]string{{"Ctrl+C", "Ctrl+V"}, {"A", "A"}, {"Ctrl+Shift+S ×3"}}[group%3]
		for _, text := range items {
			id++
			visuals = append(visuals, display.Visual{ID: id, GroupID: uint64(group + 1), Text: text, Alpha: 1})
		}
	}
	keys, _, err := picture.draw(visuals, th, previewScale, float64(max(1, width-deskWidth-2*pad)), time.Unix(0, 0))
	if err != nil {
		return nil, err
	}
	draw.Draw(img, image.Rect(deskWidth+pad, pad, deskWidth+pad+keys.Bounds().Dx(), pad+keys.Bounds().Dy()), keys, image.Point{}, draw.Over)
	return img, nil
}
