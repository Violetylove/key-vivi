# 编码与提交规范

## 注释

代码注释一律使用中文，且必须精炼。

- 只写"为什么这样做"和"约束是什么"，不复述代码在做什么。
- 单行能说清就写单行；禁止把实现逐句翻译成注释。
- 导出标识符使用文档注释，首句给出结论，补充说明另起一段。
- 踩过的坑要留下"现象 + 原因"，例如："分层窗口必须提交预乘 BGRA，否则半透明边缘会出现色晕"。
- 日志字符串保持英文，便于 grep 与对照文档；注释中文化不影响日志。

```go
// 正确：说明约束
// 窗口和 GDI 资源有线程亲和性，须在创建它们的 UI 线程释放。

// 错误：复述代码
// 调用 Show 方法显示窗口。
```

## 提交信息

标题与正文全部使用英文，遵循 Conventional Commits：

```text
<type>(<optional scope>): <description>
```

描述用祈使句、简洁。常用类型：feat、fix、refactor、test、docs、build、ci、chore、perf、revert。破坏性变更用 `!` 或 `BREAKING CHANGE` 脚注。

示例：

```text
chore: initialize KeyVivi repository
feat(display): add FIFO keystroke queue
fix(keyboard): preserve held modifier state
test(platform): verify hotkey cleanup
```

## 提交节奏

**不要一改动就提交。**

- 每天收工，或一个模块／功能完成时提交一次。
- 粒度按模块或功能，宁可少而完整。
- 同一模块的连续修补合并成一个提交，不要留下"再修一下"的碎片提交。
- 需要重写历史时先打备份分支，重写后用 `git diff` 确认内容零差异。

## 提交前检查

必须在仓库根目录通过以下命令，并检查 `git diff --cached`：

```bash
gofmt -w .
go vet ./...
go test -race ./...
go build -ldflags="-s -w -H=windowsgui" -o dist/KeyVivi.exe ./cmd/keyvivi
```

集成测试需要交互式桌面，会注入键盘事件，显式运行：

```bash
go test -race -tags integration ./tests/integration
```

不要提交产物（`dist/`）、测试截图或证据（`tests/artifacts/`）、密钥，或与本次改动无关的内容。不要伪造作者身份，未经同意不要重写历史。

## 目录布局

程序入口放 `cmd/keyvivi`，实现在 `internal` 各模块，测试放 `tests/unit` 或 `tests/integration`，文档在 `docs`，产物在 `dist`，本地测试证据在 `tests/artifacts`。被测代码未导出时，允许在同包内放 `*_test.go`，并在文件头说明原因。

构建成功不能作为功能完成的证据；桌面行为必须在真实桌面验证。
