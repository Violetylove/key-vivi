package ui

import (
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"key-vivi/internal/keyboard"
)

// eventsSeen 是供 KEYVIVI_DEBUG 计数的钩子事件总数；只记数量，不记按了哪些键。
var eventsSeen atomic.Uint64

func processKeys(events <-chan keyboard.Event, done <-chan struct{}, handle func(keyboard.Event)) {
	for {
		select {
		case <-done:
			return
		case event := <-events:
			eventsSeen.Add(1)
			fyne.Do(func() { handle(event) })
		}
	}
}
func expireQueue(done <-chan struct{}, expire func(time.Time)) {
	ticker := time.NewTicker(30 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case now := <-ticker.C:
			fyne.Do(func() { expire(now) })
		}
	}
}
