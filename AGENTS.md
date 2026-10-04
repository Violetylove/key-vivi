# AGENTS.md - KeyVivi 协作规则

更新日期：2026-10-04（Asia/Shanghai）。

## 当前技术路线

- 已确定采用 Go + Win32 + Go 位图自绘；入口为 `cmd/keyvivi/main.go` → `internal/app.Run`。
- 产品需求、里程碑、进度与验收统一维护在 [project_spec.md](docs/project_spec.md)；实现设计维护在 [architecture.md](docs/architecture.md)。同类内容只维护一份，不再另建项目计划或设计副本。
- 旧实现的人工反馈只能作为输入行为参考，不能证明当前装配层或自绘窗口已通过验收。
- 用户已确认采用 YAML 配置与设置 UI，纳入 v1.0 的 M2.1；可选风格、鼠标和多显示器跟随仍属于 v2.0。

## 编码与提交

- 文件修改前读取内容，增量修改；未经授权不得删除代码重写。
- 注释一律中文、精炼，只解释原因和约束；导出标识符首句给出结论；踩坑记录包含现象与已确认原因。日志字符串英文，不记录按键内容。
- 源码放 `cmd/keyvivi` 与 `internal/{app,keyboard,display,render,platform}`；测试放 `tests/unit` 或 `tests/integration`，证据放 `tests/artifacts`，文档放 `docs`，产物放 `dist`。
- 被测代码未导出时允许同包 `*_test.go`，文件头说明原因；禁止将源码、测试或 exe 堆在根目录。
- 提交信息全部英文，遵循 Conventional Commits；模块/功能完成或每天收工时提交一次，不为连续修补拆碎提交。
- 提交前必须通过 `gofmt -w .`、`go vet ./...`、`go test -race ./...`，并检查 `git diff --cached`。不提交 `dist/`、`tests/artifacts/` 或无关改动。
- 规则与示例见 [CONTRIBUTING.md](docs/CONTRIBUTING.md)。

## 输入、线程与窗口约束

- 组合输入由按下/释放状态决定，不用 300ms 超时猜测意图；左右修饰键独立追踪，组合后保留仍按住的键。
- 钩子线程只传递事件，回调不得阻塞键盘输入；必须有有效消息循环。丢事件需可诊断并恢复状态，不能留下永久按住的修饰键。
- UI 线程使用 `runtime.LockOSThread`；窗口、控制器、队列、动画和托盘状态由该线程持有。channel 不会自动调度 UI；投递新事件必须显式唤醒消息循环。
- 分层窗口必须包含 `WS_EX_LAYERED`；通过 `UpdateLayeredWindow` 提交预乘 BGRA 位图，同时更新内容、尺寸和位置。`Apply(nil)` 隐藏，销毁必须在创建线程执行。
- 位置由显示器工作区、位图尺寸与 DPI 计算；不写死屏幕分辨率。窗口显示、定位和样式由原生平台层管理，不抢焦点。
- 暂停立即清空逻辑队列和动画快照、隐藏窗口；恢复不能补显暂停期间输入或跨暂停仍按住的键。
- 所有已获得的句柄、图标、定时器、钩子和热键都有清理路径；不要在初始化回调中 `defer` 需要存活到主循环结束的资源。

## 环境与验证

- 历史A/B/A/B的工作区内Low、工作区外Medium差异已定位为exe继承工作区的Low文件标签。构建使用 `build.ps1`，只将产物exe标为普通用户Medium；开发和日常均直接启动exe。保留工作区目录安全设置，运行时先检查令牌，低于Medium时在配置/托盘/钩子装配前明确失败，不自行提权。桌面可见性仍需人工确认。
- 代理启动的普通窗口与叠加窗口可能都无法被截图或 `GetPixel` 看见；隔离原因尚未完全确认。可见性只能在真实桌面人工确认，不能用截图缺失推断窗口实现错误。
- 普通构建支持 `CGO_ENABLED=0`；Windows 竞态检查需要 CGO/C 编译器。开发依赖与用户运行依赖分别验收。
- 普通测试用 `go test -race ./...`；原生集成测试显式用 `go test -race -tags integration ./internal/app ./tests/integration`，会注入 F24，并验证实际Low进程被拒绝。
- 里程碑统一为 M1 核心可用、M2 正式发布、M3 增强版，各含两个子项；任务、进度与验收共用 M1.1 等编号，不再另设阶段编号。子项全部必需场景通过才算完成；编译、单元测试和 API 成功不能替代桌面验收。
- 日期使用系统时间，面向用户采用 Asia/Shanghai；无可靠记录时不编造耗时、资源占用或完成状态。
