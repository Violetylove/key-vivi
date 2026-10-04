package app

import (
	"errors"
	"fmt"

	"key-vivi/internal/platform"
)

type integrityError struct{ level uint32 }

func (e *integrityError) Error() string {
	return fmt.Sprintf("process integrity level %#x is below the required medium level %#x", e.level, platform.IntegrityMedium)
}

func checkStartupIntegrity(level uint32) error {
	if level < platform.IntegrityMedium {
		return &integrityError{level: level}
	}
	return nil
}

// FailureMessage 将启动条件错误转为可操作的中文提示，日志仍使用原始英文错误。
func FailureMessage(err error) string {
	var restricted *integrityError
	if errors.As(err, &restricted) {
		return fmt.Sprintf("KeyVivi 未启动：当前进程的完整性级别为 %#x，低于普通用户所需的 Medium（0x2000）。\n\n低完整性运行会影响托盘和全局按键采集。请使用已正确构建的 KeyVivi.exe；若这是本项目的开发产物，请重新执行 build.ps1 构建后直接双击 exe。\n\n程序未安装键盘钩子，也未写入配置。", restricted.level)
	}
	return "程序已清理资源并退出。\n" + err.Error()
}
