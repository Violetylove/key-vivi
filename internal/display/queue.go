package display

import (
	"strings"
	"time"
)

type Entry struct {
	Text    string
	Expires time.Time
}
type Queue struct{ entries []Entry }

func (q *Queue) Push(text string, now time.Time) {
	q.Expire(now)
	entry := Entry{text, now.Add(1500 * time.Millisecond)}
	// Counts come from the input state machine; only repeated input updates the tail.
	if strings.Contains(text, " ×") && len(q.entries) > 0 && base(q.entries[len(q.entries)-1].Text) == base(text) {
		q.entries[len(q.entries)-1] = entry
	} else {
		q.entries = append(q.entries, entry)
		if len(q.entries) > 5 {
			q.entries = q.entries[len(q.entries)-5:]
		}
	}
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
func (q *Queue) Clear()           { q.entries = nil }
