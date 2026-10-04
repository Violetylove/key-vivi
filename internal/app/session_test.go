package app

// session 未导出，同包测试覆盖提示、暂停与真实输入时间的装配边界。
import (
	"key-vivi/internal/keyboard"
	"slices"
	"testing"
	"time"
)

func TestSessionShowsBothInputsBeforeRepeatCountAppears(t *testing.T) {
	s := newSession()
	now := time.Unix(100, 0)
	for i, want := range [][]string{{"A"}, {"A", "A"}, {"A ×3"}, {"A ×4"}} {
		at := now.Add(time.Duration(i) * 200 * time.Millisecond)
		s.input(keyboard.Event{VKCode: 'A', When: at.Add(-time.Millisecond)}, at)
		s.input(keyboard.Event{VKCode: 'A', IsDown: true, When: at}, at)
		if got := s.items(at.Add(100 * time.Millisecond)); !slices.Equal(got, want) {
			t.Fatalf("input %d: got %v want %v", i+1, got, want)
		}
	}
}

func TestSessionStartsWithoutHintAndExpiresIndependently(t *testing.T) {
	s := newSession()
	now := time.Now()
	if len(s.items(now)) != 0 {
		t.Fatal("empty startup should stay hidden")
	}
	s.input(keyboard.Event{VKCode: 'A', IsDown: true, When: now}, now)
	s.input(keyboard.Event{VKCode: 'B', IsDown: true, When: now.Add(time.Second)}, now.Add(time.Second))
	items := s.items(now.Add(1900 * time.Millisecond))
	if len(items) != 1 || items[0] != "B" {
		t.Fatalf("old deadline removed new input: %v", items)
	}
	if len(s.items(now.Add(2900*time.Millisecond))) != 0 {
		t.Fatal("visuals should hide after the last fade-out")
	}
}

func TestSessionPauseClearsHintAndRejectsBufferedInput(t *testing.T) {
	s := newSession()
	now := time.Now()
	s.showHint("ready", now, 10*time.Second)
	s.toggle(now.Add(time.Second), []uint32{'A'})
	if len(s.items(now.Add(time.Second))) != 0 {
		t.Fatal("pause left startup hint visible")
	}
	s.toggle(now.Add(2*time.Second), []uint32{'A'})
	s.input(keyboard.Event{VKCode: 'B', IsDown: true, When: now.Add(1500 * time.Millisecond)}, now.Add(2*time.Second))
	s.input(keyboard.Event{VKCode: 'A', IsDown: true, When: now.Add(2100 * time.Millisecond)}, now.Add(2100*time.Millisecond))
	if len(s.items(now.Add(2100*time.Millisecond))) != 0 {
		t.Fatal("buffered or held paused input leaked")
	}
	s.input(keyboard.Event{VKCode: 'A', When: now.Add(2200 * time.Millisecond)}, now.Add(2200*time.Millisecond))
	s.input(keyboard.Event{VKCode: 'A', IsDown: true, When: now.Add(2300 * time.Millisecond)}, now.Add(2300*time.Millisecond))
	if items := s.items(now.Add(2300 * time.Millisecond)); len(items) != 1 || items[0] != "A" {
		t.Fatal(items)
	}
}

func TestSessionUsesCaptureTimeWhenUIDeliveryIsDelayed(t *testing.T) {
	s := newSession()
	now := time.Now()
	s.input(keyboard.Event{VKCode: 'A', IsDown: true, When: now}, now.Add(time.Second))
	s.input(keyboard.Event{VKCode: 'A', IsDown: true, When: now.Add(600 * time.Millisecond)}, now.Add(time.Second))
	items := s.items(now.Add(time.Second))
	if len(items) != 2 || items[0] != "A" || items[1] != "A" {
		t.Fatalf("delivery time incorrectly merged keys: %v", items)
	}
	if len(s.items(now.Add(1900*time.Millisecond))) != 2 || len(s.items(now.Add(2500*time.Millisecond))) != 0 {
		t.Fatal("capture time must renew and expire the whole group")
	}
}

func TestSessionOverflowRecoveryBlocksLostModifierRelease(t *testing.T) {
	s := newSession()
	now := time.Now()
	s.input(keyboard.Event{VKCode: 0xa2, IsDown: true, When: now}, now)
	s.reset(now.Add(time.Second), nil)
	s.input(keyboard.Event{VKCode: 'C', IsDown: true, When: now.Add(1100 * time.Millisecond)}, now.Add(1100*time.Millisecond))
	if items := s.items(now.Add(1100 * time.Millisecond)); len(items) != 1 || items[0] != "C" {
		t.Fatalf("lost release left Ctrl stuck: %v", items)
	}
}

func TestReadyHintDurationRejectsInvalidAndUnboundedValues(t *testing.T) {
	for _, raw := range []string{"invalid", "NaN", "+Inf", "-1", "3601"} {
		t.Setenv("KEYVIVI_READY_HINT", raw)
		if got := readyHintDuration(); got != 1500*time.Millisecond {
			t.Fatalf("%q gave %s", raw, got)
		}
	}
	t.Setenv("KEYVIVI_READY_HINT", "0")
	if readyHintDuration() != 0 {
		t.Fatal("zero must disable hint")
	}
	t.Setenv("KEYVIVI_READY_HINT", "2.5")
	if readyHintDuration() != 2500*time.Millisecond {
		t.Fatal("fractional duration must be supported")
	}
}
