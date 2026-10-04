package unit_test

import (
	"key-vivi/internal/keyboard"
	"testing"
	"time"
)

func TestModifierKeys(t *testing.T) {
	for _, vk := range []uint32{0x10, 0x11, 0x12, 0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5, 0x5B, 0x5C} {
		s := keyboard.NewState()
		now := time.Now()
		if got := s.Handle(keyboard.Event{VKCode: vk, IsDown: true}, now); got != "" {
			t.Fatalf("down %x: %s", vk, got)
		}
		if got := s.Handle(keyboard.Event{VKCode: vk, IsDown: false}, now); got != keyboard.ModifierName(vk) {
			t.Fatalf("up %x: %q", vk, got)
		}
	}
}
func TestInputSequences(t *testing.T) {
	cases := []struct {
		name   string
		events []keyboard.Event
		want   []string
	}{
		{"held control", []keyboard.Event{{VKCode: 0xA2, IsDown: true}, {VKCode: 0x43, IsDown: true}, {VKCode: 0x43, IsDown: false}, {VKCode: 0x56, IsDown: true}, {VKCode: 0x56, IsDown: false}, {VKCode: 0xA2, IsDown: false}, {VKCode: 0x41, IsDown: true}}, []string{"", "Ctrl+C", "", "Ctrl+V", "", "", "A"}},
		{"modifiers only", []keyboard.Event{{VKCode: 0xA2, IsDown: true}, {VKCode: 0xA0, IsDown: true}, {VKCode: 0xA2, IsDown: false}, {VKCode: 0xA0, IsDown: false}}, []string{"", "", "", "Ctrl+Shift"}},
		{"both control sides", []keyboard.Event{{VKCode: 0xA2, IsDown: true}, {VKCode: 0xA3, IsDown: true}, {VKCode: 0xA2, IsDown: false}, {VKCode: 0x43, IsDown: true}, {VKCode: 0xA3, IsDown: false}}, []string{"", "", "", "Ctrl+C", ""}},
		{"fixed order", []keyboard.Event{{VKCode: 0xA0, IsDown: true}, {VKCode: 0xA2, IsDown: true}, {VKCode: 0x53, IsDown: true}, {VKCode: 0xA0, IsDown: false}, {VKCode: 0xA2, IsDown: false}}, []string{"", "", "Ctrl+Shift+S", "", ""}},
		{"no sticky modifier", []keyboard.Event{{VKCode: 0xA2, IsDown: true}, {VKCode: 0xA2, IsDown: false}, {VKCode: 0x41, IsDown: true}}, []string{"", "Ctrl", "A"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := keyboard.NewState()
			now := time.Now()
			for i, e := range c.events {
				if got := s.Handle(e, now.Add(time.Duration(i)*time.Second)); got != c.want[i] {
					t.Fatalf("event %d: got %q want %q", i, got, c.want[i])
				}
			}
		})
	}
}
func TestRepeatAndThreshold(t *testing.T) {
	s := keyboard.NewState()
	now := time.Now()
	for i, want := range []string{"A", "A", "A ×3"} {
		if got := s.Handle(keyboard.Event{VKCode: 0x41, IsDown: true}, now.Add(time.Duration(i)*100*time.Millisecond)); got != want {
			t.Fatal(got)
		}
	}
	if got := s.Handle(keyboard.Event{VKCode: 0x41, IsDown: true}, now.Add(700*time.Millisecond)); got != "A" {
		t.Fatal(got)
	}
	if got := s.Handle(keyboard.Event{VKCode: 0x42, IsDown: true}, now.Add(710*time.Millisecond)); got != "B" {
		t.Fatal(got)
	}
	s = keyboard.NewState()
	s.Handle(keyboard.Event{VKCode: 0xA2, IsDown: true}, now)
	if got := s.Handle(keyboard.Event{VKCode: 0x43, IsDown: true}, now.Add(2*time.Second)); got != "Ctrl+C" {
		t.Fatal(got)
	}
	if got := s.Handle(keyboard.Event{VKCode: 0x43, IsDown: true}, now.Add(2100*time.Millisecond)); got != "Ctrl+C" {
		t.Fatal(got)
	}
	if got := s.Handle(keyboard.Event{VKCode: 0x43, IsDown: true}, now.Add(2200*time.Millisecond)); got != "Ctrl+C ×3" {
		t.Fatal(got)
	}
}
