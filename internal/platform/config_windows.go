package platform

import "golang.org/x/sys/windows"

// ReplaceConfigFile 在同目录安全替换配置，失败时保留原文件。
func ReplaceConfigFile(temp, target string) error {
	src, err := windows.UTF16PtrFromString(temp)
	if err != nil {
		return err
	}
	dst, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(src, dst, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
