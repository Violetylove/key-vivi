package unit_test

import (
	"key-vivi/internal/display"
	"key-vivi/internal/keyboard"
	"testing"
	"time"
)

func TestFIFOQueue(t *testing.T) {
	var q display.Queue
	now := time.Now()
	for _, s := range []string{"A", "B", "C", "D", "E", "F"} {
		q.Push(s, now)
	}
	entries := q.Entries()
	if len(entries) != 5 || entries[0].Text != "B" || entries[4].Text != "F" {
		t.Fatal(entries)
	}
	q.Clear()
	q.Push("A", now)
	q.Push("B", now.Add(time.Second))
	q.Expire(now.Add(1500 * time.Millisecond))
	if got := q.Entries(); len(got) != 1 || got[0].Text != "B" {
		t.Fatal(got)
	}
}
func TestQueueRepeatRefresh(t *testing.T) {
	var q display.Queue
	now := time.Now()
	q.Push("A", now)
	q.Push("A ×2", now.Add(100*time.Millisecond))
	if len(q.Entries()) != 1 {
		t.Fatal(q.Entries())
	}
	q.Expire(now.Add(1500 * time.Millisecond))
	if len(q.Entries()) != 1 {
		t.Fatal("repeat failed to refresh expiry")
	}
	q.Expire(now.Add(1600 * time.Millisecond))
	if len(q.Entries()) != 0 {
		t.Fatal("expiry failed")
	}
	q.Push("A", now)
	q.Push("A", now.Add(600*time.Millisecond))
	if len(q.Entries()) != 2 {
		t.Fatal("separate input merged")
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
