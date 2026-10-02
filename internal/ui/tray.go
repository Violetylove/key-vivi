package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func setupTray(a fyne.App, toggle func(), about func(), paused func() bool) func() {
	desk := a.(desktop.App)
	pause := fyne.NewMenuItem("暂停", func() { toggle() })
	menu := fyne.NewMenu("KeyVivi", pause, fyne.NewMenuItem("关于 KeyVivi", about), fyne.NewMenuItemSeparator(), fyne.NewMenuItem("退出", a.Quit))
	desk.SetSystemTrayMenu(menu)
	desk.SetSystemTrayIcon(trayIcon(paused()))
	return func() {
		if paused() {
			pause.Label = "继续"
		} else {
			pause.Label = "暂停"
		}
		// 图标也承载状态：菜单只在打开时可见。
		desk.SetSystemTrayIcon(trayIcon(paused()))
		menu.Refresh()
	}
}
