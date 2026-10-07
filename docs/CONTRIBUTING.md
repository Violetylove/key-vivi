# 贡献规范

## 文档与目录

文档职责见[协作规则](../AGENTS.md#文档职责)。修改需求或实现时同步对应文档，避免重复维护。

目录与测试入口见[仓库导航](../REPO_MAP.md)。生产源码与测试分离，根目录不放源码、测试或 exe。

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

构建关闭 CGO，用户运行无需 Go 或 C 编译器；Windows 竞态检查需要 `CGO_ENABLED=1` 和可用的 C 编译器。构建后直接运行 `dist\KeyVivi.exe`。

原生集成测试需要交互式桌面，会创建窗口、注册热键并注入 F24：

```powershell
$env:CGO_ENABLED = '1'
go test -race -tags integration ./tests/integration -count=1 -v -timeout=120s
```

普通 `go test ./...` 也会运行 `tests/unit/testdata` 中的应用和字体回归；入口通过 Go overlay 加入原包，显式执行 vet，并继承外层的 `-race`。原生设置回归同样由 `tests/integration` 的入口运行。不要把同包测试移回生产目录。

实际 exe 回归可选，需要先构建，再从普通用户交互式桌面执行。测试复制 exe 到独立临时目录，不改用户配置：

```powershell
$env:KEYVIVI_EXE = (Resolve-Path dist/KeyVivi.exe).Path
go test -race -tags integration ./tests/integration -run '^TestExecutable' -count=1 -v -timeout=30s
Remove-Item Env:KEYVIVI_EXE
```

覆盖正常退出和热键释放、热键占用时的托盘路径，以及模拟 `TaskbarCreated` 恢复。模拟通知不等同于真实 Explorer 重启。测试默认输出到临时目录；需要保留进程诊断时设置 `KEYVIVI_ARTIFACT_DIR`。一次性预览、像素探针与长时间采样工具已移除，历史结果保存在本地证据或 Git 历史中。

## 发行

推送 `v*` 标签触发[发行工作流](../.github/workflows/release.yml)：Windows x64 上检查格式、执行 vet 和竞态测试，直接 `go build`，通过 `softprops/action-gh-release` 上传 `KeyVivi.exe` 并生成发行说明。含 `-` 的标签标为预发行版，例如 `v1.0.0-rc.1`；预发行版不进入 GitHub 的正式版 `latest` 查询。发布前收尾检查见[规格](project_spec.md#验收条件)。

完成检查、提交并审阅暂存差异后创建标签，推送提交与标签，例如：

```powershell
git tag v1.0.0
git push origin main v1.0.0
```

## 提交

提交信息使用英文 Conventional Commits，例如 `fix(settings): preserve draft after saving`。功能完成或收工时统一提交，同一功能的连续修补不拆碎。

提交前检查 `git diff --cached`，不包含产物、证据、密钥或无关改动。未经授权不重写历史、不伪造作者身份。
