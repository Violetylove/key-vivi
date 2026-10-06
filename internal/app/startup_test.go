package app

// 启动校验与错误类型未导出，同包验证权限失败发生在资源装配之前的条件边界。
import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestStartupIntegrityAndFailureMessage(t *testing.T) {
	for _, level := range []uint32{0, 0x1000, 0x1fff, 0x2000, 0x2100, 0x3000, 0x4000} {
		err := checkStartupIntegrity(level)
		if level >= 0x2000 {
			if err != nil {
				t.Fatal("ordinary or elevated process rejected", level, err)
			}
			continue
		}
		var restricted *integrityError
		if !errors.As(err, &restricted) || restricted.level != level {
			t.Fatal("restricted startup accepted", level, err)
		}
		message := FailureMessage(fmt.Errorf("startup: %w", err))
		if !strings.Contains(message, "未启动") || !strings.Contains(message, "普通桌面") || strings.Contains(err.Error(), "托盘") {
			t.Fatal("startup guidance or English diagnostic missing", message, err)
		}
	}
	err := errors.New("native window failed")
	if !strings.Contains(FailureMessage(err), err.Error()) {
		t.Fatal("native failure cause lost")
	}
}
