package ui

import (
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type chineseTheme struct {
	fyne.Theme
	font fyne.Resource
}

func (t chineseTheme) Font(style fyne.TextStyle) fyne.Resource { return t.font }

func configureTheme(a fyne.App) {
	// 使用系统已安装的中文字体，不随程序分发。
	fontPath := filepath.Join(os.Getenv("WINDIR"), "Fonts", "Deng.ttf")
	if data, err := os.ReadFile(fontPath); err == nil {
		a.Settings().SetTheme(chineseTheme{theme.DefaultTheme(), fyne.NewStaticResource("Deng.ttf", data)})
	}
}
