package render

// 字体加载与 face 缓存未导出，同包测试验证系统字体缺失时的降级路径。
import (
	"math"
	"sync"
	"testing"
)

func TestOutlineFontFallbackScalesAndUsesReadableHints(t *testing.T) {
	t.Setenv("WINDIR", t.TempDir())
	fontOnce = sync.Once{}
	fontData, fontErr = nil, nil
	CloseFonts()
	defer CloseFonts()
	th := DefaultTheme()
	th.MinWidth = 0
	w1, h1, err := Measure([]string{"Ctrl+Shift+Backspace"}, th, 1)
	if err != nil {
		t.Fatal(err)
	}
	w2, h2, err := Measure([]string{"Ctrl+Shift+Backspace"}, th, 2)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(float64(w2)/float64(w1)-2) > .06 || math.Abs(float64(h2)/float64(h1)-2) > .06 {
		t.Fatalf("fallback DPI: %dx%d -> %dx%d", w1, h1, w2, h2)
	}
	if HintText("KeyVivi 已启动", "KeyVivi ready") != "KeyVivi ready" {
		t.Fatal("missing Chinese glyphs were not detected")
	}
}

func TestFontCacheRetainsLabelAndBadgeAndBoundsGrowth(t *testing.T) {
	CloseFonts()
	defer CloseFonts()
	cache.mu.Lock()
	defer cache.mu.Unlock()
	label, err := faceFor(18, 96)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := faceFor(countSize(18), 96); err != nil {
		t.Fatal(err)
	}
	again, err := faceFor(18, 96)
	if err != nil || again != label {
		t.Fatal("badge rendering discarded the label face", err)
	}
	for i := 0; i < 50; i++ {
		if _, err := faceFor(12+float64(i)/10, 120); err != nil {
			t.Fatal(err)
		}
	}
	if len(cache.faces) > 16 {
		t.Fatal("font cache kept growing with new sizes")
	}
}
