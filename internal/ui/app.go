package ui

import (
	"fmt"
	"image/color"

	"key-vivi/internal/keyboard"
	"key-vivi/internal/platform"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func Run() {
	a := app.New()
	configureTheme(a)
	w := a.NewWindow("KeyVivi")
	text := canvas.NewText("按任意键测试", color.White)
	text.TextSize = 24
	text.Alignment = fyne.TextAlignCenter
	bg := canvas.NewRectangle(color.RGBA{15, 19, 28, 255})
	w.SetContent(container.NewStack(bg, container.NewVBox(container.NewCenter(text), widget.NewButton("退出", a.Quit))))
	w.Resize(fyne.NewSize(440, 110))
	w.SetFixedSize(true)
	w.SetCloseIntercept(a.Quit)

	events := make(chan keyboard.Event, 1024)
	stopHook, err := platform.StartKeyboardHook(events)
	if err != nil {
		text.Text = fmt.Sprintf("键盘监听启动失败: %v", err)
		text.Refresh()
		w.ShowAndRun()
		return
	}
	defer stopHook()
	done := make(chan struct{})
	defer close(done)
	go processKeys(events, done, func(value string) {
		text.Text = value
		text.Refresh()
	}, func() {
		// Keep the exit control reachable after the subtitle disappears.
		text.Text = ""
		text.Refresh()
	})
	w.ShowAndRun()
}
