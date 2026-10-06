//go:build windows && integration

package integration_test

// 对实际发行产物做进程外回归，避免仅验证平台 API 而漏掉应用装配与清理。
import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"key-vivi/internal/platform"
)

var acceptanceUser32 = windows.NewLazyDLL("user32.dll")
var acceptanceShell32 = windows.NewLazyDLL("shell32.dll")

type executableWindows struct{ message, overlay uintptr }

type executableProcess struct {
	command *exec.Cmd
	done    chan error
	windows executableWindows
	logPath string
}

func startAcceptanceExecutable(t *testing.T, debug bool) *executableProcess {
	t.Helper()
	source := os.Getenv("KEYVIVI_EXE")
	if source == "" {
		t.Skip("set KEYVIVI_EXE to the built executable to run process acceptance")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	// 每次使用独立配置目录，不能覆盖用户的 YAML 或误停用户正在运行的实例。
	path := filepath.Join(t.TempDir(), "KeyVivi.exe")
	if err := os.WriteFile(path, data, 0700); err != nil {
		t.Fatal(err)
	}
	artifactDir := os.Getenv("KEYVIVI_ARTIFACT_DIR")
	if artifactDir == "" {
		artifactDir = t.TempDir()
	}
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(artifactDir, strings.ReplaceAll(t.Name(), "/", "-")+"-process.txt")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(path)
	for _, value := range os.Environ() {
		name, _, _ := strings.Cut(value, "=")
		if !strings.HasPrefix(strings.ToUpper(name), "KEYVIVI_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "KEYVIVI_READY_HINT=0")
	if debug {
		cmd.Env = append(cmd.Env, "KEYVIVI_DEBUG=1")
	}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		t.Fatal(err)
	}
	p := &executableProcess{command: cmd, done: make(chan error, 1), logPath: logPath}
	go func() {
		err := cmd.Wait()
		logFile.Close()
		p.done <- err
	}()
	t.Cleanup(func() {
		// 只清理本测试创建的子进程；失败时也不能留下隐藏实例或提示框。
		_ = cmd.Process.Kill()
	})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case err := <-p.done:
			t.Fatalf("executable exited during startup: %v", err)
		default:
		}
		p.windows = findExecutableWindows(uint32(cmd.Process.Pid))
		if p.windows.message != 0 && p.windows.overlay != 0 && executableTrayPresent(p.windows.message) {
			t.Logf("executable ready: pid=%d message=%#x overlay=%#x", cmd.Process.Pid, p.windows.message, p.windows.overlay)
			return p
		}
		time.Sleep(20 * time.Millisecond)
	}
	logText, _ := os.ReadFile(logPath)
	t.Fatalf("executable did not finish startup: %s", logText)
	return nil
}

type executableWindowScan struct {
	pid     uint32
	windows executableWindows
}

// Win32 回调不能释放；复用一个回调，通过同步枚举上下文传参，避免轮询耗尽回调槽。
var executableEnumerationCallback = syscall.NewCallback(func(hwnd uintptr, context unsafe.Pointer) uintptr {
	scan := (*executableWindowScan)(context)
	var owner uint32
	acceptanceUser32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&owner)))
	if owner != scan.pid {
		return 1
	}
	var class [64]uint16
	acceptanceUser32.NewProc("GetClassNameW").Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class)))
	if windows.UTF16ToString(class[:]) != "KeyViviWindow" {
		return 1
	}
	style, _, _ := acceptanceUser32.NewProc("GetWindowLongPtrW").Call(hwnd, ^uintptr(19)) // GWL_EXSTYLE=-20。
	if style&0x80000 != 0 {
		scan.windows.overlay = hwnd
	} else {
		scan.windows.message = hwnd
	}
	return 1
})

func findExecutableWindows(pid uint32) executableWindows {
	scan := executableWindowScan{pid: pid}
	acceptanceUser32.NewProc("EnumWindows").Call(executableEnumerationCallback, uintptr(unsafe.Pointer(&scan)))
	return scan.windows
}

type acceptanceTrayID struct {
	size uint32
	hwnd uintptr
	id   uint32
	guid windows.GUID
}

func executableTrayPresent(hwnd uintptr) bool {
	id := acceptanceTrayID{hwnd: hwnd, id: 1}
	id.size = uint32(unsafe.Sizeof(id))
	var rect [4]int32
	result, _, _ := acceptanceShell32.NewProc("Shell_NotifyIconGetRect").Call(uintptr(unsafe.Pointer(&id)), uintptr(unsafe.Pointer(&rect)))
	return int32(result) == 0
}

func pingExecutable(hwnd uintptr) error {
	var result uintptr
	ret, _, err := acceptanceUser32.NewProc("SendMessageTimeoutW").Call(hwnd, 0, 0, 0, 3, 500, uintptr(unsafe.Pointer(&result)))
	if ret == 0 {
		return fmt.Errorf("executable UI did not respond within 500ms: %v", err)
	}
	return nil
}

func quitAcceptanceExecutable(t *testing.T, p *executableProcess) time.Duration {
	t.Helper()
	begin := time.Now()
	// 发到本测试的消息窗口，走应用原有消息循环退出和逆序清理路径。
	if ret, _, err := acceptanceUser32.NewProc("PostMessageW").Call(p.windows.message, 0x8003, 0, 0); ret == 0 {
		t.Fatalf("request executable quit: %v", err)
	}
	select {
	case err := <-p.done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("executable did not exit within 2 seconds")
	}
	elapsed := time.Since(begin)
	if executableTrayPresent(p.windows.message) {
		t.Fatal("tray icon survived process exit")
	}
	for _, hwnd := range []uintptr{p.windows.message, p.windows.overlay} {
		if ret, _, _ := acceptanceUser32.NewProc("IsWindow").Call(hwnd); ret != 0 {
			t.Fatal("application window survived process exit")
		}
	}
	t.Logf("normal message-loop exit completed in %s; tray and windows removed", elapsed)
	return elapsed
}

func TestExecutableLifecycle(t *testing.T) {
	p := startAcceptanceExecutable(t, true)
	if err := pingExecutable(p.windows.message); err != nil {
		t.Fatal(err)
	}
	quitAcceptanceExecutable(t, p)
	stop, err := platform.StartPauseHotkey(func() {})
	if err != nil {
		t.Fatalf("pause hotkey was not released after executable exit: %v", err)
	}
	stop()
}

func TestExecutableOccupiedPauseHotkey(t *testing.T) {
	if os.Getenv("KEYVIVI_EXE") == "" {
		t.Skip("set KEYVIVI_EXE to run process acceptance")
	}
	stop, err := platform.StartPauseHotkey(func() {})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	p := startAcceptanceExecutable(t, true)
	deadline := time.Now().Add(2 * time.Second)
	for {
		text, err := os.ReadFile(p.logPath)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(text), "startup ready: tray=true pause_hotkey=false") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("occupied hotkey did not leave a usable tray and a clear diagnostic")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := pingExecutable(p.windows.message); err != nil {
		t.Fatal(err)
	}
	quitAcceptanceExecutable(t, p)
}

func TestExecutableTaskbarCreatedRecovery(t *testing.T) {
	p := startAcceptanceExecutable(t, true)
	// 只删除本测试实例的托盘条目，模拟 Explorer 丢失条目后广播重建通知。
	// 这能覆盖应用恢复路径，不能替代真实 Explorer 重启的桌面验收。
	data := struct {
		size                uint32
		hwnd                uintptr
		id, flags, callback uint32
		icon                uintptr
		tip                 [128]uint16
		state, stateMask    uint32
		info                [256]uint16
		version             uint32
		infoTitle           [64]uint16
		infoFlags           uint32
		guid                windows.GUID
		balloonIcon         uintptr
	}{hwnd: p.windows.message, id: 1}
	data.size = uint32(unsafe.Sizeof(data))
	if ret, _, err := acceptanceShell32.NewProc("Shell_NotifyIconW").Call(2, uintptr(unsafe.Pointer(&data))); ret == 0 {
		t.Fatalf("remove the test tray entry: %v", err)
	}
	if executableTrayPresent(p.windows.message) {
		t.Fatal("test tray entry was not removed")
	}
	name := windows.StringToUTF16Ptr("TaskbarCreated")
	message, _, err := acceptanceUser32.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(name)))
	if message == 0 {
		t.Fatal(err)
	}
	if ret, _, err := acceptanceUser32.NewProc("PostMessageW").Call(p.windows.message, message, 0, 0); ret == 0 {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for !executableTrayPresent(p.windows.message) {
		if time.Now().After(deadline) {
			t.Fatal("tray was not restored after TaskbarCreated")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := pingExecutable(p.windows.message); err != nil {
		t.Fatal(err)
	}
	quitAcceptanceExecutable(t, p)
}
