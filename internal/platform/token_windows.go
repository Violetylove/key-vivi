package platform

import (
	"encoding/binary"
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
// 产物继承Low文件标签时，普通桌面启动也会降权，影响托盘及全局输入。
// 读取级别供装配前检查；运行时不能自行提升令牌或改目录权限。
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
	if uintptr(len(buffer)) < unsafe.Sizeof(windows.Tokenmandatorylabel{}) {
		return 0, fmt.Errorf("integrity label is truncated")
	}
	label := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&buffer[0]))
	start := uintptr(unsafe.Pointer(&buffer[0]))
	sid := uintptr(unsafe.Pointer(label.Label.Sid))
	if sid < start || sid-start >= uintptr(len(buffer)) {
		return 0, fmt.Errorf("integrity label SID is outside token buffer")
	}
	// SID在Go缓冲区内；系统包装器将uintptr转回指针会触发checkptr，改用有界字节解析。
	return integrityRID(buffer[int(sid-start):])
}

func integrityRID(sid []byte) (uint32, error) {
	if len(sid) < 8 || sid[0] != 1 {
		return 0, fmt.Errorf("integrity label SID header is invalid")
	}
	for i, want := range []byte{0, 0, 0, 0, 0, 16} {
		if sid[i+2] != want {
			return 0, fmt.Errorf("integrity label SID authority is invalid")
		}
	}
	count := int(sid[1])
	if count == 0 || count > 15 || len(sid) < 8+4*count {
		return 0, fmt.Errorf("integrity label SID sub-authorities are invalid")
	}
	// RID是SID的最后一段子授权。
	return binary.LittleEndian.Uint32(sid[8+4*(count-1):]), nil
}

// RestrictedIntegrity 判断完整性级别是否低于 Medium，即普通用户进程的级别。
func RestrictedIntegrity() (bool, uint32, error) {
	level, err := IntegrityLevel()
	if err != nil {
		return false, 0, err
	}
	return level < IntegrityMedium, level, nil
}
