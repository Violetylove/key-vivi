package app

import (
	"time"

	"key-vivi/internal/display"
	"key-vivi/internal/keyboard"
)

// session 的显示状态只由 UI 线程持有，启动提示不占用输入队列槽。
type session struct {
	controller *display.Controller
	animation  display.Animator
	hint       string
	hintBorn   time.Time
	hintUntil  time.Time
}

func newSession() *session { return &session{controller: display.NewController()} }

func (s *session) configure(c config, now time.Time, held []uint32) {
	s.reset(now, held)
	options := c.displayOptions()
	s.controller.Queue.Options, s.animation.Options = options, options
}

func (s *session) showHint(text string, now time.Time, duration time.Duration) {
	s.hint, s.hintBorn, s.hintUntil = text, now, now.Add(duration)
}

func (s *session) input(e keyboard.Event, now time.Time) {
	s.controller.Handle(e, now)
	if !e.When.IsZero() {
		now = e.When
	}
	s.animation.Update(s.controller.Queue.Entries(), now)
	if len(s.controller.Queue.Entries()) > 0 {
		s.hint = ""
	}
}

func (s *session) reset(now time.Time, held []uint32) {
	s.controller.ResetInput(held, now)
	s.animation.Clear()
	s.hint = ""
}

func (s *session) toggle(now time.Time, held []uint32) {
	s.controller.TogglePause()
	s.reset(now, held)
}

func (s *session) items(now time.Time) []string {
	visuals := s.visuals(now)
	items := make([]string, len(visuals))
	for i, item := range visuals {
		items[i] = item.Text
	}
	return items
}

func (s *session) visuals(now time.Time) []display.Visual {
	s.controller.Queue.Expire(now)
	if s.controller.Paused {
		return nil
	}
	entries := s.controller.Queue.Entries()
	s.animation.Update(entries, now)
	visuals := s.animation.Frame(now)
	if len(visuals) == 0 {
		fade := time.Duration(0)
		if s.animation.Options.Animated() {
			fade = display.FadeOut
		}
		if s.hint != "" && now.Before(s.hintUntil.Add(fade)) {
			alpha := 1.0
			if s.animation.Options.Animated() {
				alpha = display.Opacity(s.hintBorn, s.hintUntil, now)
			}
			return []display.Visual{{ID: ^uint64(0), Text: s.hint, Alpha: alpha}}
		}
		return nil
	}
	return visuals
}
