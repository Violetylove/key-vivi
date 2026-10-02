package platform

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Integrity levels from the mandatory label SID (S-1-16-<rid>).
const (
	IntegrityUntrusted = 0x0000
	IntegrityLow       = 0x1000
	IntegrityMedium    = 0x2000
	IntegrityHigh      = 0x3000
)

// IntegrityLevel returns the process token's mandatory integrity level RID.
//
// Windows withholds input destined for higher-integrity windows from a
// low-integrity process's global low-level keyboard hook. A sandboxed launch
// therefore leaves the hook seeing only this process's own windows: keystrokes
// register while our window has focus and nowhere else. Reading the level lets
// the app say so instead of appearing broken.
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
	// The RID is the SID's last sub-authority.
	return sid.SubAuthority(uint32(count - 1)), nil
}

// RestrictedIntegrity reports whether the integrity level is below medium,
// which is the level a normal user process runs at.
func RestrictedIntegrity() (bool, uint32, error) {
	level, err := IntegrityLevel()
	if err != nil {
		return false, 0, err
	}
	return level < IntegrityMedium, level, nil
}
