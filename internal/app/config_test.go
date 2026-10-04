package app

// 配置模型与保存事务未导出，同包测试覆盖文件、草稿和输入状态的装配边界。
import (
	"errors"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"key-vivi/internal/display"
	"key-vivi/internal/keyboard"
	"key-vivi/internal/platform"
	"key-vivi/internal/render"

	"go.yaml.in/yaml/v3"
)

func TestConfigMissingFieldsAndStrictErrors(t *testing.T) {
	c, _, err := parseConfig([]byte("version: 1\nappearance:\n  font_size_px: 24\nbehavior:\n  animation: false\n"))
	if err != nil || c.Appearance.FontSize != 24 || c.Appearance.SettingsTheme != "mocha" || c.Behavior.Animation || c.Region != defaultConfig().Region || c.Behavior.Hold != 1500 {
		t.Fatalf("partial config: %#v %v", c, err)
	}
	for _, raw := range []string{"", "[]", "region: null", "version: 2", "version: 1\nversion: 1", "unknown: 1", "region:\n  missing: 1", "appearance: [", "behavior:\n  max_groups: 0", "appearance:\n  font_size_px: 200", "appearance:\n  text_color: invalid", "appearance:\n  settings_theme: unknown", "behavior:\n  hold_ms: .nan", "version: 1\n---\nversion: 1", "region: &r {}\nappearance: *r", "appearance:\n  <<: {font_size_px: 20}"} {
		got, node, err := parseConfig([]byte(raw))
		if err == nil || node != nil || got != defaultConfig() {
			t.Fatalf("invalid config accepted: %q %#v %v", raw, got, err)
		}
	}
	if _, _, err := parseConfig([]byte(strings.Repeat("#", 64*1024+1))); err == nil {
		t.Fatal("oversized config accepted")
	}
}

func TestConfigCreationCommentsReloadAndFailedReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "keyvivi.yaml")
	s := &configStore{path: path}
	c, err := s.load()
	if err != nil || c != defaultConfig() {
		t.Fatal(c, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "显示区域") || !strings.Contains(string(data), `text_color: "#FFFFFF"`) {
		t.Fatal("defaults lack comments or quoted colors")
	}
	data = append([]byte("# 我的设置，请保留\n"), data...)
	data = []byte(strings.Replace(string(data), "margin_x_px: 24", "margin_x_px: 24 # 自定义说明", 1))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	s = &configStore{path: path}
	c, err = s.load()
	if err != nil {
		t.Fatal(err)
	}
	c.Region.Position = "top_right"
	c.Region.MarginX = 36
	c.Behavior.Animation = false
	if err := s.save(c); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(path)
	if !strings.Contains(string(saved), "我的设置，请保留") || !strings.Contains(string(saved), "自定义说明") {
		t.Fatal("save discarded comments")
	}
	reloaded, err := (&configStore{path: path}).load()
	if err != nil || reloaded != c {
		t.Fatal(reloaded, err)
	}
	oldDocument := s.document
	s.replace = func(string, string) error { return errors.New("replacement denied") }
	c.Appearance.FontSize = 28
	if err := s.save(c); err == nil {
		t.Fatal("replacement failure reported success")
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(saved) || s.document != oldDocument {
		t.Fatal("failed replacement changed file or model")
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".keyvivi-*.tmp"))
	if len(files) != 0 {
		t.Fatal("temporary file leaked")
	}
}

func TestConfigMarginRange(t *testing.T) {
	for _, raw := range []string{
		"region:\n  margin_x_px: -1\n",
		"region:\n  margin_y_px: -1\n",
		"region:\n  margin_x_px: 10001\n",
		"region:\n  margin_y_px: 10001\n",
	} {
		if _, _, err := parseConfig([]byte(raw)); err == nil {
			t.Fatalf("out-of-range margin accepted: %q", raw)
		}
	}
}

func TestMeasuredRowWidthReplacesElementLimit(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		c := defaultConfig()
		c.Region.Width = 1200
		q := display.Queue{Options: c.displayOptions()}
		q.Fits = func(items []string) bool {
			plan, err := render.Layout(items, c.theme(), scale, 0)
			if err != nil {
				t.Fatal(err)
			}
			return float64(plan.Width) <= float64(c.Region.Width)*scale
		}
		now := time.Unix(100, 0)
		for i := 0; i < 12; i++ {
			q.Push(string(rune('A'+i)), now.Add(time.Duration(i)*time.Millisecond), 1)
		}
		prefix := q.Entries()
		if len(prefix) != 12 || prefix[0].GroupID != prefix[11].GroupID {
			t.Fatal("element count caused a row break", scale, prefix)
		}
		wrapped := false
		for i := 12; i < 32; i++ {
			q.Push("Ctrl+Shift+Backspace", now.Add(time.Duration(i)*time.Millisecond), 1)
			entries := q.Entries()
			if entries[len(entries)-1].GroupID != prefix[0].GroupID {
				// 同行追加会续期，只比较输入身份与内容。
				for j, want := range prefix {
					if entries[j].ID != want.ID || entries[j].Text != want.Text || entries[j].GroupID != want.GroupID {
						t.Fatal("width wrap discarded the row prefix", scale)
					}
				}
				wrapped = true
				break
			}
		}
		if !wrapped {
			t.Fatal("measured row width did not trigger wrapping", scale)
		}
	}
}

func TestInvalidConfigPreservedAndUnwritablePathUsesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keyvivi.yaml")
	bad := []byte("behavior:\n  hold_ms: invalid\n")
	if err := os.WriteFile(path, bad, 0600); err != nil {
		t.Fatal(err)
	}
	c, err := (&configStore{path: path}).load()
	if err == nil || c != defaultConfig() {
		t.Fatal(c, err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(bad) {
		t.Fatal("bad config was overwritten")
	}
	c, err = (&configStore{path: filepath.Join(path, "keyvivi.yaml")}).load()
	if err == nil || c != defaultConfig() {
		t.Fatal("unwritable directory did not return defaults", err)
	}
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0600)
	s := &configStore{path: path}
	if err := s.save(defaultConfig()); err == nil {
		t.Fatal("read-only target unexpectedly replaced")
	}
}

func TestSettingsFieldsRoundTripAndConfiguredRuntime(t *testing.T) {
	c := defaultConfig()
	c.Region = regionConfig{Position: "bottom_right", MarginX: 40, MarginY: 30, Width: 600}
	c.Appearance = appearanceConfig{24, "#11AAFF", "#332211", 60, 4, 10, "latte"}
	c.Behavior = behaviorConfig{2, 300, 800, false, true}
	values := configValues(c)
	got, err := configFromValues(values)
	if err != nil || got != c || len(values) != 16 {
		t.Fatal(got, err)
	}
	for key, bad := range map[string]string{"groups": "7", "hold": "NaN", "text": "#zzzzzz", "position": "9", "animation": "invalid", "settings_theme": "unknown"} {
		values := configValues(c)
		values[key] = bad
		if _, err := configFromValues(values); err == nil {
			t.Fatal("invalid UI draft accepted", key)
		}
	}
	s := newSession()
	now := time.Unix(100, 0)
	s.controller.Paused = true
	s.configure(c, now, []uint32{0xa2})
	if !s.controller.Paused {
		t.Fatal("saving config changed pause state")
	}
	s.toggle(now, nil)
	s.controller.Queue.Fits = func(items []string) bool { return len(items) <= 2 }
	for i, key := range []uint32{'A', 'B', 'C', 'D', 'E'} {
		at := now.Add(time.Duration(i) * 10 * time.Millisecond)
		s.input(keyboard.Event{VKCode: key, IsDown: true, When: at}, at)
	}
	entries := s.controller.Queue.Entries()
	if len(entries) != 3 || entries[0].Text != "C" || entries[2].Text != "E" {
		t.Fatal("configured capacity not applied", entries)
	}
	if visuals := s.visuals(now.Add(50 * time.Millisecond)); len(visuals) != 3 || visuals[0].Alpha != 1 {
		t.Fatal("disabled animation still faded", visuals)
	}
	if len(s.visuals(now.Add(840*time.Millisecond))) != 0 {
		t.Fatal("configured hold or disabled fade-out not applied")
	}
	c.Behavior.Animation = true
	s.configure(c, now.Add(time.Second), []uint32{0xa2})
	s.input(keyboard.Event{VKCode: 'C', IsDown: true, When: now.Add(1010 * time.Millisecond)}, now.Add(1010*time.Millisecond))
	if s.items(now.Add(1200 * time.Millisecond))[0] != "C" {
		t.Fatal("saved config inherited held modifier")
	}
	th := c.theme()
	if th.FontSize != 24 || th.GroupGap != 4 || th.RowGap != 10 || th.KeyColor.A != 153 || th.TextColor.G != 170 {
		t.Fatal("configured theme not applied", th)
	}
}

func TestRegionAnchorsAndBitmapGrowthKeepBaseline(t *testing.T) {
	for i, p := range positions {
		c := defaultConfig()
		c.Region.Position = p
		got, err := configFromValues(configValues(c))
		want := []int{0, 1, 2, 6, 7, 8}[i]
		if err != nil || got != c || positionAnchor(p) != want {
			t.Fatal("position option or geometry mapping", p, got, err)
		}
		for _, scale := range []float64{1, 1.25, 1.5, 2} {
			ox, oy := c.regionOffsets(scale)
			margin := int(24 * scale)
			x, y := platform.RegionPosition(0, 0, 1920, 1040, 420, 180, 180, want, ox, oy)
			wantX := []int{margin, (1920 - 420) / 2, 1920 - 420 - margin}[i%3]
			wantY := margin
			if i >= 3 {
				wantY = 1040 - 180 - margin
			}
			if x != wantX || y != wantY {
				t.Fatal("screen margins do not follow position", p, scale, x, y)
			}
			if i%3 == 1 {
				c.Region.MarginX = 1000
				if centered, _ := c.regionOffsets(scale); centered != 0 {
					t.Fatal("centered position reads horizontal margin", p)
				}
				c.Region.MarginX = 24
			}
		}
	}
	for _, p := range []string{"middle_left", "center", "middle_right"} {
		if _, _, err := parseConfig([]byte("region:\n  position: " + p)); err == nil {
			t.Fatal("removed middle position accepted", p)
		}
	}
	for anchor := 0; anchor < 9; anchor++ {
		x, y := platform.RegionPosition(-1920, 20, 1920, 1000, 420, 50, 180, anchor, 0, 0)
		x2, y2 := platform.RegionPosition(-1920, 20, 1920, 1000, 420, 170, 180, anchor, 0, 0)
		wantX := -1920 + (1920-420)*(anchor%3)/2
		wantBottom := 20 + (1000-180)*(anchor/3)/2 + 180
		if x != wantX || x != x2 || y+50 != wantBottom || y2+170 != wantBottom {
			t.Fatalf("anchor %d shifted: %d,%d -> %d,%d", anchor, x, y, x2, y2)
		}
	}
	for _, offset := range []int{-10000, 10000} {
		x, y := platform.RegionPosition(10, 20, 300, 200, 200, 150, 180, 4, offset, offset)
		if x < 10 || x+200 > 310 || y < 20 || y+150 > 220 {
			t.Fatal("region escaped work area", x, y)
		}
	}
	c := defaultConfig()
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		h, err := regionHeight(c, c.theme(), scale)
		if err != nil || h < 100 {
			t.Fatal(h, err)
		}
		x, y := platform.RegionPosition(0, 0, 1920, 1040, int(420*scale), h, h, 6, int(24*scale), -int(24*scale))
		if x != int(24*scale) || y+h != 1040-int(24*scale) {
			t.Fatal("default bottom-left margin incorrect", scale, x, y)
		}
	}
}

func TestConfigNodeRoundTripPreservesEverySetting(t *testing.T) {
	c := defaultConfig()
	c.Appearance.SettingsTheme = "latte"
	c.Behavior.Animation = false
	c.Behavior.StartPaused = true
	node, err := configNode(c)
	if err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	decoded, _, err := parseConfig(data)
	if err != nil || decoded != c {
		t.Fatal(decoded, err)
	}
	exe, _ := os.Executable()
	path, err := executableConfigPath()
	if err != nil || filepath.Dir(path) != filepath.Dir(exe) {
		t.Fatal("config path depends on working directory", path, err)
	}
}

func TestConfiguredSceneCapacitySurvivesIdleAndAnimationSwitch(t *testing.T) {
	c := defaultConfig()
	c.Behavior.MaxGroups = 6
	c.Behavior.GroupPause = 150
	c.Behavior.Hold = 3000
	c.Behavior.Animation = false
	c.Region.Width = 600
	s := newSession()
	now := time.Unix(100, 0)
	s.configure(c, now, nil)
	picture := scene{widthLogical: 600, options: s.controller.Queue.Options}
	for i := 0; i < 6; i++ {
		at := now.Add(time.Duration(i) * 200 * time.Millisecond)
		s.input(keyboard.Event{VKCode: uint32('A' + i), IsDown: true, When: at}, at)
	}
	img, _, err := picture.draw(s.visuals(now.Add(time.Second)), c.theme(), 1, 1600, now.Add(time.Second))
	if err != nil || len(picture.snapshots) != 6 || img.Bounds().Dx() != 600 {
		t.Fatal("scene kept default capacity or width", err)
	}
	for _, snapshot := range picture.snapshots {
		if snapshot.y.from != snapshot.y.to {
			t.Fatal("disabled movement still animates")
		}
	}
	if _, _, err := picture.draw(nil, c.theme(), 1, 1600, now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	img, _, err = picture.draw([]display.Visual{{ID: 99, Text: "A", Alpha: 1}}, c.theme(), 1, 1600, now.Add(6*time.Second))
	if err != nil || img.Bounds().Dx() != 600 || picture.options.GroupLimit() != 6 {
		t.Fatal("idle reset discarded configuration", err)
	}
}

func TestSettingsPreview(t *testing.T) {
	if os.Getenv("KEYVIVI_SETTINGS_PREVIEW") == "" {
		t.Skip("set KEYVIVI_SETTINGS_PREVIEW=1 to export settings sample preview")
	}
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		c := defaultConfig()
		img, err := configPreview(c, scale, int(810*scale), int(190*scale), 1920, 1040)
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds().Dx() != int(810*scale) {
			t.Fatal("preview DPI size incorrect")
		}
		if scale == 1 {
			path := filepath.Join("..", "..", "tests", "artifacts", "settings-sample-preview.png")
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			f, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			err = png.Encode(f, img)
			closeErr := f.Close()
			if err != nil || closeErr != nil {
				t.Fatal(err, closeErr)
			}
		}
	}
}
