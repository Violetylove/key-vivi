package app

// scene 未导出，同包测试验证连续布局抢占、快照复用及暂停清除的装配边界。
import (
	"math"
	"testing"
	"time"

	"key-vivi/internal/display"
	"key-vivi/internal/keyboard"
	"key-vivi/internal/render"
)

func TestSceneHoldingFrameReusesImageAndPauseClearsSnapshots(t *testing.T) {
	var s scene
	now := time.Unix(100, 0)
	visuals := []display.Visual{{ID: 1, Text: "Ctrl+C", Alpha: 1}}
	img, changed, err := s.draw(visuals, render.DefaultTheme(), 1, 800, now)
	if err != nil || !changed || img == nil {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	again, changed, err := s.draw(visuals, render.DefaultTheme(), 1, 800, now.Add(time.Second))
	if err != nil || changed || again != img {
		t.Fatal("holding frame was rasterized again", err)
	}
	img, changed, err = s.draw(nil, render.DefaultTheme(), 1, 800, now.Add(time.Second))
	if err != nil || !changed || img != nil || len(s.snapshots) != 0 {
		t.Fatal("pause left snapshots", err)
	}
}

func TestSceneMergesShortcutRepeatIntoOneElement(t *testing.T) {
	session := newSession()
	var picture scene
	now := time.Unix(100, 0)
	session.input(keyboard.Event{VKCode: 0xA2, IsDown: true}, now)
	for _, offset := range []time.Duration{0, 200 * time.Millisecond} {
		at := now.Add(offset)
		session.input(keyboard.Event{VKCode: 'C', IsDown: true}, at)
		if _, _, err := picture.draw(session.visuals(at), render.DefaultTheme(), 1, 1000, at); err != nil {
			t.Fatal(err)
		}
	}
	if len(picture.snapshots) != 2 {
		t.Fatal("two shortcut groups were merged early")
	}
	id := session.controller.Queue.Entries()[0].ID
	born := session.controller.Queue.Entries()[0].Born
	at := now.Add(400 * time.Millisecond)
	session.input(keyboard.Event{VKCode: 'C', IsDown: true}, at)
	if _, _, err := picture.draw(session.visuals(at), render.DefaultTheme(), 1, 1000, at); err != nil {
		t.Fatal(err)
	}
	if len(picture.snapshots) != 1 || picture.snapshots[tokenKey{id: id}].text != "Ctrl+C" || picture.snapshots[tokenKey{id: id}].count != "×3" || session.controller.Queue.Entries()[0].Born != born {
		t.Fatal("repeat restarted input or left stale shortcut keycaps")
	}
}

func TestSceneBatchedInputDoesNotStackNewGroups(t *testing.T) {
	var picture scene
	now := time.Unix(100, 0)
	visuals := []display.Visual{{ID: 1, Text: "A", Alpha: 1}}
	if _, _, err := picture.draw(visuals, render.DefaultTheme(), 1, 1000, now); err != nil {
		t.Fatal(err)
	}
	visuals = append(visuals, display.Visual{ID: 2, Text: "B", Alpha: 1}, display.Visual{ID: 3, Text: "C", Alpha: 1})
	at := now.Add(time.Second)
	if _, _, err := picture.draw(visuals, render.DefaultTheme(), 1, 1000, at); err != nil {
		t.Fatal(err)
	}
	for ms := 0; ms <= 120; ms += 16 {
		at := now.Add(time.Second + time.Duration(ms)*time.Millisecond)
		for id := uint64(1); id < 3; id++ {
			left := picture.snapshots[tokenKey{id: id}]
			right := picture.snapshots[tokenKey{id: id + 1}]
			if right.y.value(at)-left.y.value(at) < float64(left.height)+15 || right.x.value(at) != left.x.value(at) {
				t.Fatalf("new groups overlap at %dms", ms)
			}
		}
	}
}

func TestSceneLayoutRetargetContinuesCurrentPosition(t *testing.T) {
	var s scene
	th := render.DefaultTheme()
	th.MinWidth = 0
	now := time.Unix(100, 0)
	visuals := []display.Visual{{ID: 1, Text: "Backspace", Alpha: 1}}
	if _, _, err := s.draw(visuals, th, 1, 1000, now); err != nil {
		t.Fatal(err)
	}
	visuals = append(visuals, display.Visual{ID: 2, Text: "Enter", Alpha: 1})
	if _, _, err := s.draw(visuals, th, 1, 1000, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	at := now.Add(time.Second + 60*time.Millisecond)
	key := tokenKey{id: 1}
	want := s.snapshots[key].y.value(at)
	x := s.snapshots[key].x.value(at)
	visuals = append(visuals, display.Visual{ID: 3, Text: "Ctrl+Shift+S", Alpha: 1})
	if _, _, err := s.draw(visuals, th, 1, 1000, at); err != nil {
		t.Fatal(err)
	}
	if math.Abs(s.snapshots[key].y.value(at)-want) > .001 || s.snapshots[key].x.value(at) != x {
		t.Fatal("rapid input restarted motion from an obsolete origin")
	}
	if _, _, err := s.draw(visuals, th, 2, 160, at.Add(20*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if s.image.Bounds().Dx() > 160 {
		t.Fatal("DPI/work-area change retained an oversized canvas")
	}
}

func TestAppendingToCurrentRowKeepsScreenCoordinatesAndCachedPrefix(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		var picture scene
		now := time.Unix(100, 0)
		visuals := []display.Visual{{ID: 1, GroupID: 1, Text: "Ctrl+C", Alpha: 1}}
		img, _, err := picture.draw(visuals, render.DefaultTheme(), scale, 2000, now)
		if err != nil {
			t.Fatal(err)
		}
		key := tokenKey{id: 1}
		prefix := picture.snapshots[key]
		x, y, cached, width := prefix.x.value(now), prefix.y.value(now), prefix.image, img.Bounds().Dx()
		for i, text := range []string{"A", "B", "Enter"} {
			at := now.Add(time.Duration(i+1) * 100 * time.Millisecond)
			visuals = append(visuals, display.Visual{ID: uint64(i + 2), GroupID: 1, Text: text, Alpha: 1})
			img, _, err = picture.draw(visuals, render.DefaultTheme(), scale, 2000, at)
			if err != nil {
				t.Fatal(err)
			}
			current := picture.snapshots[key]
			if current.x.value(at) != x || current.y.value(at) != y || current.image != cached || img.Bounds().Dx() != width {
				t.Fatalf("append moved or rebuilt prefix at scale %v", scale)
			}
		}
	}
}

func TestAppendingDuringVerticalMoveSharesWholeRowMotion(t *testing.T) {
	var picture scene
	now := time.Unix(100, 0)
	visuals := []display.Visual{{ID: 1, GroupID: 1, Text: "A", Alpha: 1}}
	if _, _, err := picture.draw(visuals, render.DefaultTheme(), 1, 800, now); err != nil {
		t.Fatal(err)
	}
	visuals = append(visuals, display.Visual{ID: 2, GroupID: 2, Text: "B", Alpha: 1})
	at := now.Add(time.Second)
	if _, _, err := picture.draw(visuals, render.DefaultTheme(), 1, 800, at); err != nil {
		t.Fatal(err)
	}
	at = at.Add(40 * time.Millisecond)
	visuals = append(visuals, display.Visual{ID: 3, GroupID: 2, Text: "C", Alpha: 1})
	if _, _, err := picture.draw(visuals, render.DefaultTheme(), 1, 800, at); err != nil {
		t.Fatal(err)
	}
	for ms := 0; ms <= 150; ms += 10 {
		time := at.Add(time.Duration(ms) * time.Millisecond)
		if picture.snapshots[tokenKey{id: 2}].y.value(time) != picture.snapshots[tokenKey{id: 3}].y.value(time) {
			t.Fatal("new key did not share row motion")
		}
	}
}

func TestMeasuredWidthWrapDoesNotClipAnyInputPrefix(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		for _, maxWidth := range []float64{80, 280, 1400} {
			s := newSession()
			th := render.DefaultTheme()
			limit := rowWidth(scale, maxWidth)
			s.controller.Queue.Fits = func(items []string) bool {
				plan, err := render.Layout(items, th, scale, 0)
				if err != nil {
					t.Fatal(err)
				}
				return float64(plan.Width) <= limit
			}
			var picture scene
			now := time.Unix(100, 0)
			for i := 0; i < 40; i++ {
				at := now.Add(time.Duration(i) * 50 * time.Millisecond)
				s.input(keyboard.Event{VKCode: uint32('A' + i%26), IsDown: true, When: at}, at)
				visuals := s.visuals(at)
				img, _, err := picture.draw(visuals, th, scale, maxWidth, at)
				if err != nil {
					t.Fatal(err)
				}
				if img.Bounds().Dx() > int(limit) {
					t.Fatal("canvas exceeds row width")
				}
				for _, visual := range visuals {
					if picture.snapshots[tokenKey{id: visual.ID}] == nil {
						t.Fatalf("wrapped input prefix disappeared: scale=%v width=%v id=%d", scale, maxWidth, visual.ID)
					}
				}
			}
		}
	}
}

func TestNarrowWorkAreaReducesMarginsBeforeReadableWidth(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		for _, width := range []int{1, 180, 240, 241, 300, 500, 1920} {
			limit := widthLimit(width, scale, 200)
			if limit > float64(width) || limit < math.Min(float64(width), 200*scale) {
				t.Fatalf("width=%d scale=%v limit=%v", width, scale, limit)
			}
		}
	}
	if widthLimit(1920, 1, 200) != 1680 {
		t.Fatal("regular work area lost default margins")
	}
}
