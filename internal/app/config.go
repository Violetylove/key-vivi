package app

import (
	"bytes"
	"fmt"
	"image/color"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
	"key-vivi/internal/display"
	"key-vivi/internal/platform"
	"key-vivi/internal/render"
)

type config struct {
	Version    int              `yaml:"version"`
	Region     regionConfig     `yaml:"region"`
	Appearance appearanceConfig `yaml:"appearance"`
	Behavior   behaviorConfig   `yaml:"behavior"`
}
type regionConfig struct {
	Position string `yaml:"position"`
	MarginX  int    `yaml:"margin_x_px"`
	MarginY  int    `yaml:"margin_y_px"`
	Width    int    `yaml:"max_width_px"`
}
type appearanceConfig struct {
	FontSize          int    `yaml:"font_size_px"`
	TextColor         string `yaml:"text_color"`
	BackgroundColor   string `yaml:"background_color"`
	BackgroundOpacity int    `yaml:"background_opacity_percent"`
	ElementGap        int    `yaml:"element_gap_px"`
	RowGap            int    `yaml:"row_gap_px"`
	SettingsTheme     string `yaml:"settings_theme"`
}
type behaviorConfig struct {
	MaxGroups   int  `yaml:"max_groups"`
	GroupPause  int  `yaml:"group_pause_ms"`
	Hold        int  `yaml:"hold_ms"`
	Animation   bool `yaml:"animation"`
	StartPaused bool `yaml:"start_paused"`
}

var positions = []string{"top_left", "top_center", "top_right", "bottom_left", "bottom_center", "bottom_right"}

// 六个可选位置沿用几何层的九宫格编号，不能把底部误映射为中部。
func positionAnchor(position string) int {
	for i, p := range positions {
		if p == position {
			if i >= 3 {
				return i + 3
			}
			return i
		}
	}
	return 6
}

func defaultConfig() config {
	return config{Version: 1,
		Region:     regionConfig{Position: "bottom_left", MarginX: 24, MarginY: 24, Width: 420},
		Appearance: appearanceConfig{18, "#FFFFFF", "#14181F", 88, 8, 16, "mocha"},
		Behavior:   behaviorConfig{3, 700, 1500, true, false}}
}

// regionOffsets 将向内边距换算为几何层偏移；居中不读取水平边距。
func (c config) regionOffsets(scale float64) (int, int) {
	x, y := int(math.Round(float64(c.Region.MarginX)*scale)), int(math.Round(float64(c.Region.MarginY)*scale))
	anchor := positionAnchor(c.Region.Position)
	if anchor%3 == 1 {
		x = 0
	} else if anchor%3 == 2 {
		x = -x
	}
	if anchor >= 6 {
		y = -y
	}
	return x, y
}

func (c config) validate() error {
	if c.Version != 1 {
		return fmt.Errorf("不支持的配置版本：%d", c.Version)
	}
	validPosition := false
	for _, p := range positions {
		validPosition = validPosition || c.Region.Position == p
	}
	if !validPosition {
		return fmt.Errorf("显示位置须为顶部或底部的六个位置之一")
	}
	if c.Appearance.SettingsTheme != "mocha" && c.Appearance.SettingsTheme != "latte" {
		return fmt.Errorf("设置主题须为 mocha 或 latte")
	}
	for _, value := range []struct {
		name             string
		value, low, high int
	}{
		{"水平边距", c.Region.MarginX, 0, 10000}, {"垂直边距", c.Region.MarginY, 0, 10000},
		{"最大行宽", c.Region.Width, 160, 1200}, {"字号", c.Appearance.FontSize, 12, 48},
		{"背景透明度", c.Appearance.BackgroundOpacity, 0, 100}, {"元素间距", c.Appearance.ElementGap, 0, 40},
		{"行间距", c.Appearance.RowGap, 0, 40}, {"最多行数", c.Behavior.MaxGroups, 1, 6},
		{"换行停顿", c.Behavior.GroupPause, 100, 5000},
		{"停留时间", c.Behavior.Hold, 200, 10000},
	} {
		if value.value < value.low || value.value > value.high {
			return fmt.Errorf("%s范围为 %d–%d", value.name, value.low, value.high)
		}
	}
	for _, value := range []string{c.Appearance.TextColor, c.Appearance.BackgroundColor} {
		if _, err := configColor(value); err != nil {
			return err
		}
	}
	return nil
}

func configColor(raw string) (color.RGBA, error) {
	if len(raw) != 7 || raw[0] != '#' {
		return color.RGBA{}, fmt.Errorf("颜色须使用 #RRGGBB 格式")
	}
	n, err := strconv.ParseUint(raw[1:], 16, 32)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("颜色须使用 #RRGGBB 格式")
	}
	return color.RGBA{R: uint8(n >> 16), G: uint8(n >> 8), B: uint8(n), A: 255}, nil
}

func (c config) theme() render.Theme {
	t := render.DefaultTheme()
	t.FontSize, t.GroupGap, t.RowGap = float64(c.Appearance.FontSize), float64(c.Appearance.ElementGap), float64(c.Appearance.RowGap)
	t.TextColor, _ = configColor(c.Appearance.TextColor)
	t.KeyColor, _ = configColor(c.Appearance.BackgroundColor)
	t.KeyColor.A = uint8((c.Appearance.BackgroundOpacity*255 + 50) / 100)
	t.CountColor = t.TextColor
	return t
}

func (c config) displayOptions() *display.Options {
	return &display.Options{MaxGroups: c.Behavior.MaxGroups,
		GroupPause: time.Duration(c.Behavior.GroupPause) * time.Millisecond, Hold: time.Duration(c.Behavior.Hold) * time.Millisecond,
		Animation: c.Behavior.Animation}
}

type configStore struct {
	path     string
	document *yaml.Node
	// replace 可注入失败，验证保存失败不会更换文件和活动配置。
	replace func(string, string) error
}

func executableConfigPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(exe), "keyvivi.yaml"), nil
}

func parseConfig(data []byte) (config, *yaml.Node, error) {
	c := defaultConfig()
	if len(data) > 64*1024 {
		return c, nil, fmt.Errorf("配置文件不能超过 64 KiB")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&c); err != nil {
		return defaultConfig(), nil, fmt.Errorf("YAML 配置错误：%w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return defaultConfig(), nil, fmt.Errorf("配置只能包含一个 YAML 文档")
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return defaultConfig(), nil, err
	}
	var inspect func(*yaml.Node) error
	inspect = func(n *yaml.Node) error {
		// 禁止别名、合并和空值，避免默认值覆盖及字段校验产生歧义。
		if n.Kind == yaml.AliasNode || n.Tag == "!!null" || n.Tag == "!!merge" {
			return fmt.Errorf("配置不支持空值、别名或合并字段")
		}
		for _, child := range n.Content {
			if err := inspect(child); err != nil {
				return err
			}
		}
		return nil
	}
	if len(node.Content) != 1 || node.Content[0].Kind != yaml.MappingNode {
		return defaultConfig(), nil, fmt.Errorf("配置须为键值映射")
	}
	if err := inspect(&node); err != nil {
		return defaultConfig(), nil, err
	}
	if err := c.validate(); err != nil {
		return defaultConfig(), nil, err
	}
	return c, &node, nil
}

func (s *configStore) load() (config, error) {
	f, err := os.Open(s.path)
	if os.IsNotExist(err) {
		c := defaultConfig()
		return c, s.save(c)
	}
	if err != nil {
		return defaultConfig(), err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 64*1024+1))
	if err != nil {
		return defaultConfig(), err
	}
	c, node, err := parseConfig(data)
	if err == nil {
		s.document = node
	}
	return c, err
}

var configComments = map[string]string{
	"version":        "配置版本；尺寸为逻辑像素，时间为毫秒。",
	"region":         "显示区域：位置以主显示器工作区为基准。",
	"position":       "顶部或底部：top_left / top_center / top_right / bottom_left / bottom_center / bottom_right。",
	"margin_x_px":    "距屏幕左右边缘：0–10000；上中、下中忽略，切换位置保留此值。",
	"margin_y_px":    "距屏幕上或下边缘：0–10000；始终向内留白。",
	"max_width_px":   "最大行宽：160–1200。",
	"appearance":     "外观与布局。",
	"settings_theme": "设置窗口配色：mocha（深色）或 latte（浅色）。",
	"font_size_px":   "字号：12–48。", "text_color": "文字颜色：加引号的 #RRGGBB。",
	"background_color": "背景颜色：加引号的 #RRGGBB。", "background_opacity_percent": "背景不透明度：0–100；0为透明。",
	"element_gap_px": "元素间距：0–40。", "row_gap_px": "行间距：0–40。",
	"behavior": "行为与动画。", "max_groups": "最多行数：1–6，包含退场行。",
	"group_pause_ms": "换行停顿：100–5000；不参与组合键识别。",
	"hold_ms":        "完整停留：200–10000；从入场结束后计时。", "animation": "启用淡入、淡出和纵向移动。",
	"start_paused": "启动时暂停；不改变当前暂停状态。",
}

func configNode(c config) (*yaml.Node, error) {
	var root yaml.Node
	if err := root.Encode(c); err != nil {
		return nil, err
	}
	var annotate func(*yaml.Node)
	annotate = func(n *yaml.Node) {
		if n.Kind != yaml.MappingNode {
			return
		}
		for i := 0; i < len(n.Content); i += 2 {
			key, value := n.Content[i], n.Content[i+1]
			key.HeadComment = configComments[key.Value]
			if strings.HasSuffix(key.Value, "color") {
				value.Style = yaml.DoubleQuotedStyle
			}
			annotate(value)
		}
	}
	annotate(&root)
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{&root}}, nil
}

func mergeConfigNode(old, fresh *yaml.Node) *yaml.Node {
	if old == nil {
		return fresh
	}
	copy := *old
	copy.Content = nil
	if old.Kind == yaml.DocumentNode {
		copy.Content = []*yaml.Node{mergeConfigNode(old.Content[0], fresh.Content[0])}
	} else if old.Kind == yaml.MappingNode {
		seen := make(map[string]bool)
		for i := 0; i < len(old.Content); i += 2 {
			for j := 0; j < len(fresh.Content); j += 2 {
				if old.Content[i].Value == fresh.Content[j].Value {
					key := *old.Content[i]
					copy.Content = append(copy.Content, &key, mergeConfigNode(old.Content[i+1], fresh.Content[j+1]))
					seen[key.Value] = true
				}
			}
		}
		for j := 0; j < len(fresh.Content); j += 2 {
			if !seen[fresh.Content[j].Value] {
				copy.Content = append(copy.Content, fresh.Content[j], fresh.Content[j+1])
			}
		}
	} else {
		copy.Kind, copy.Tag, copy.Value = fresh.Kind, fresh.Tag, fresh.Value
		if fresh.Style == yaml.DoubleQuotedStyle {
			copy.Style = fresh.Style
		}
	}
	return &copy
}

func (s *configStore) save(c config) error {
	if err := c.validate(); err != nil {
		return err
	}
	fresh, err := configNode(c)
	if err != nil {
		return err
	}
	node := mergeConfigNode(s.document, fresh)
	var data bytes.Buffer
	encoder := yaml.NewEncoder(&data)
	encoder.SetIndent(2)
	if err := encoder.Encode(node); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".keyvivi-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(data.Bytes()); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	replace := s.replace
	if replace == nil {
		replace = platform.ReplaceConfigFile
	}
	if err = replace(name, s.path); err != nil {
		return err
	}
	s.document = node
	return nil
}
