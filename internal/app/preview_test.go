package app

// scene 与 session 未导出，同包预览使用真实装配状态，不用另写一套动画模拟。
import (
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"key-vivi/internal/display"
	"key-vivi/internal/keyboard"
	"key-vivi/internal/render"
)

func TestAnimationPreview(t *testing.T) {
	if os.Getenv("KEYVIVI_ANIMATION_PREVIEW") == "" {
		t.Skip("set KEYVIVI_ANIMATION_PREVIEW=1 to export the real renderer")
	}
	now := time.Unix(100, 0)
	s := newSession()
	var picture scene
	palette := make(color.Palette, 256)
	for i := range palette {
		palette[i] = color.Gray{Y: uint8(i)}
	}
	var animation gif.GIF
	sheet := image.NewRGBA(image.Rect(0, 0, 640, 8*250))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.Gray{Y: 235}), image.Point{}, draw.Src)
	sequence := []struct {
		ms  int
		key uint32
	}{{0, 'A'}, {600, 'B'}, {1300, 'C'}, {1500, 'C'}, {1580, 'C'}, {1720, 'D'}, {1920, 'D'}, {2000, 'D'}}
	event := 0
	row := 0
	for ms := 0; ms <= 4200; ms += 20 {
		at := now.Add(time.Duration(ms) * time.Millisecond)
		for event < len(sequence) && sequence[event].ms <= ms {
			e := sequence[event]
			s.input(keyboard.Event{VKCode: e.key, IsDown: true, When: now.Add(time.Duration(e.ms) * time.Millisecond)}, at)
			event++
		}
		img, _, err := picture.draw(s.visuals(at), render.DefaultTheme(), 1, 600, at)
		if err != nil {
			t.Fatal(err)
		}
		canvas := image.NewRGBA(image.Rect(0, 0, 640, 250))
		draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.Gray{Y: 235}), image.Point{}, draw.Src)
		if img != nil {
			origin := image.Pt((640-img.Bounds().Dx())/2, 230-img.Bounds().Dy())
			draw.Draw(canvas, img.Bounds().Add(origin), img, image.Point{}, draw.Over)
		}
		label := font.Drawer{Dst: canvas, Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P(12, 20)}
		label.DrawString(at.Sub(now).String())
		frame := image.NewPaletted(canvas.Bounds(), palette)
		draw.FloydSteinberg.Draw(frame, frame.Bounds(), canvas, image.Point{})
		animation.Image = append(animation.Image, frame)
		animation.Delay = append(animation.Delay, 2)
		if ms == 60 || ms == 100 || ms == 1560 || ms == 1600 || ms == 1760 || ms == 1960 || ms == 3420 || ms == 3720 {
			draw.Draw(sheet, canvas.Bounds().Add(image.Pt(0, row*250)), canvas, image.Point{}, draw.Src)
			row++
		}
	}
	out := filepath.Join("..", "..", "tests", "artifacts")
	if err := os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(out, "display-animation.gif"))
	if err != nil {
		t.Fatal(err)
	}
	if err := gif.EncodeAll(f, &animation); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	f, err = os.Create(filepath.Join(out, "display-animation-frames.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, sheet); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
	// 同一布局算法输出不同 DPI 的实际像素尺寸，附带长输入与极窄工作区。
	sheet = image.NewRGBA(image.Rect(0, 0, 1100, 6*400))
	draw.Draw(sheet, sheet.Bounds(), image.NewUniform(color.Gray{Y: 235}), image.Point{}, draw.Src)
	for i, scale := range []float64{1, 1.25, 1.5, 2, 2, 2} {
		var sample scene
		limit := 1040.0
		if i >= 4 {
			limit = 180
		}
		visuals := []display.Visual{{ID: 1, Text: "Ctrl+Shift+Backspace", Alpha: 1}, {ID: 2, Text: "A ×3", Alpha: 1}, {ID: 3, Text: "Enter", Alpha: 1}}
		if i == 5 {
			visuals = visuals[:1]
		}
		img, _, err := sample.draw(visuals, render.DefaultTheme(), scale, limit, now)
		if err != nil {
			t.Fatal(err)
		}
		origin := image.Pt(20, i*400+30)
		draw.Draw(sheet, img.Bounds().Add(origin), img, image.Point{}, draw.Over)
	}
	f, err = os.Create(filepath.Join(out, "display-dpi-layout.png"))
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, sheet); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()
}

func TestKeycapStylePreview(t *testing.T) {
	if os.Getenv("KEYVIVI_ANIMATION_PREVIEW") == "" {
		t.Skip("set KEYVIVI_ANIMATION_PREVIEW=1 to export keycap styles")
	}
	cases := []struct {
		label string
		items []string
	}{
		{"Single keys", []string{"A", "B", "Enter", "↓"}},
		{"Shortcut groups", []string{"Ctrl+Shift+S", "Alt+Tab"}},
		{"Two presses", []string{"A", "A"}},
		{"Three presses", []string{"A ×3"}},
		{"Numpad plus", []string{"Ctrl+Num+ ×4", "Backspace"}},
	}
	canvas := image.NewRGBA(image.Rect(0, 0, 1100, len(cases)*100))
	for column, background := range []color.RGBA{{R: 235, G: 238, B: 243, A: 255}, {R: 15, G: 19, B: 27, A: 255}} {
		area := image.Rect(column*550, 0, (column+1)*550, canvas.Bounds().Dy())
		draw.Draw(canvas, area, image.NewUniform(background), image.Point{}, draw.Src)
		labelColor := color.RGBA{R: 93, G: 104, B: 121, A: 255}
		if column == 1 {
			labelColor = color.RGBA{R: 164, G: 178, B: 198, A: 255}
		}
		for row, sample := range cases {
			img, err := render.Bar(sample.items, render.DefaultTheme(), 1, 1)
			if err != nil {
				t.Fatal(err)
			}
			origin := image.Pt(column*550+24, row*100+35)
			draw.Draw(canvas, img.Bounds().Add(origin), img, image.Point{}, draw.Over)
			label := font.Drawer{Dst: canvas, Src: image.NewUniform(labelColor), Face: basicfont.Face7x13, Dot: fixed.P(column*550+26, row*100+21)}
			label.DrawString(sample.label)
		}
	}
	out := filepath.Join("..", "..", "tests", "artifacts", "keycap-preview.png")
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, canvas); err != nil {
		t.Fatal(err)
	}
}

func TestVerticalGroupsPreview(t *testing.T) {
	if os.Getenv("KEYVIVI_ANIMATION_PREVIEW") == "" {
		t.Skip("set KEYVIVI_ANIMATION_PREVIEW=1 to export vertical groups")
	}
	now := time.Unix(100, 0)
	s := newSession()
	s.controller.Queue.Fits = func(items []string) bool {
		plan, err := render.Layout(items, render.DefaultTheme(), 1, 0)
		if err != nil {
			t.Fatal(err)
		}
		return float64(plan.Width) <= rowWidth(1, 600)
	}
	for i, key := range []uint32{'H', 'E', 'L', 'L', 'O'} {
		s.input(keyboard.Event{VKCode: key, IsDown: true, When: now.Add(time.Duration(i) * 50 * time.Millisecond)}, now)
	}
	for _, event := range []struct {
		ms   int
		key  uint32
		down bool
	}{
		{900, 0xA2, true}, {950, 'C', true}, {975, 'C', false}, {1050, 'V', true}, {1075, 'V', false}, {1100, 0xA2, false},
		{1750, 'A', true}, {1800, 'A', true}, {1850, 'A', true},
	} {
		s.input(keyboard.Event{VKCode: event.key, IsDown: event.down, When: now.Add(time.Duration(event.ms) * time.Millisecond)}, now)
	}
	var picture scene
	img, _, err := picture.draw(s.visuals(now.Add(1900*time.Millisecond)), render.DefaultTheme(), 1, 600, now.Add(1900*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	canvas := image.NewRGBA(image.Rect(0, 0, 1280, 260))
	for column, background := range []color.RGBA{{R: 235, G: 238, B: 243, A: 255}, {R: 15, G: 19, B: 27, A: 255}} {
		area := image.Rect(column*640, 0, (column+1)*640, 260)
		draw.Draw(canvas, area, image.NewUniform(background), image.Point{}, draw.Src)
		origin := image.Pt(column*640+20, 240-img.Bounds().Dy())
		draw.Draw(canvas, img.Bounds().Add(origin), img, image.Point{}, draw.Over)
	}
	out := filepath.Join("..", "..", "tests", "artifacts", "vertical-groups-preview.png")
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, canvas); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkSceneHolding(b *testing.B) {
	benchmarkScene(b, false)
}

func BenchmarkSceneAnimated(b *testing.B) {
	benchmarkScene(b, true)
}

func benchmarkScene(b *testing.B, moving bool) {
	now := time.Unix(100, 0)
	visuals := []display.Visual{{ID: 1, Text: "Ctrl+Shift+Backspace", Alpha: 1}, {ID: 2, Text: "A ×3", Alpha: 1}, {ID: 3, Text: "Enter", Alpha: 1}}
	var s scene
	if _, _, err := s.draw(visuals, render.DefaultTheme(), 1, 1000, now); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if moving {
			visuals[0].Alpha = float64(i%100) / 100
		}
		if _, _, err := s.draw(visuals, render.DefaultTheme(), 1, 1000, now.Add(time.Duration(i)*time.Millisecond)); err != nil {
			b.Fatal(err)
		}
	}
}
