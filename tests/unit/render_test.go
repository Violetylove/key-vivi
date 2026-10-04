package unit_test

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

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

func TestKeycapsKeepFullShortcutAndSeparateOnlyRepeatBadge(t *testing.T) {
	for _, tc := range []struct {
		input  string
		labels []string
		count  string
	}{
		{"Ctrl+Shift+S", []string{"Ctrl+Shift+S"}, ""},
		{"Ctrl+Alt", []string{"Ctrl+Alt"}, ""},
		{"Ctrl+Num+ ×3", []string{"Ctrl+Num+"}, "×3"},
		{"Ctrl++", []string{"Ctrl++"}, ""},
		{"Num+", []string{"Num+"}, ""},
		{"+", []string{"+"}, ""},
	} {
		plan, err := render.Layout([]string{tc.input}, render.DefaultTheme(), 1, 1200)
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Tokens) != len(tc.labels) {
			t.Fatalf("%s: %v", tc.input, plan.Tokens)
		}
		for part, token := range plan.Tokens {
			count := ""
			if part == len(tc.labels)-1 {
				count = tc.count
			}
			if token.Index != 0 || token.Part != part || token.Text != tc.labels[part] || token.Count != count {
				t.Fatalf("%s: %v", tc.input, plan.Tokens)
			}
		}
	}
}

func TestKeycapGroupsKeepTransparentGapsAndIndependentOpacity(t *testing.T) {
	th := render.DefaultTheme()
	plan, err := render.Layout([]string{"Ctrl+C", "A"}, th, 1, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Tokens) != 2 {
		t.Fatal(plan.Tokens)
	}
	var layers []render.Layer
	for _, token := range plan.Tokens {
		img, err := render.KeyImage(token, th, plan)
		if err != nil {
			t.Fatal(err)
		}
		alpha := 1.0
		if token.Index == 0 {
			alpha = .25
		}
		layers = append(layers, render.Layer{Image: img, X: int(token.X) - plan.Inset, Alpha: alpha})
	}
	frame := render.Frame(plan.Width, plan.Height, layers)
	for part, token := range plan.Tokens {
		alpha := frame.RGBAAt(int(token.X)+5, plan.Height/2).A
		if token.Index == 0 && (alpha == 0 || alpha > 65) || token.Index == 1 && alpha < 200 {
			t.Fatalf("part %d has wrong group opacity: %d", part, alpha)
		}
		if part == 0 {
			continue
		}
		previous := plan.Tokens[part-1]
		gap := token.X - previous.X - previous.Width
		want := th.GroupGap
		if gap != want {
			t.Fatalf("gap=%v want=%v", gap, want)
		}
		x := int(previous.X + previous.Width + gap/2)
		if frame.RGBAAt(x, plan.Height/2).A != 0 {
			t.Fatal("gap still has a backdrop or separator")
		}
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
	// 阴影与键帽分别叠加，内侧仍应半透明；不能把整个窗口填成衬底。
	_, _, _, a := img.At(b.Dx()-6, b.Dy()/2).RGBA()
	if got := a >> 8; got < uint32(th.KeyColor.A) || got >= 255 {
		t.Fatalf("key background alpha = %d, want translucent", got)
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
	_, _, _, fullA := full.At(b.Dx()-6, b.Dy()/2).RGBA()
	_, _, _, halfA := half.At(b.Dx()-6, b.Dy()/2).RGBA()
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
	if _, _, _, a := zero.At(b.Dx()-6, b.Dy()/2).RGBA(); a != 0 {
		t.Fatalf("alpha 0 should be fully transparent, got %d", a>>8)
	}
}

func TestBarScalesWithDPI(t *testing.T) {
	th := render.DefaultTheme()
	th.MinWidth = 0
	items := []string{"Ctrl+Shift+Backspace", "A ×3", "Enter"}
	w1, h1, err := render.Measure(items, th, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, scale := range []float64{1.25, 1.5, 2} {
		w2, h2, err := render.Measure(items, th, scale)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(float64(w2)/float64(w1)-scale) > .08 || math.Abs(float64(h2)/float64(h1)-scale) > .08 {
			t.Fatalf("scale %.2f is not linear: %dx%d -> %dx%d", scale, w1, h1, w2, h2)
		}
	}
}

func TestLayoutKeepsNewestReadableAndNeverExceedsWorkArea(t *testing.T) {
	th := render.DefaultTheme()
	texts := []string{"old", "older", strings.Repeat("Ctrl+Shift+长组合键", 12)}
	for _, width := range []float64{1, 32, 80, 180, 360} {
		plan, err := render.Layout(texts, th, 2, width)
		if err != nil {
			t.Fatal(err)
		}
		if float64(plan.Width) > width || plan.Width < 1 {
			t.Fatal("layout exceeds work area", plan)
		}
		last := plan.Tokens[len(plan.Tokens)-1]
		if last.Index != 2 || !utf8.ValidString(last.Text) {
			t.Fatal("lost newest input or split UTF-8", last)
		}
		if last.X < 0 || last.X+last.Width > float64(plan.Width)+.01 {
			t.Fatal("text extends outside layout", last, plan.Width)
		}
		if plan.FontSize < 12 || plan.FontSize > th.FontSize {
			t.Fatal("font shrink exceeded readability bounds", plan.FontSize)
		}
	}
	if texts[2] != strings.Repeat("Ctrl+Shift+长组合键", 12) {
		t.Fatal("layout mutated logical input")
	}
}

func TestFontMeasureDrawAndCloseCanRunConcurrently(t *testing.T) {
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 12; i++ {
				if _, err := render.Bar([]string{"Ctrl+C"}, render.DefaultTheme(), 1, 1+float64(i%4)/4); err != nil {
					t.Error(err)
				}
				render.CloseFonts()
			}
		}()
	}
	wg.Wait()
}

func TestReusedCanvasDoesNotRetainRemovedText(t *testing.T) {
	sprite := image.NewRGBA(image.Rect(0, 0, 16, 16))
	draw.Draw(sprite, sprite.Bounds(), image.White, image.Point{}, draw.Src)
	canvas := render.Frame(100, 40, []render.Layer{{Image: sprite, X: 30, Y: 12, Alpha: 1}})
	reused := render.FrameInto(canvas, 100, 40, nil)
	if reused != canvas {
		t.Fatal("unchanged dimensions allocated another canvas")
	}
	for _, value := range reused.Pix {
		if value != 0 {
			t.Fatal("removed text or background survived the next frame")
		}
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
