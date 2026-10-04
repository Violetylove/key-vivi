package display

import (
	"slices"
	"time"
)

// Visual 是一项当前画面，不与逻辑队列的到期删除共用生命周期。
type Visual struct {
	ID      uint64
	GroupID uint64
	Text    string
	Alpha   float64
}

// Animator 由 UI 线程持有；到期快照保留到退场结束，容量淘汰立即移除。
type Animator struct {
	Options *Options
	entries []Entry
}

// Update 同步逻辑队列，连按更新保留入场进度和身份。
func (a *Animator) Update(entries []Entry, now time.Time) {
	kept := a.entries[:0]
	for _, old := range a.entries {
		index := slices.IndexFunc(entries, func(e Entry) bool { return e.ID == old.ID })
		if index >= 0 {
			kept = append(kept, entries[index])
		} else if !now.Before(old.Expires) && now.Before(old.Expires.Add(a.Options.fadeOut())) {
			kept = append(kept, old)
		}
	}
	a.entries = kept
	for _, e := range entries {
		if !now.Before(e.Expires.Add(a.Options.fadeOut())) {
			continue
		}
		if !slices.ContainsFunc(a.entries, func(old Entry) bool { return old.ID == e.ID }) {
			a.entries = append(a.entries, e)
		}
	}
	slices.SortFunc(a.entries, func(x, y Entry) int {
		if x.ID < y.ID {
			return -1
		}
		if x.ID > y.ID {
			return 1
		}
		return 0
	})
	// 退场也占组数，满三个组时整组淘汰，不能只删组内的前几个键。
	groups := 0
	var previous uint64
	cut := 0
	for i := len(a.entries) - 1; i >= 0; i-- {
		if groups == 0 || a.entries[i].GroupID != previous {
			groups++
			previous = a.entries[i].GroupID
		}
		if groups > a.Options.GroupLimit() {
			cut = i + 1
			break
		}
	}
	a.entries = a.entries[cut:]
}

// Frame 按采集时刻推导画面，延迟投递不会重新播放过期输入。
func (a *Animator) Frame(now time.Time) []Visual {
	var frame []Visual
	for _, e := range a.entries {
		if now.Before(e.Expires.Add(a.Options.fadeOut())) {
			alpha := 1.0
			if a.Options.Animated() {
				alpha = Opacity(e.Born, e.Expires, now)
			}
			frame = append(frame, Visual{ID: e.ID, GroupID: e.GroupID, Text: e.Text, Alpha: alpha})
		}
	}
	return frame
}

// Clear 立即丢弃全部画面，供暂停、丢事件恢复和退出使用。
func (a *Animator) Clear() { a.entries = nil }

// Opacity 的保持阶段完整占用 Hold；提示可使用独立到期时刻。
func Opacity(born, expires, now time.Time) float64 {
	if now.Before(born) {
		return 0
	}
	if !now.Before(expires) {
		entered := ease(float64(expires.Sub(born)) / float64(FadeIn))
		return entered * (1 - ease(float64(now.Sub(expires))/float64(FadeOut)))
	}
	if now.Before(born.Add(FadeIn)) {
		return ease(float64(now.Sub(born)) / float64(FadeIn))
	}
	return 1
}

func ease(t float64) float64 {
	if t < 0 {
		return 0
	}
	if t > 1 {
		return 1
	}
	return t * t * (3 - 2*t)
}
