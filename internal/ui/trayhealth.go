package ui

import (
	"io"
	"log"
	"strings"
	"sync"
)

// trayHealth 记录系统托盘是否初始化成功。
//
// Fyne 的托盘 API 失败时不返回错误，只写标准日志；托盘又是唯一的退出入口，
// 缺失时用户既没有窗口也没有图标、无法退出。这里截获日志，以便提示并给出兜底退出。
type trayHealth struct {
	mu     sync.Mutex
	failed bool
	detail string
}

// watchTray 复制标准日志输出以捕获托盘失败；内容原样转发，不影响正常日志。
func watchTray() *trayHealth {
	health := &trayHealth{}
	log.SetOutput(io.MultiWriter(log.Writer(), health))
	return health
}

func (h *trayHealth) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "systray error") {
		h.mu.Lock()
		h.failed = true
		h.detail = strings.TrimSpace(string(p))
		h.mu.Unlock()
	}
	return len(p), nil
}

// Failed 返回是否记录过托盘错误，以及第一行详情。
func (h *trayHealth) Failed() (bool, string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.failed, h.detail
}
