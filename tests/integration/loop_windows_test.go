//go:build windows && integration

package integration_test

import (
	"errors"
	"golang.org/x/sys/windows"
	"key-vivi/internal/platform"
	"key-vivi/internal/render"
	"testing"
	"time"
)

func TestLoopWakesAfterIdleAndCleansOnOwnerThread(t *testing.T) {
	for cycle := 0; cycle < 2; cycle++ {
		ready := make(chan *platform.Loop, 1)
		ticks := make(chan uint32, 2)
		cleaned := make(chan uint32, 1)
		finished := make(chan error, 1)
		var owner uint32
		go func() {
			finished <- platform.Run(func(loop *platform.Loop) error {
				owner = windows.GetCurrentThreadId()
				loop.OnCleanup(func() { cleaned <- windows.GetCurrentThreadId() })
				ready <- loop
				return nil
			}, func(time.Time) bool { ticks <- windows.GetCurrentThreadId(); return false })
		}()
		var loop *platform.Loop
		select {
		case loop = <-ready:
		case <-time.After(3 * time.Second):
			t.Fatal("loop startup timed out")
		}
		for i := 0; i < 2; i++ {
			if err := loop.Wake(); err != nil {
				t.Fatal(err)
			}
			select {
			case thread := <-ticks:
				if thread != owner {
					t.Fatal("tick moved off owner thread")
				}
			case <-time.After(3 * time.Second):
				loop.Quit()
				t.Fatal("idle loop did not wake")
			}
		}
		loop.Quit()
		loop.Quit()
		select {
		case err := <-finished:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("loop did not quit")
		}
		if thread := <-cleaned; thread != owner {
			t.Fatal("cleanup moved off owner thread")
		}
		if err := loop.Wake(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoopInitializationFailureStillCleansResources(t *testing.T) {
	want := errors.New("expected initialization failure")
	var order []int
	err := platform.Run(func(loop *platform.Loop) error {
		loop.OnCleanup(func() { order = append(order, 1) })
		loop.OnCleanup(func() { order = append(order, 2) })
		return want
	}, nil)
	if !errors.Is(err, want) || len(order) != 2 || order[0] != 2 || order[1] != 1 {
		t.Fatalf("cleanup=%v err=%v", order, err)
	}
}

func TestLoopClockStopsAndRestartsOnOwnerThread(t *testing.T) {
	ready := make(chan *platform.Loop, 1)
	ticks := make(chan uint32, 8)
	finished := make(chan error, 1)
	var owner uint32
	go func() {
		count := 0
		finished <- platform.Run(func(loop *platform.Loop) error {
			owner = windows.GetCurrentThreadId()
			ready <- loop
			return nil
		}, func(time.Time) bool {
			count++
			ticks <- windows.GetCurrentThreadId()
			return count%3 != 0
		})
	}()
	var loop *platform.Loop
	select {
	case loop = <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("loop startup timed out")
	}
	defer loop.Quit()
	for cycle := 0; cycle < 2; cycle++ {
		if err := loop.Wake(); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 3; i++ {
			select {
			case thread := <-ticks:
				if thread != owner {
					t.Fatal("clock tick moved off owner thread")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("clock did not advance")
			}
		}
		select {
		case <-ticks:
			t.Fatal("stopped clock kept ticking")
		case <-time.After(60 * time.Millisecond):
		}
	}
	// 活跃时钟也必须可退出，不能遗留向已销毁窗口投递消息的协程。
	if err := loop.Wake(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ticks:
	case <-time.After(3 * time.Second):
		t.Fatal("clock did not restart")
	}
	loop.Quit()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("active clock prevented exit")
	}
}

func TestNativeTrayAddUpdateAndDestroy(t *testing.T) {
	err := platform.Run(func(loop *platform.Loop) error {
		defer loop.Quit()
		for i := 0; i < 3; i++ {
			tray, err := platform.NewTray(loop, platform.TrayActions{Icon: render.TrayIcon, Toggle: func() {}, About: func() {}, Exit: loop.Quit})
			if err != nil {
				return err
			}
			if err := tray.SetPaused(true); err != nil {
				tray.Destroy()
				return err
			}
			if err := tray.SetPaused(false); err != nil {
				tray.Destroy()
				return err
			}
			tray.Destroy()
			tray.Destroy()
		}
		return nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}
