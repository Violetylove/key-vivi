package unit_test

import (
	"testing"

	// 保留生产包依赖，使外层测试缓存随应用源码变化失效。
	_ "key-vivi/internal/app"
	"key-vivi/tests/testutil"
)

func TestInternalPackages(t *testing.T) {
	for _, pkg := range []string{"app", "render"} {
		t.Run(pkg, func(t *testing.T) { testutil.RunPackage(t, pkg, false) })
	}
}
