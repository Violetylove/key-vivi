package display

import "time"

// Options 在 UI 线程应用配置后保持不变；零指针沿用默认行为。
type Options struct {
	MaxGroups, MaxElements int
	GroupPause, Hold       time.Duration
	Animation              bool
}

func (o *Options) values() Options {
	if o == nil {
		return Options{MaxGroups, MaxElements, GroupPause, Hold, true}
	}
	return *o
}

// GroupLimit 返回包含退场组的容量。
func (o *Options) GroupLimit() int { return o.values().MaxGroups }

// Animated 返回是否播放淡入、淡出和移动。
func (o *Options) Animated() bool { return o.values().Animation }

func (o *Options) fadeIn() time.Duration {
	if !o.Animated() {
		return 0
	}
	return FadeIn
}
func (o *Options) fadeOut() time.Duration {
	if !o.Animated() {
		return 0
	}
	return FadeOut
}
