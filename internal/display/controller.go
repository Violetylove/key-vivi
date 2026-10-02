package display

import (
	"key-vivi/internal/keyboard"
	"time"
)

// Controller is owned by the Fyne runtime; no worker directly mutates its state.
type Controller struct {
	Queue   Queue
	Paused  bool
	state   *keyboard.State
	held    map[uint32]bool
	blocked map[uint32]bool
}

func NewController() *Controller {
	return &Controller{state: keyboard.NewState(), held: make(map[uint32]bool), blocked: make(map[uint32]bool)}
}
func (c *Controller) TogglePause() {
	c.Paused = !c.Paused
	c.Queue.Clear()
	c.state = keyboard.NewState()
	// Keys held across a pause transition must be released before reuse.
	for vk := range c.held {
		c.blocked[vk] = true
	}
}
func (c *Controller) Handle(e keyboard.Event, now time.Time) {
	if e.IsDown {
		c.held[e.VKCode] = true
	} else {
		delete(c.held, e.VKCode)
	}
	// The registered control shortcut must never enter the display queue.
	ctrl, alt := false, false
	for vk := range c.held {
		switch keyboard.ModifierName(vk) {
		case "Ctrl":
			ctrl = true
		case "Alt":
			alt = true
		}
	}
	if e.VKCode == 0x4B && ctrl && alt {
		c.state = keyboard.NewState()
		return
	}
	if c.Paused {
		if e.IsDown {
			c.blocked[e.VKCode] = true
		} else {
			delete(c.blocked, e.VKCode)
		}
		return
	}
	if c.blocked[e.VKCode] {
		if !e.IsDown {
			delete(c.blocked, e.VKCode)
		}
		return
	}
	// After pause/reset, do not invent modifier presses for still-held keys.
	if text := c.state.Handle(e, now); text != "" {
		c.Queue.Push(text, now)
	}
}
