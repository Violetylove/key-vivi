package main

import (
	"key-vivi/internal/app"
	"key-vivi/internal/platform"
	"log"
	"os"
)

func main() {
	if err := app.Run(); err != nil {
		log.Printf("application stopped: %v", err)
		platform.ShowMessage(0, "KeyVivi 启动或运行失败", "程序已清理资源并退出。\n"+err.Error())
		os.Exit(1)
	}
}
