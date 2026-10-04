package display

import (
	"fmt"
	"strings"
	"time"
)

// Entry 保留键帽身份与出生时间，同组输入共享到期时刻。
type Entry struct {
	ID      uint64
	GroupID uint64
	Text    string
	Born    time.Time
	Expires time.Time
}

// v1.0 从入场结束后计完整停留，不能用淡入时间挤占停留时间。
const (
	FadeIn     = 100 * time.Millisecond
	Hold       = 1500 * time.Millisecond
	FadeOut    = 300 * time.Millisecond
	Move       = 120 * time.Millisecond
	GroupPause = 700 * time.Millisecond
	MaxGroups  = 3
	// MaxElements 是默认每组容量，组合键只占一个元素。
	MaxElements = 8
)

// Queue 保存输入组；停顿、元素达到上限或宽度不足时另起一组。
type Queue struct {
	// Options 为空时保留默认三组与计时规则。
	Options                 *Options
	entries                 []Entry
	nextID                  uint64
	lastInput               time.Time
	lastBase                string
	lastCount, repeatOffset int
	// Fits 由 UI 提供实际字体宽度判断；单个超宽组合仍交给渲染层降级。
	Fits func([]string) bool
}

// Push 在第三次连按时合并本组输入；前两次各保留一个可见元素。
func (q *Queue) Push(text string, now time.Time, count int) {
	options := q.Options.values()
	q.Expire(now)
	count = max(1, count)
	name := base(text)
	if name != q.lastBase || count <= q.lastCount {
		q.repeatOffset = 0
	}
	start := len(q.entries)
	if start > 0 && now.Sub(q.lastInput) < options.GroupPause {
		group := q.entries[start-1].GroupID
		for start > 0 && q.entries[start-1].GroupID == group {
			start--
		}
	}
	localCount := max(1, count-q.repeatOffset)
	index := len(q.entries) - 1
	merge := localCount > 2 && index >= start && base(q.entries[index].Text) == name
	if merge && localCount == 3 && index > start && base(q.entries[index-1].Text) == name {
		index--
	}
	label := name
	if merge {
		label = fmt.Sprintf("%s ×%d", name, localCount)
	}
	candidate := make([]string, 0, len(q.entries)-start+1)
	end := len(q.entries)
	if merge {
		end = index
	}
	for _, e := range q.entries[start:end] {
		candidate = append(candidate, e.Text)
	}
	candidate = append(candidate, label)
	// 元素数和实测宽度共同限流，长组合提前换行，不能挤掉当前行前缀。
	full := len(candidate) > options.MaxElements || q.Fits != nil && !q.Fits(candidate)
	if start == len(q.entries) || full {
		start, merge, label = len(q.entries), false, name
		q.repeatOffset = count - 1
	}
	if merge {
		q.entries = q.entries[:index+1]
		q.entries[index].Text = label
	} else {
		q.nextID++
		group := q.nextID
		if start < len(q.entries) {
			group = q.entries[start].GroupID
		}
		q.entries = append(q.entries, Entry{ID: q.nextID, GroupID: group, Text: label, Born: now})
	}
	expires := now.Add(options.Hold)
	for _, e := range q.entries[start:] {
		if e.Expires.After(expires) {
			expires = e.Expires
		}
		if entered := e.Born.Add(q.Options.fadeIn() + options.Hold); entered.After(expires) {
			expires = entered
		}
	}
	for i := start; i < len(q.entries); i++ {
		q.entries[i].Expires = expires
	}
	group := q.entries[len(q.entries)-1].GroupID
	groups := 1
	cut := 0
	for i := len(q.entries) - 1; i >= 0; i-- {
		if q.entries[i].GroupID != group {
			groups++
			group = q.entries[i].GroupID
		}
		if groups > options.MaxGroups {
			cut = i + 1
			break
		}
	}
	q.entries = q.entries[cut:]
	q.lastInput, q.lastBase, q.lastCount = now, name, count
}
func base(text string) string { before, _, _ := strings.Cut(text, " ×"); return before }
func (q *Queue) Expire(now time.Time) {
	kept := q.entries[:0]
	for _, e := range q.entries {
		if now.Before(e.Expires) {
			kept = append(kept, e)
		}
	}
	q.entries = kept
}
func (q *Queue) Entries() []Entry { return append([]Entry(nil), q.entries...) }
func (q *Queue) Clear() {
	q.entries = nil
	q.lastInput = time.Time{}
	q.lastBase, q.lastCount, q.repeatOffset = "", 0, 0
}
