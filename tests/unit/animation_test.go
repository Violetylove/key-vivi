package unit_test

import (
	"math"
	"testing"
	"time"

	"key-vivi/internal/display"
)

func TestAnimationKeepsFullHoldThenFadesIndependently(t *testing.T) {
	now := time.Unix(100, 0)
	var q display.Queue
	var a display.Animator
	q.Push("A", now, 1)
	a.Update(q.Entries(), now)
	for _, sample := range []struct {
		ms    int
		alpha float64
	}{{0, 0}, {50, .5}, {100, 1}, {1599, 1}, {1600, 1}, {1750, .5}} {
		at := now.Add(time.Duration(sample.ms) * time.Millisecond)
		q.Expire(at)
		a.Update(q.Entries(), at)
		frame := a.Frame(at)
		if len(frame) != 1 || math.Abs(frame[0].Alpha-sample.alpha) > .001 {
			t.Fatalf("at %dms: %v, want alpha %.2f", sample.ms, frame, sample.alpha)
		}
	}
	q.Push("B", now.Add(1800*time.Millisecond), 1)
	a.Update(q.Entries(), now.Add(1800*time.Millisecond))
	at := now.Add(1900 * time.Millisecond)
	q.Expire(at)
	a.Update(q.Entries(), at)
	if frame := a.Frame(at); len(frame) != 1 || frame[0].Text != "B" || frame[0].Alpha != 1 {
		t.Fatalf("old fade-out hid new input: %v", frame)
	}
	at = now.Add(3700 * time.Millisecond)
	q.Expire(at)
	a.Update(q.Entries(), at)
	if len(a.Frame(at)) != 0 {
		t.Fatal("clock would remain active after all fades")
	}
}

func TestRepeatPreservesIdentityOpacityAndFullRemainingHold(t *testing.T) {
	now := time.Unix(100, 0)
	var q display.Queue
	var a display.Animator
	q.Push("A", now, 1)
	first := q.Entries()[0]
	a.Update(q.Entries(), now)
	q.Push("A", now.Add(50*time.Millisecond), 2)
	entries := q.Entries()
	if len(entries) != 2 || entries[0].ID != first.ID || entries[0].Born != first.Born || entries[1].ID == first.ID || entries[0].Expires != entries[1].Expires {
		t.Fatal(entries)
	}
	a.Update(q.Entries(), now.Add(50*time.Millisecond))
	if got := a.Frame(now.Add(50 * time.Millisecond)); len(got) != 2 || got[0].Alpha != .5 || got[1].Alpha != 0 {
		t.Fatal(got)
	}
	q.Push("A ×3", now.Add(400*time.Millisecond), 3)
	e := q.Entries()[0]
	if e.ID != first.ID || e.Born != first.Born || e.Expires != now.Add(1900*time.Millisecond) {
		t.Fatal(e)
	}
	a.Update(q.Entries(), now.Add(400*time.Millisecond))
	if frame := a.Frame(now.Add(400 * time.Millisecond)); len(frame) != 1 || frame[0].Text != "A ×3" || frame[0].Alpha != 1 {
		t.Fatal("merge restarted fade-in or retained the second snapshot", frame)
	}
	q.Clear()
	q.Push("A", now.Add(time.Second), 1)
	if q.Entries()[0].ID <= first.ID {
		t.Fatal("clear reused a stale animation identity")
	}
}

func TestNewInputDropsOldestExitSnapshotAtVisualCapacity(t *testing.T) {
	now := time.Unix(100, 0)
	var q display.Queue
	q.Fits = func(items []string) bool { return len(items) <= 1 }
	var a display.Animator
	for i, text := range []string{"A", "B", "C"} {
		at := now.Add(time.Duration(i) * 100 * time.Millisecond)
		q.Push(text, at, 1)
		a.Update(q.Entries(), at)
	}
	at := now.Add(1650 * time.Millisecond)
	q.Expire(at)
	a.Update(q.Entries(), at)
	if len(a.Frame(at)) != 3 {
		t.Fatal("expiry discarded the exit snapshot")
	}
	q.Push("F", at, 1)
	a.Update(q.Entries(), at)
	frame := a.Frame(at)
	if len(frame) != 3 || frame[0].Text != "B" || frame[2].Text != "F" {
		t.Fatal(frame)
	}
	a.Clear()
	if len(a.Frame(at)) != 0 {
		t.Fatal("pause retained exit snapshots")
	}
}

func TestDelayedDeliveryDoesNotReplayFullyExpiredInput(t *testing.T) {
	now := time.Unix(100, 0)
	var q display.Queue
	q.Push("A", now, 1)
	var a display.Animator
	a.Update(q.Entries(), now.Add(2*time.Second))
	if len(a.Frame(now.Add(2*time.Second))) != 0 {
		t.Fatal("stale input replayed")
	}
}
