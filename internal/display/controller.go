package display

import (
	"key-vivi/internal/keyboard"
	"time"
)

// Controller 由 UI 消息循环单线程持有；工作协程不直接改其状态。
type Controller struct {
	Queue   Queue
	Paused  bool
	state   *keyboard.State
	held    map[uint32]bool
	blocked map[uint32]bool
	cutoff  time.Time
	// FilterExitHotkey 仅在应用成功注册退出热键时启用。
	FilterExitHotkey bool
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
	if !e.When.IsZero() {
		if e.When.Before(c.cutoff) {
			return
		}
		now = e.When
	}
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
	if e.IsDown && (e.VKCode == 0x4B || e.VKCode == 0x51 && c.FilterExitHotkey) && ctrl && alt {
		c.state = keyboard.NewState()
		for vk := range c.held {
			c.blocked[vk] = true
		}
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
		c.Queue.Push(text, now, c.state.RepeatCount())
	}
}

// ResetInput 用物理按下快照恢复状态，并丢弃重置之前排队的输入。
func (c *Controller) ResetInput(held []uint32, cutoff time.Time) {
	c.Queue.Clear()
	c.state = keyboard.NewState()
	clear(c.held)
	clear(c.blocked)
	for _, vk := range held {
		c.held[vk] = true
		c.blocked[vk] = true
	}
	c.cutoff = cutoff
}
