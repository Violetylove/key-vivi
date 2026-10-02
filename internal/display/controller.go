package display

import (
	"key-vivi/internal/keyboard"
	"time"
)

// Controller 由 Fyne 运行时单线程持有；工作协程不直接改其状态。
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
	// 跨暂停仍按住的键必须先释放才能再次使用。
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
	// 已注册的控制快捷键绝不能进入显示队列。
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
	// 暂停或重置后，不为仍按住的键补造修饰键。
	if text := c.state.Handle(e, now); text != "" {
		c.Queue.Push(text, now)
	}
}
