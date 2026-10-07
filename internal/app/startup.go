package app

// FailureMessage 保留原始故障原因，退出前由 Run 完成资源清理。
func FailureMessage(err error) string {
	return "程序已清理资源并退出。\n" + err.Error()
}
