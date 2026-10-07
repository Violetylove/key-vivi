// Package testutil 提供集中存放的同包测试入口，不改变产品导出接口。
package testutil

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// RunPackage 用 Go overlay 将 testdata 中的测试加入原包，不写入源码目录。
func RunPackage(t *testing.T, pkg string, integration bool) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(filepath.Dir(cwd))
	sources := filepath.Join(cwd, "testdata", pkg)
	files, err := os.ReadDir(sources)
	if err != nil {
		t.Fatal(err)
	}
	replace := make(map[string]string)
	temporary := t.TempDir()
	for _, file := range files {
		if filepath.Ext(file.Name()) == ".go" {
			// 外层测试读取源文件，让测试缓存能感知 testdata 变更。
			data, err := os.ReadFile(filepath.Join(sources, file.Name()))
			if err != nil {
				t.Fatal(err)
			}
			copyPath := filepath.Join(temporary, file.Name())
			if err := os.WriteFile(copyPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			replace[filepath.Join(root, "internal", pkg, file.Name())] = copyPath
		}
	}
	if len(replace) == 0 {
		t.Fatal("no package tests found")
	}
	data, err := json.Marshal(struct{ Replace map[string]string }{replace})
	if err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(temporary, "overlay.json")
	if err := os.WriteFile(overlay, data, 0600); err != nil {
		t.Fatal(err)
	}
	flags := []string{"-overlay", overlay}
	if integration {
		flags = append(flags, "-tags", "integration")
	}
	// testdata 不参与普通包枚举，因此显式 vet，且继承外层竞态检查模式。
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for _, operation := range []string{"vet", "test"} {
		args := append([]string{operation}, flags...)
		if operation == "test" {
			args = append(args, "-count=1", "-timeout=60s", "-v")
			if raceEnabled {
				args = append(args, "-race")
			}
		}
		args = append(args, "./internal/"+pkg)
		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		t.Logf("go %s %s:\n%s", operation, pkg, output)
		if err != nil {
			t.Fatalf("package %s %s failed: %v", pkg, operation, err)
		}
	}
}
