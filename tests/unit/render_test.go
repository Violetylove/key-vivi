package unit_test

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"key-vivi/internal/render"
)

func TestBarSizeFollowsContent(t *testing.T) {
	th := render.DefaultTheme()
	shortW, shortH, err := render.Measure([]string{"A"}, th, 1)
	if err != nil {
		t.Fatal(err)
	}
	long := []string{"Ctrl", "Shift", "Backspace", "A ×3", "Enter"}
	longW, longH, err := render.Measure(long, th, 1)
	if err != nil {
		t.Fatal(err)
	}
	if longW <= shortW {
		t.Fatalf("longer content should be wider: short=%d long=%d", shortW, longW)
	}
	if shortW < int(th.MinWidth) {
		t.Fatalf("width %d is below MinWidth %v", shortW, th.MinWidth)
	}
	// 高度只由字号与内边距决定，与内容无关。
	if shortH != longH {
		t.Fatalf("height must not depend on content: %d vs %d", shortH, longH)
	}
}

func TestBarMatchesMeasure(t *testing.T) {
	th := render.DefaultTheme()
	items := []string{"A", "B"}
	wantW, wantH, err := render.Measure(items, th, 1)
	if err != nil {
		t.Fatal(err)
	}
	img, err := render.Bar(items, th, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := img.Bounds().Dx(); got != wantW {
		t.Fatalf("width %d, Measure said %d", got, wantW)
	}
	if got := img.Bounds().Dy(); got != wantH {
		t.Fatalf("height %d, Measure said %d", got, wantH)
	}
}

func TestBarCornersAreTransparent(t *testing.T) {
	th := render.DefaultTheme()
	img, err := render.Bar([]string{"A"}, th, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	for _, corner := range [][2]int{{0, 0}, {b.Dx() - 1, 0}, {0, b.Dy() - 1}, {b.Dx() - 1, b.Dy() - 1}} {
		if _, _, _, a := img.At(corner[0], corner[1]).RGBA(); a != 0 {
			t.Fatalf("corner %v alpha = %d, want 0", corner, a>>8)
		}
	}
	// 右侧内边距处应是不透明的背景，且 alpha 等于 BarColor 的 alpha。
	_, _, _, a := img.At(b.Dx()-3, b.Dy()/2).RGBA()
	if got := a >> 8; got != 191 {
		t.Fatalf("background alpha = %d, want 191", got)
	}
}

func TestBarAlphaScalesWithFade(t *testing.T) {
	th := render.DefaultTheme()
	full, err := render.Bar([]string{"A"}, th, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	half, err := render.Bar([]string{"A"}, th, 0.5, 1)
	if err != nil {
		t.Fatal(err)
	}
	b := full.Bounds()
	_, _, _, fullA := full.At(b.Dx()-3, b.Dy()/2).RGBA()
	_, _, _, halfA := half.At(b.Dx()-3, b.Dy()/2).RGBA()
	if fullA == 0 {
		t.Fatal("full alpha rendered nothing")
	}
	ratio := float64(halfA) / float64(fullA)
	if ratio < 0.4 || ratio > 0.6 {
		t.Fatalf("alpha 0.5 gave ratio %.2f, want about 0.5", ratio)
	}
	zero, err := render.Bar([]string{"A"}, th, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := zero.At(b.Dx()-3, b.Dy()/2).RGBA(); a != 0 {
		t.Fatalf("alpha 0 should be fully transparent, got %d", a>>8)
	}
}

func TestBarScalesWithDPI(t *testing.T) {
	th := render.DefaultTheme()
	w1, h1, err := render.Measure([]string{"A"}, th, 1)
	if err != nil {
		t.Fatal(err)
	}
	w2, h2, err := render.Measure([]string{"A"}, th, 2)
	if err != nil {
		t.Fatal(err)
	}
	if w2 <= w1 || h2 <= h1 {
		t.Fatalf("scale 2 should be larger: %dx%d vs %dx%d", w2, h2, w1, h1)
	}
}

func TestFitDropsOldestFirst(t *testing.T) {
	th := render.DefaultTheme()
	items := []string{"A", "B", "C", "D"}

	if got := render.Fit(items, th, 1, 0); len(got) != len(items) {
		t.Fatalf("maxWidth 0 should keep everything, got %v", got)
	}
	wide, _, err := render.Measure(items, th, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := render.Fit(items, th, 1, float64(wide+10)); len(got) != len(items) {
		t.Fatalf("ample width should keep everything, got %v", got)
	}
	got := render.Fit(items, th, 1, float64(wide-1))
	if len(got) == 0 {
		t.Fatal("Fit returned nothing")
	}
	if got[len(got)-1] != "D" {
		t.Fatalf("newest item must survive, got %v", got)
	}
	if len(got) >= len(items) {
		t.Fatalf("expected items to be dropped, got %v", got)
	}
	// 极窄时只留最新一项。
	if got := render.Fit(items, th, 1, 1); len(got) != 1 || got[0] != "D" {
		t.Fatalf("narrow width should keep only the newest, got %v", got)
	}
}

func TestBarRejectsEmptyInput(t *testing.T) {
	if _, err := render.Bar(nil, render.DefaultTheme(), 1, 1); err != render.ErrNoItems {
		t.Fatalf("want ErrNoItems, got %v", err)
	}
	if _, _, err := render.Measure(nil, render.DefaultTheme(), 1); err != render.ErrNoItems {
		t.Fatalf("want ErrNoItems, got %v", err)
	}
}

// TestRenderDump 在设置 KEYVIVI_RENDER_DUMP 时把字幕画到浅灰底上输出 PNG，
// 便于肉眼确认圆角、半透明与排版；未设置时跳过。
func TestRenderDump(t *testing.T) {
	if os.Getenv("KEYVIVI_RENDER_DUMP") == "" {
		t.Skip("set KEYVIVI_RENDER_DUMP=1 to write the preview image")
	}
	th := render.DefaultTheme()
	items := []string{"Ctrl+C", "A ×3", "Backspace", "↓"}
	img, err := render.Bar(items, th, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	pad := 20
	canvas := image.NewRGBA(image.Rect(0, 0, img.Bounds().Dx()+2*pad, img.Bounds().Dy()+2*pad))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{200, 200, 200, 255}), image.Point{}, draw.Src)
	draw.Draw(canvas, img.Bounds().Add(image.Point{X: pad, Y: pad}), img, image.Point{}, draw.Over)
	out := filepath.Join("..", "artifacts", "render-bar.png")
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := png.Encode(file, canvas); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %s (%dx%d)", out, canvas.Bounds().Dx(), canvas.Bounds().Dy())
}
