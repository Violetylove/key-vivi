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
		platform.ShowMessage(0, "KeyVivi 启动或运行失败", app.FailureMessage(err))
		os.Exit(1)
	}
}
