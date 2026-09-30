package ui

import (
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"key-vivi/internal/keyboard"
)

// Generation checks invalidate expiration callbacks queued before newer input.
func processKeys(events <-chan keyboard.Event, done <-chan struct{}, show func(string), hide func()) {
	state := keyboard.NewState()
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	var expiry <-chan time.Time
	var generation atomic.Uint64
	for {
		select {
		case <-done:
			return
		case event := <-events:
			value := state.Handle(event, time.Now())
			if value == "" {
				continue
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(1500 * time.Millisecond)
			expiry = timer.C
			token := generation.Add(1)
			fyne.Do(func() {
				if generation.Load() == token {
					show(value)
				}
			})
		case <-expiry:
			expiry = nil
			token := generation.Load()
			fyne.Do(func() {
				if generation.Load() == token {
					hide()
				}
			})
		}
	}
}
