package keyboard

import (
	"fmt"
	"strings"
	"time"
)

type Event struct {
	VKCode uint32
	IsDown bool
}
type State struct {
	held     map[uint32]bool
	pending  map[string]bool
	used     bool
	last     string
	lastTime time.Time
	count    int
}

func NewState() *State {
	return &State{held: make(map[uint32]bool), pending: make(map[string]bool)}
}

func ModifierName(vk uint32) string {
	switch vk {
	case 0x10, 0xA0, 0xA1:
		return "Shift"
	case 0x11, 0xA2, 0xA3:
		return "Ctrl"
	case 0x12, 0xA4, 0xA5:
		return "Alt"
	case 0x5B, 0x5C:
		return "Win"
	}
	return ""
}

var modifierOrder = []string{"Ctrl", "Alt", "Shift", "Win"}

func ordered(names map[string]bool) string {
	var parts []string
	for _, name := range modifierOrder {
		if names[name] {
			parts = append(parts, name)
		}
	}
	return strings.Join(parts, "+")
}
func (s *State) Handle(e Event, now time.Time) string {
	if name := ModifierName(e.VKCode); name != "" {
		if e.IsDown {
			if s.held[e.VKCode] {
				return ""
			}
			if len(s.held) == 0 {
				clear(s.pending)
				s.used = false
			}
			s.held[e.VKCode] = true
			s.pending[name] = true
			return ""
		}
		if !s.held[e.VKCode] {
			return ""
		}
		delete(s.held, e.VKCode)
		if len(s.held) != 0 {
			return ""
		}
		value := ""
		if !s.used {
			value = ordered(s.pending)
		}
		clear(s.pending)
		s.used = false
		if value != "" {
			return s.repeat(value, now)
		}
		return ""
	}
	if !e.IsDown {
		return ""
	}
	name := getKeyName(e.VKCode)
	if name == "" {
		return ""
	}
	names := make(map[string]bool)
	for vk := range s.held {
		names[ModifierName(vk)] = true
	}
	if prefix := ordered(names); prefix != "" {
		name = prefix + "+" + name
		s.used = true
	}
	return s.repeat(name, now)
}
func (s *State) repeat(value string, now time.Time) string {
	if s.last == value && now.Sub(s.lastTime) < 500*time.Millisecond {
		s.count++
	} else {
		s.count = 1
	}
	s.last, s.lastTime = value, now
	if s.count > 1 {
		return fmt.Sprintf("%s ×%d", value, s.count)
	}
	return value
}
func getKeyName(vk uint32) string {
	if name := ModifierName(vk); name != "" {
		return name
	}
	if vk >= 0x30 && vk <= 0x39 || vk >= 0x41 && vk <= 0x5A {
		return string(rune(vk))
	}
	if vk >= 0x70 && vk <= 0x7B {
		return fmt.Sprintf("F%d", vk-0x70+1)
	}
	if vk >= 0x60 && vk <= 0x69 {
		return fmt.Sprintf("Num%d", vk-0x60)
	}
	return map[uint32]string{
		0x20: "Space", 0x0D: "Enter", 0x08: "Backspace", 0x09: "Tab", 0x1B: "Esc",
		0x2E: "Delete", 0x2D: "Insert", 0x24: "Home", 0x23: "End", 0x21: "PgUp", 0x22: "PgDn",
		0x25: "←", 0x26: "↑", 0x27: "→", 0x28: "↓", 0x14: "CapsLock", 0x90: "NumLock",
		0xBA: ";", 0xBB: "=", 0xBC: ",", 0xBD: "-", 0xBE: ".", 0xBF: "/", 0xC0: "`",
		0xDB: "[", 0xDC: "\\", 0xDD: "]", 0xDE: "'", 0x6A: "Num*", 0x6B: "Num+", 0x6D: "Num-", 0x6E: "Num.", 0x6F: "Num/",
	}[vk]
}
