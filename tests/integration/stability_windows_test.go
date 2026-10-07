//go:build windows && integration

package integration_test

// 稳定性测量针对无竞态插桩的实际 exe；注入只在专用窗口拥有前台时进行。
import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"key-vivi/internal/keyboard"
	"key-vivi/internal/platform"
)

type stabilitySample struct {
	Phase            string  `json:"phase"`
	ElapsedSeconds   float64 `json:"elapsed_seconds"`
	CPUSeconds       float64 `json:"cpu_seconds"`
	WorkingSetBytes  uint64  `json:"working_set_bytes"`
	PrivateBytes     uint64  `json:"private_bytes"`
	Handles          uint32  `json:"handles"`
	GDI              uint32  `json:"gdi"`
	USER             uint32  `json:"user"`
	KeyboardEvents   uint64  `json:"keyboard_events"`
	SentKeys         uint64  `json:"sent_keys"`
	ReceivedKeys     uint64  `json:"received_keys"`
	SystemKeys       uint64  `json:"system_keys"`
	IMECompositions  uint64  `json:"ime_compositions"`
	ForegroundPauses uint64  `json:"foreground_pauses"`
	OverlayVisible   bool    `json:"overlay_visible"`
}

type stabilityMemory struct {
	size, faults                                                                                                              uint32
	peakWorkingSet, workingSet, quotaPeakPaged, quotaPaged, quotaPeakNonPaged, quotaNonPaged, pagefile, peakPagefile, private uintptr
}

func executableResources(process windows.Handle) (stabilitySample, error) {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(process, &creation, &exit, &kernel, &user); err != nil {
		return stabilitySample{}, err
	}
	ticks := func(value windows.Filetime) uint64 { return uint64(value.HighDateTime)<<32 | uint64(value.LowDateTime) }
	sample := stabilitySample{CPUSeconds: float64(ticks(kernel)+ticks(user)) / 1e7}
	memory := stabilityMemory{size: uint32(unsafe.Sizeof(stabilityMemory{}))}
	if ret, _, err := windows.NewLazyDLL("psapi.dll").NewProc("GetProcessMemoryInfo").Call(uintptr(process), uintptr(unsafe.Pointer(&memory)), uintptr(memory.size)); ret == 0 {
		return sample, err
	}
	sample.WorkingSetBytes, sample.PrivateBytes = uint64(memory.workingSet), uint64(memory.private)
	if ret, _, err := windows.NewLazyDLL("kernel32.dll").NewProc("GetProcessHandleCount").Call(uintptr(process), uintptr(unsafe.Pointer(&sample.Handles))); ret == 0 {
		return sample, err
	}
	gdi, _, _ := acceptanceUser32.NewProc("GetGuiResources").Call(uintptr(process), 0)
	userCount, _, _ := acceptanceUser32.NewProc("GetGuiResources").Call(uintptr(process), 1)
	if gdi == 0 || userCount == 0 {
		return sample, fmt.Errorf("GetGuiResources returned no resources for the live executable UI")
	}
	sample.GDI, sample.USER = uint32(gdi), uint32(userCount)
	return sample, nil
}

func TestExecutableStability(t *testing.T) {
	if os.Getenv("KEYVIVI_MEASURE_STABILITY") != "1" {
		t.Skip("set KEYVIVI_MEASURE_STABILITY=1 and KEYVIVI_EXE for explicit desktop stability measurement")
	}
	seconds := 600
	if raw := os.Getenv("KEYVIVI_STABILITY_SECONDS"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 5 || value > 3600 {
			t.Fatal("KEYVIVI_STABILITY_SECONDS must be 5..3600; acceptance requires 600 seconds per phase")
		}
		seconds = value
	}
	phases := []string{"idle", "rapid"}
	if phase := os.Getenv("KEYVIVI_STABILITY_PHASE"); phase != "" {
		if phase != "idle" && phase != "rapid" {
			t.Fatal("KEYVIVI_STABILITY_PHASE must be idle or rapid; omit it to measure both")
		}
		phases = []string{phase}
	}
	p := startAcceptanceExecutable(t, false)
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(p.command.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(process)
	artifactDir := os.Getenv("KEYVIVI_ARTIFACT_DIR")
	if artifactDir == "" {
		artifactDir = t.TempDir()
	}
	file, err := os.Create(filepath.Join(artifactDir, "stability.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	t.Logf("stability measurement: seconds_per_phase=%d logical_processors=%d; KEYVIVI_READY_HINT=0, diagnostics disabled", seconds, runtime.NumCPU())
	events := make(chan keyboard.Event, 1024)
	stopHook, err := platform.StartKeyboardHook(events)
	if err != nil {
		t.Fatal(err)
	}
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		for range events {
		}
	}()
	defer func() { stopHook(); close(events); <-drainDone }()
	for _, phase := range phases {
		var sink *acceptanceInputSink
		if phase == "rapid" {
			sink = newAcceptanceInputSink(t)
			defer sink.close()
		}
		begin := time.Now()
		baseline := platform.HookEventCount()
		var sent, foregroundPauses uint64
		var first, last stabilitySample
		var samples uint64
		nextSample := begin
		ticker := time.NewTicker(time.Second / 30)
		deadline := begin.Add(time.Duration(seconds) * time.Second)
		for now := range ticker.C {
			select {
			case err := <-p.done:
				ticker.Stop()
				t.Fatalf("executable exited during %s: %v", phase, err)
			default:
			}
			if phase == "rapid" && now.Before(deadline) {
				if sink.isForeground() && acceptanceModifiersReleased() {
					if err := acceptanceSendKey(uint16('A' + sent%26)); err != nil {
						ticker.Stop()
						t.Fatal(err)
					}
					sent++
				} else {
					foregroundPauses++
				}
			}
			if !now.Before(nextSample) || !now.Before(deadline) {
				if err := pingExecutable(p.windows.message); err != nil {
					ticker.Stop()
					t.Fatal(err)
				}
				sample, err := executableResources(process)
				if err != nil {
					ticker.Stop()
					t.Fatal(err)
				}
				sample.Phase, sample.ElapsedSeconds = phase, time.Since(begin).Seconds()
				sample.KeyboardEvents, sample.SentKeys, sample.ForegroundPauses = platform.HookEventCount()-baseline, sent, foregroundPauses
				visible, _, _ := acceptanceUser32.NewProc("IsWindowVisible").Call(p.windows.overlay)
				sample.OverlayVisible = visible != 0
				if phase == "rapid" && sample.ElapsedSeconds > 1 && !sample.OverlayVisible {
					ticker.Stop()
					t.Fatal("rapid keyboard input did not keep the executable overlay visible")
				}
				if sink != nil {
					sample.ReceivedKeys = sink.received.Load()
					sample.SystemKeys = sink.systemKeys.Load()
					sample.IMECompositions = sink.imeCompositions.Load()
				}
				if err := encoder.Encode(sample); err != nil {
					ticker.Stop()
					t.Fatal(err)
				}
				if samples == 0 {
					first = sample
				}
				last, samples = sample, samples+1
				if samples == 1 || samples%6 == 0 || !now.Before(deadline) {
					t.Logf("phase=%s elapsed=%.1fs cpu=%.3fs working_set=%d private=%d handles=%d GDI=%d USER=%d sent=%d received=%d focus_pauses=%d", phase, sample.ElapsedSeconds, sample.CPUSeconds, sample.WorkingSetBytes, sample.PrivateBytes, sample.Handles, sample.GDI, sample.USER, sent, sample.ReceivedKeys, foregroundPauses)
				}
				nextSample = now.Add(10 * time.Second)
			}
			if !now.Before(deadline) {
				break
			}
		}
		ticker.Stop()
		cpuPercent := (last.CPUSeconds - first.CPUSeconds) / (last.ElapsedSeconds - first.ElapsedSeconds) * 100 / float64(runtime.NumCPU())
		t.Logf("phase=%s duration=%.1fs machine_cpu=%.4f%% keyboard_events=%d; resource samples require trend review", phase, last.ElapsedSeconds, cpuPercent, last.KeyboardEvents)
		if phase == "idle" && last.KeyboardEvents != 0 {
			t.Log("idle interval contained keyboard input; do not count it as uninterrupted idle acceptance")
		}
		if phase == "rapid" {
			if sent < uint64(seconds)*27 {
				t.Fatalf("rapid workload was interrupted by foreground/modifier changes: sent=%d expected_at_least=%d", sent, seconds*27)
			}
			until := time.Now().Add(time.Second)
			for sink.received.Load() < sent && time.Now().Before(until) {
				time.Sleep(time.Millisecond)
			}
			if sink.received.Load() < sent {
				t.Fatalf("foreground sink lost injected events: sent=%d received=%d system_keys=%d ime_compositions=%d", sent, sink.received.Load(), sink.systemKeys.Load(), sink.imeCompositions.Load())
			}
			sink.close()
		}
	}
	quitAcceptanceExecutable(t, p)
}

type acceptanceInputSink struct {
	hwnd            uintptr
	received        atomic.Uint64
	systemKeys      atomic.Uint64
	imeCompositions atomic.Uint64
	done            chan struct{}
	closed          atomic.Bool
}

type acceptanceWindowClass struct {
	size, style                        uint32
	callback                           uintptr
	classExtra, extra                  int32
	instance, icon, cursor, background uintptr
	menu, name                         *uint16
	smallIcon                          uintptr
}

type acceptanceMessage struct {
	hwnd           uintptr
	message        uint32
	wParam, lParam uintptr
	timestamp      uint32
	x, y           int32
	private        uint32
}

type acceptanceGUIThreadInfo struct {
	size, flags                                   uint32
	active, focus, capture, menu, moveSize, caret uintptr
	caretRect                                     [4]int32
}

func newAcceptanceInputSink(t *testing.T) *acceptanceInputSink {
	t.Helper()
	sink := &acceptanceInputSink{done: make(chan struct{})}
	ready := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(sink.done)
		instance, _, _ := windows.NewLazyDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
		className := windows.StringToUTF16Ptr(fmt.Sprintf("KeyViviAcceptanceInput-%d", os.Getpid()))
		callback := syscall.NewCallback(func(hwnd, message, wParam, lParam uintptr) uintptr {
			// 无焦点时按键可能作为 WM_SYSKEYDOWN 到达活动窗口，也属于实际收到的输入。
			if message == 0x100 || message == 0x104 {
				sink.received.Add(1)
			}
			if message == 0x104 {
				sink.systemKeys.Add(1)
			}
			if message == 0x10f {
				sink.imeCompositions.Add(1)
			}
			if message == 2 {
				acceptanceUser32.NewProc("PostQuitMessage").Call(0)
			}
			ret, _, _ := acceptanceUser32.NewProc("DefWindowProcW").Call(hwnd, message, wParam, lParam)
			return ret
		})
		class := acceptanceWindowClass{callback: callback, instance: instance, background: 6, name: className}
		class.size = uint32(unsafe.Sizeof(class))
		if ret, _, err := acceptanceUser32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class))); ret == 0 {
			ready <- fmt.Errorf("register input sink: %v", err)
			return
		}
		defer acceptanceUser32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(className)), instance)
		title := windows.StringToUTF16Ptr("KeyVivi 验收输入（切换窗口停止注入）")
		hwnd, _, err := acceptanceUser32.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), 0x10cf0000, 200, 200, 600, 160, 0, 0, instance, 0)
		if hwnd == 0 {
			ready <- fmt.Errorf("create input sink: %v", err)
			return
		}
		sink.hwnd = hwnd
		defer acceptanceUser32.NewProc("DestroyWindow").Call(hwnd)
		acceptanceUser32.NewProc("ShowWindow").Call(hwnd, 1)
		acceptanceUser32.NewProc("SetForegroundWindow").Call(hwnd)
		ready <- nil
		var message acceptanceMessage
		for {
			ret, _, _ := acceptanceUser32.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0)
			if int32(ret) <= 0 {
				return
			}
			acceptanceUser32.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&message)))
			acceptanceUser32.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&message)))
		}
	}()
	if err := <-ready; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sink.close)
	if !sink.isForeground() {
		t.Log("input sink is waiting up to 60 seconds for a manual click; no input is injected before foreground is acquired")
		deadline := time.Now().Add(time.Minute)
		for !sink.isForeground() {
			if time.Now().After(deadline) {
				sink.close()
				t.Fatal("input sink did not acquire foreground; no keyboard input was injected")
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
	return sink
}

func (s *acceptanceInputSink) isForeground() bool {
	hwnd, _, _ := acceptanceUser32.NewProc("GetForegroundWindow").Call()
	if hwnd != s.hwnd {
		return false
	}
	// 前台切换完成前键盘焦点可能尚未迁移，不能仅凭前台 HWND 开始注入。
	thread, _, _ := acceptanceUser32.NewProc("GetWindowThreadProcessId").Call(s.hwnd, 0)
	if thread == 0 {
		return false
	}
	info := acceptanceGUIThreadInfo{size: uint32(unsafe.Sizeof(acceptanceGUIThreadInfo{}))}
	ret, _, _ := acceptanceUser32.NewProc("GetGUIThreadInfo").Call(thread, uintptr(unsafe.Pointer(&info)))
	return ret != 0 && info.active == s.hwnd && info.focus == s.hwnd
}

func (s *acceptanceInputSink) close() {
	if !s.closed.CompareAndSwap(false, true) {
		return
	}
	acceptanceUser32.NewProc("PostMessageW").Call(s.hwnd, 0x10, 0, 0)
	<-s.done
}

func acceptanceModifiersReleased() bool {
	for _, key := range []uintptr{0x10, 0x11, 0x12, 0x5b, 0x5c} {
		state, _, _ := acceptanceUser32.NewProc("GetAsyncKeyState").Call(key)
		if state&0x8000 != 0 {
			return false
		}
	}
	return true
}

func acceptanceSendKey(key uint16) error {
	// INPUT 在 x64 为 40 字节；一次调用提交完整按下/释放，不能留下按住状态。
	type input struct {
		kind        uint32
		_           uint32
		key, scan   uint16
		flags, time uint32
		extra       uintptr
		_           uint64
	}
	inputs := [2]input{{kind: 1, key: key}, {kind: 1, key: key, flags: 2}}
	ret, _, err := acceptanceUser32.NewProc("SendInput").Call(2, uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	if ret != 2 {
		// 即使系统只接收按下事件，也必须补发抬起，不能在失败路径留下按住状态。
		release := input{kind: 1, key: key, flags: 2}
		acceptanceUser32.NewProc("SendInput").Call(1, uintptr(unsafe.Pointer(&release)), unsafe.Sizeof(release))
		return fmt.Errorf("SendInput delivered %d of 2 events: %v", ret, err)
	}
	return nil
}
