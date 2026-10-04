package unit_test

import (
	"key-vivi/internal/display"
	"key-vivi/internal/keyboard"
	"testing"
	"time"
)

func TestFIFOQueue(t *testing.T) {
	var q display.Queue
	q.Fits = func(items []string) bool { return len(items) <= 2 }
	now := time.Now()
	for _, s := range []string{"A", "B", "C", "D", "E", "F"} {
		q.Push(s, now, 1)
	}
	entries := q.Entries()
	if len(entries) != 6 || entries[0].Text != "A" || entries[5].Text != "F" || entries[0].GroupID != entries[1].GroupID || entries[2].GroupID == entries[0].GroupID {
		t.Fatal(entries)
	}
	q.Push("G", now, 1)
	if got := q.Entries(); len(got) != 5 || got[0].Text != "C" || got[4].Text != "G" {
		t.Fatal("capacity must remove an entire group", got)
	}
	q.Clear()
	q.Push("A", now, 1)
	q.Push("B", now.Add(time.Second), 1)
	q.Expire(now.Add(1600 * time.Millisecond))
	if got := q.Entries(); len(got) != 1 || got[0].Text != "B" {
		t.Fatal(got)
	}
	q.Clear()
	q.Fits = nil
	for _, text := range []string{"A", "B", "C", "D", "E", "F", "G", "Ctrl+Shift+S", "Enter"} {
		q.Push(text, now, 1)
	}
	if got := q.Entries(); len(got) != 9 || got[8].GroupID != got[0].GroupID {
		t.Fatal("element count must not wrap before the width limit", got)
	}
}
func TestQueueRepeatRefresh(t *testing.T) {
	var q display.Queue
	now := time.Now()
	q.Push("A", now, 1)
	q.Push("A", now.Add(100*time.Millisecond), 2)
	q.Push("A ×3", now.Add(200*time.Millisecond), 3)
	if len(q.Entries()) != 1 {
		t.Fatal(q.Entries())
	}
	q.Expire(now.Add(1500 * time.Millisecond))
	if len(q.Entries()) != 1 {
		t.Fatal("repeat failed to refresh expiry")
	}
	q.Expire(now.Add(1700 * time.Millisecond))
	if len(q.Entries()) != 0 {
		t.Fatal("expiry failed")
	}
	q.Push("A", now, 1)
	q.Push("A", now.Add(600*time.Millisecond), 1)
	if len(q.Entries()) != 2 {
		t.Fatal("separate input merged")
	}
}

func TestGroupPauseBoundaryAndWholeGroupLifetime(t *testing.T) {
	var q display.Queue
	now := time.Unix(100, 0)
	q.Push("A", now, 1)
	q.Push("B", now.Add(699*time.Millisecond), 1)
	first := q.Entries()
	if first[0].GroupID != first[1].GroupID || first[0].Expires != first[1].Expires {
		t.Fatal(first)
	}
	q.Push("C", now.Add(1399*time.Millisecond), 1)
	if got := q.Entries(); len(got) != 3 || got[2].GroupID == got[0].GroupID || got[0] != first[0] {
		t.Fatal("700ms must freeze the previous group", got)
	}
	q.Expire(now.Add(2200 * time.Millisecond))
	if len(q.Entries()) != 3 {
		t.Fatal("prefix expired before the rest of its group")
	}
	q.Expire(now.Add(2299 * time.Millisecond))
	if got := q.Entries(); len(got) != 1 || got[0].Text != "C" {
		t.Fatal("group did not expire atomically", got)
	}
}

func TestWidthWrapStartsLocalRepeatWithoutMergingPreviousRow(t *testing.T) {
	var q display.Queue
	q.Fits = func(items []string) bool { return len(items) <= 2 }
	now := time.Unix(100, 0)
	q.Push("B", now, 1)
	q.Push("A", now.Add(10*time.Millisecond), 1)
	old := q.Entries()
	q.Push("A", now.Add(20*time.Millisecond), 2)
	q.Push("A ×3", now.Add(30*time.Millisecond), 3)
	if got := q.Entries(); len(got) != 4 || got[2].Text != "A" || got[3].Text != "A" || got[0] != old[0] || got[1] != old[1] {
		t.Fatal("wrap lost the two visible presses", got)
	}
	q.Push("A ×4", now.Add(40*time.Millisecond), 4)
	if got := q.Entries(); len(got) != 3 || got[2].Text != "A ×3" || got[2].GroupID == got[0].GroupID {
		t.Fatal("repeat crossed row boundary", got)
	}
}

func TestFastRepeatMergeNeverShortensGroupHold(t *testing.T) {
	var q display.Queue
	now := time.Unix(100, 0)
	q.Push("A", now, 1)
	q.Push("A", now.Add(time.Millisecond), 2)
	expires := q.Entries()[0].Expires
	q.Push("A ×3", now.Add(2*time.Millisecond), 3)
	if q.Entries()[0].Expires.Before(expires) {
		t.Fatal("merge shortened the hold")
	}
}

func TestSecondInputRemainsVisibleThenThirdMergesCurrentRun(t *testing.T) {
	for _, modifier := range []uint32{0, 0xA2} {
		c := display.NewController()
		now := time.Unix(100, 0)
		name := "A"
		if modifier != 0 {
			c.Handle(keyboard.Event{VKCode: modifier, IsDown: true}, now)
			name = "Ctrl+A"
		}
		c.Handle(keyboard.Event{VKCode: 'A', IsDown: true}, now)
		first := c.Queue.Entries()[0]
		for i, text := range []string{name, name + " ×3", name + " ×4"} {
			at := now.Add(time.Duration(i+1) * 200 * time.Millisecond)
			c.Handle(keyboard.Event{VKCode: 'A'}, at.Add(-time.Millisecond))
			c.Handle(keyboard.Event{VKCode: 'A', IsDown: true}, at)
			entries := c.Queue.Entries()
			if i == 0 {
				if len(entries) != 2 || entries[0].ID != first.ID || entries[0].Born != first.Born || entries[1].Text != name || entries[1].ID == first.ID || entries[1].Born != at || entries[1].Expires != at.Add(display.FadeIn+display.Hold) || entries[0].Expires != entries[1].Expires || entries[0].GroupID != entries[1].GroupID {
					t.Fatalf("second input was hidden or reset the first: %v", entries)
				}
				continue
			}
			if len(entries) != 1 || entries[0].Text != text || entries[0].ID != first.ID || entries[0].Born != first.Born || entries[0].Expires != at.Add(display.Hold) {
				t.Fatalf("repeat %d: %v", i+2, entries)
			}
		}
		c.Handle(keyboard.Event{VKCode: 'A', IsDown: true}, now.Add(1100*time.Millisecond))
		if got := c.Queue.Entries(); len(got) != 2 || got[1].Text != name || got[1].ID == first.ID {
			t.Fatalf("500ms boundary merged separate input: %v", got)
		}
	}
}

func TestRepeatMergeDoesNotConsumeEarlierSameKey(t *testing.T) {
	c := display.NewController()
	now := time.Unix(100, 0)
	c.Handle(keyboard.Event{VKCode: 'A', IsDown: true}, now)
	first := c.Queue.Entries()[0]
	for _, ms := range []int{600, 800, 1000} {
		c.Handle(keyboard.Event{VKCode: 'A', IsDown: true}, now.Add(time.Duration(ms)*time.Millisecond))
	}
	if got := c.Queue.Entries(); len(got) != 2 || got[0].ID != first.ID || got[0].Text != first.Text || got[0].Born != first.Born || got[1].Text != "A ×3" {
		t.Fatalf("merge consumed an earlier run: %v", got)
	}
}
func TestHeldKeyAcrossPause(t *testing.T) {
	c := display.NewController()
	now := time.Now()
	c.TogglePause()
	c.Handle(keyboard.Event{VKCode: 0x41, IsDown: true}, now)
	c.TogglePause()
	c.Handle(keyboard.Event{VKCode: 0x41, IsDown: true}, now)
	if len(c.Queue.Entries()) != 0 {
		t.Fatal("held paused key leaked after resume")
	}
	c.Handle(keyboard.Event{VKCode: 0x41, IsDown: false}, now)
	c.Handle(keyboard.Event{VKCode: 0x41, IsDown: true}, now)
	if got := c.Queue.Entries(); len(got) != 1 || got[0].Text != "A" {
		t.Fatal(got)
	}
}

func TestPauseAndControlShortcut(t *testing.T) {
	c := display.NewController()
	now := time.Now()
	c.Handle(keyboard.Event{VKCode: 0x41, IsDown: true}, now)
	c.TogglePause()
	if len(c.Queue.Entries()) != 0 {
		t.Fatal("pause did not clear")
	}
	c.Handle(keyboard.Event{VKCode: 0x42, IsDown: true}, now)
	c.TogglePause()
	if len(c.Queue.Entries()) != 0 {
		t.Fatal("paused input leaked")
	}
	for _, e := range []keyboard.Event{
		{VKCode: 0xA2, IsDown: true}, {VKCode: 0xA4, IsDown: true}, {VKCode: 0x4B, IsDown: true},
		{VKCode: 0x4B, IsDown: false}, {VKCode: 0xA4, IsDown: false}, {VKCode: 0xA2, IsDown: false},
	} {
		c.Handle(e, now)
	}
	if len(c.Queue.Entries()) != 0 {
		t.Fatal("control shortcut leaked", c.Queue.Entries())
	}
	c.Handle(keyboard.Event{VKCode: 0x43, IsDown: true}, now)
	if c.Queue.Entries()[0].Text != "C" {
		t.Fatal(c.Queue.Entries())
	}
}

func TestControlShortcutReleaseDoesNotBlockNextOrdinaryK(t *testing.T) {
	c := display.NewController()
	now := time.Now()
	for _, e := range []keyboard.Event{
		{VKCode: 0xa2, IsDown: true}, {VKCode: 0xa4, IsDown: true}, {VKCode: 'K', IsDown: true},
		{VKCode: 'K'}, {VKCode: 0xa4}, {VKCode: 0xa2}, {VKCode: 'K', IsDown: true},
	} {
		c.Handle(e, now)
	}
	if got := c.Queue.Entries(); len(got) != 1 || got[0].Text != "K" {
		t.Fatalf("shortcut release blocked next K: %v", got)
	}
}
