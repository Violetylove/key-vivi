//go:build windows && integration

package integration_test

import (
	"testing"

	// 同包测试在子进程运行，外层仍须跟踪应用源码依赖。
	_ "key-vivi/internal/app"
	"key-vivi/tests/testutil"
)

func TestNativeSettings(t *testing.T) {
	testutil.RunPackage(t, "app", true)
}
