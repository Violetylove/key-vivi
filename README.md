# KeyVivi

Windows 按键可视化工具，使用 Go + Fyne。当前按键功能已获用户确认，正在准备 v0.5 的托盘、暂停和桌面悬浮功能。

## 目录

```text
cmd/keyvivi/          程序入口
internal/keyboard/   输入事件和按键状态
internal/platform/   Windows API 与键盘钩子
internal/ui/         窗口、字体与显示调度
tests/unit/         单元测试
tests/integration/  桌面集成测试
tests/artifacts/    测试证据
docs/               规格与项目进度
dist/               构建产物
```

## 构建与测试

在项目根目录执行。桌面构建需要 Go、CGO 和可用的 C 编译器。

```bash
go test -race ./...
go build -ldflags="-s -w -H=windowsgui" -o dist/KeyVivi.exe ./cmd/keyvivi
```

集成测试需要 Windows 交互桌面，会注入一次 F24 按下/释放事件，默认不运行：

```bash
go test -race -tags integration ./tests/integration
```

程序入口：[KeyVivi.exe](<dist/KeyVivi.exe>)。

## 项目文档

- [规格与里程碑验收](<docs/project_spec.md>)
- [计划与当前进度](<docs/PROJECT.md>)
- [协作规则](<AGENTS.md>)
