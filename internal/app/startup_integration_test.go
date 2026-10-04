//go:build windows && integration

package app

// Run与权限错误未导出，同包子进程实际以Low执行，验证失败先于配置和钩子装配。
import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"key-vivi/internal/platform"
)

func TestStartupRejectsLowBeforeConfigAndHooks(t *testing.T) {
	if os.Getenv("KEYVIVI_LOW_STARTUP_HELPER") == "1" {
		level, err := platform.IntegrityLevel()
		if err != nil || level != platform.IntegrityLow {
			t.Fatal("helper did not start with low integrity", level, err)
		}
		var restricted *integrityError
		if err := Run(); !errors.As(err, &restricted) {
			t.Fatal("low process reached application assembly", err)
		}
		if platform.HookEventCount() != 0 {
			t.Fatal("low process installed a hook")
		}
		path, err := executableConfigPath()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("low process touched config", err)
		}
		return
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	copyPath := filepath.Join(t.TempDir(), "low-startup-test.exe")
	if err := os.WriteFile(copyPath, data, 0700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("icacls", copyPath, "/setintegritylevel", "L").CombinedOutput(); err != nil {
		t.Fatalf("prepare low test file: %v %s", err, output)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, copyPath, "-test.run=^TestStartupRejectsLowBeforeConfigAndHooks$", "-test.v")
	child.Env = append(os.Environ(), "KEYVIVI_LOW_STARTUP_HELPER=1")
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("low startup regression: %v %s", err, output)
	}
	t.Log("actual Low subprocess rejected before config and hook assembly")
}
