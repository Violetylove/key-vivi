package platform

import (
	"fmt"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

var commonDialogs = windows.NewLazyDLL("comdlg32.dll")
var chooseColor = commonDialogs.NewProc("ChooseColorW")
var commonDialogError = commonDialogs.NewProc("CommDlgExtendedError")

type settingsChooseColor struct {
	size                 uint32
	owner, instance      uintptr
	result               uint32
	custom               *uint32
	flags                uint32
	data, hook, template uintptr
}

func settingsColorRef(value string) (uint32, error) {
	if len(value) != 7 || value[0] != '#' {
		return 0, fmt.Errorf("invalid color format")
	}
	rgb, err := strconv.ParseUint(value[1:], 16, 24)
	if err != nil {
		return 0, fmt.Errorf("invalid color value")
	}
	return uint32(rgb&0xFF)<<16 | uint32(rgb&0xFF00) | uint32(rgb>>16), nil
}

func settingsCustomColors() [16]uint32 {
	var colors [16]uint32
	for i, hex := range []string{"#FFFFFF", "#14181F", "#1E1E2E", "#CDD6F4", "#EFF1F5", "#4C4F69", "#F5C2E7", "#CBA6F7", "#F38BA8", "#FAB387", "#F9E2AF", "#A6E3A1", "#94E2D5", "#89DCEB", "#89B4FA", "#B4BEFE"} {
		colors[i], _ = settingsColorRef(hex)
	}
	return colors
}

// 原生选色器提供完整RGB选择，取消和失败都不能更改草稿；自定义色仅留在内存。
func chooseSettingsColor(owner uintptr, value string, custom *[16]uint32) (string, bool, error) {
	initial, err := settingsColorRef(value)
	if err != nil {
		return value, false, err
	}
	options := settingsChooseColor{size: uint32(unsafe.Sizeof(settingsChooseColor{})), owner: owner,
		result: initial, custom: &custom[0], flags: 3}
	if ok, _, _ := chooseColor.Call(uintptr(unsafe.Pointer(&options))); ok == 0 {
		if code, _, _ := commonDialogError.Call(); code != 0 {
			return value, false, fmt.Errorf("ChooseColorW error %#x", code)
		}
		return value, false, nil
	}
	rgb := options.result
	return fmt.Sprintf("#%02X%02X%02X", rgb&0xFF, (rgb>>8)&0xFF, (rgb>>16)&0xFF), true, nil
}
