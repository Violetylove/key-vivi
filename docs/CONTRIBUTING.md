# 贡献规范

## 文档与目录

文档职责见[协作规则](../AGENTS.md#文档职责)。修改需求或实现时同步对应文档，避免重复维护。

| 路径 | 用途 |
|---|---|
| `cmd/keyvivi` | 程序入口 |
| `internal/{app,keyboard,display,render,platform}` | 实现代码 |
| `tests/unit`、`tests/integration` | 单元和原生集成测试 |
| `tests/artifacts` | 本地验证证据，Git 忽略 |
| `docs` | 项目文档 |
| `dist` | 构建产物，Git 忽略 |

未导出实现允许使用同包 `*_test.go`，文件头说明原因。根目录不放源码、测试或 exe。

## 编码

- 修改前读取文件，保留无关改动。
- 注释使用中文，只解释原因和约束；导出标识符首句给出结论。
- 踩坑注释记录现象与已确认原因。
- 日志使用英文，不记录键码、组合名称或字幕内容。
- Go 文件使用 LF 换行，由 `.gitattributes` 固定，避免 Windows 检出导致格式检查失败。
- 输入、线程、窗口与清理约束见[设计文档](architecture.md)。

## 构建与测试

在 Windows 仓库根目录执行，Go 版本以 [go.mod](../go.mod) 为准：

```powershell
gofmt -w .
go vet ./...
$env:CGO_ENABLED = '1'
go test -race ./...
$env:CGO_ENABLED = '0'
go build -trimpath -ldflags='-s -w -H=windowsgui' -o dist/KeyVivi.exe ./cmd/keyvivi
```

构建关闭 CGO，用户运行无需 Go 或 C 编译器；Windows 竞态检查需要 `CGO_ENABLED=1` 和可用的 C 编译器。构建后直接运行 `dist\KeyVivi.exe`。本地目录带 Low 标签时，按[窗口排查](issue-layered-window.md#低完整性启动)处理；这不是源码或构建命令的要求。

原生集成测试需要交互式桌面，会创建窗口、注册热键并注入 F24：

```powershell
$env:CGO_ENABLED = '1'
go test -race -tags integration ./internal/app ./tests/integration -count=1 -v -timeout=90s
```

可选验证通过环境变量显式启用：

| 环境变量 | 验证命令 |
|---|---|
| `KEYVIVI_SETTINGS_PREVIEW=1` | `go test -race -tags integration ./internal/app -run 'TestNativeSettingsDraftSaveCancelDefaultsAndDPI' -count=1` |
| `KEYVIVI_ANIMATION_PREVIEW=1` | `go test ./internal/app -run 'Preview$' -count=1` |
| `KEYVIVI_MEASURE_CADENCE=1` | `go test -tags integration ./tests/integration -run '^TestLoopCadence$' -count=1 -v -timeout=15s` |

预览写入 `tests/artifacts`。帧间隔测量不包含渲染与桌面合成；正式验收要求见[项目规格](project_spec.md#验收条件)。

实际 exe 的进程外验收需先构建，并从普通用户交互式桌面执行；受限代理桌面不能替代该环境。测试复制 exe 到独立临时目录，不改用户配置：

```powershell
$env:KEYVIVI_EXE = (Resolve-Path dist/KeyVivi.exe).Path
$env:KEYVIVI_ARTIFACT_DIR = Join-Path (Get-Location) 'tests/artifacts/executable'
go test -race -tags integration ./tests/integration -run '^TestExecutable(Lifecycle|OccupiedPauseHotkey|TaskbarCreatedRecovery)$' -count=1 -v -timeout=30s
$env:KEYVIVI_MEASURE_STABILITY = '1'
go test -race -tags integration ./tests/integration -run '^TestExecutableStability$' -count=1 -v -timeout=25m
Remove-Item Env:KEYVIVI_EXE, Env:KEYVIVI_ARTIFACT_DIR, Env:KEYVIVI_MEASURE_STABILITY
```

稳定性默认空闲和快速输入各 600 秒，每 10 秒记录 CPU 时间、工作集、私有内存及进程/GDI/USER 句柄到 `stability.jsonl`，并检查 UI 响应。CPU 百分比按逻辑处理器数归一化；资源样本需审阅趋势，不以测试返回成功代替无增长结论。空闲期间检测到按键时不得记为纯空闲通过。快速输入为每秒约 30 次，仅向同时拥有前台与键盘焦点的专用测试窗口注入，修饰键按住或焦点移出时停止；自动取得焦点失败时等待人工点击最多 60 秒，输入量不足会失败。接收计数覆盖普通及系统按键消息，另记录系统按键与输入法消息数量，以辅助定位差异，不记录按键内容。

可用 `KEYVIVI_STABILITY_PHASE=idle` 或 `rapid` 单独测量一段；`KEYVIVI_STABILITY_SECONDS`（5–3600）仅用于调试测量工具，短测不能替代每段 10 分钟验收。用完后清除这些环境变量。生命周期测试覆盖消息循环正常退出、窗口/托盘移除及热键释放；托盘恢复测试模拟条目丢失和 `TaskbarCreated` 通知，不等同于真实 Explorer 重启或人工点击退出。

## 发行

推送 `v*` 标签触发[发行工作流](../.github/workflows/release.yml)：Windows x64 上检查格式、执行 vet 和竞态测试，直接 `go build`，通过 `softprops/action-gh-release` 上传 `KeyVivi.exe` 并生成发行说明。含 `-` 的标签标为预发行版，例如 `v0.1.0-rc.1`。原生桌面验收仍按[规格](project_spec.md#验收条件)执行。

确认发布条件后创建并推送标签，例如：

```powershell
git tag v0.1.0
git push origin v0.1.0
```

## 提交

提交信息使用英文 Conventional Commits，例如 `fix(settings): preserve draft after saving`。功能完成或收工时统一提交，同一功能的连续修补不拆碎。

提交前检查 `git diff --cached`，不包含产物、证据、密钥或无关改动。未经授权不重写历史、不伪造作者身份。
