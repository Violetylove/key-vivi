package platform

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 完整性级别取自强制标签 SID（S-1-16-<rid>）。
const (
	IntegrityUntrusted = 0x0000
	IntegrityLow       = 0x1000
	IntegrityMedium    = 0x2000
	IntegrityHigh      = 0x3000
)

// IntegrityLevel 返回进程令牌的强制完整性级别 RID。
//
// Windows 不会把发往更高完整性窗口的输入交给低完整性进程的全局键盘钩子，
// 沙箱下钩子只能看到本进程的窗口——焦点在本窗口时才有反应。读出级别才能如实提示，
// 而不是让程序看起来坏掉。
func IntegrityLevel() (uint32, error) {
	token := windows.GetCurrentProcessToken()
	var size uint32
	err := windows.GetTokenInformation(token, windows.TokenIntegrityLevel, nil, 0, &size)
	if err != nil && err != windows.ERROR_INSUFFICIENT_BUFFER {
		return 0, fmt.Errorf("GetTokenInformation(size): %w", err)
	}
	if size == 0 {
		return 0, fmt.Errorf("GetTokenInformation returned no integrity label size")
	}
	buffer := make([]byte, size)
	if err := windows.GetTokenInformation(token, windows.TokenIntegrityLevel, &buffer[0], size, &size); err != nil {
		return 0, fmt.Errorf("GetTokenInformation(label): %w", err)
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&buffer[0]))
	sid := label.Label.Sid
	count := int(sid.SubAuthorityCount())
	if count == 0 {
		return 0, fmt.Errorf("integrity label SID has no sub-authority")
	}
	// RID 是 SID 的最后一段子授权。
	return sid.SubAuthority(uint32(count - 1)), nil
}

// RestrictedIntegrity 判断完整性级别是否低于 Medium，即普通用户进程的级别。
func RestrictedIntegrity() (bool, uint32, error) {
	level, err := IntegrityLevel()
	if err != nil {
		return false, 0, err
	}
	return level < IntegrityMedium, level, nil
}
