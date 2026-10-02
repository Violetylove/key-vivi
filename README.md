# KeyVivi

Windows 按键可视化工具，使用 Go + Fyne。v0.5（托持、暂停、FIFO 字幕队列）实现完成，桌面验收进行中。

**启动方式：双击 [`launch.bat`](<launch.bat>)，不要直接双击 `dist\KeyVivi.exe`。** 原因见下方「启动方式」。

## 目录

```text
cmd/keyvivi/          程序入口
internal/display/     FIFO 队列与暂停状态
internal/keyboard/    输入事件和按键状态
internal/platform/    Windows API、键盘钩子、托盘图标
internal/ui/          窗口、字体、显示调度与托盘
tests/unit/           单元测试
tests/integration/    桌面集成测试（-tags integration）
tests/artifacts/      测试证据（Git 忽略）
docs/                 规格与项目进度
dist/                 构建产物（Git 忽略）
launch.bat            推荐启动入口
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

## 安装与卸载

免安装，不需要管理员权限，不写注册表，不创建自启动或服务。双击 exe 即可运行，退出后删除程序文件夹即移除主程序。

由于 Fyne v2.8.1 不提供存储位置覆盖接口，程序运行后框架自身会写入以下用户目录：

```text
%APPDATA%\fyne\              框架设置与偏好（settings.json、preferences.json）
%LOCALAPPDATA%\fyne\         框架缓存
%LOCALAPPDATA%\Temp\         托盘图标临时文件（进程退出后可清理）
```

这些路径不受程序控制。如需彻底清除，关闭程序后手动删除上述 `fyne` 目录即可；删除后重启程序会重新生成，不影响使用。这属于框架已知行为，不作为缺陷处理，详见[规格第 3.4 节](<docs/project_spec.md>)。

## 启动方式（重要）

**双击 [`launch.bat`](<launch.bat>)，不要直接双击 `dist\KeyVivi.exe`。**

`launch.bat` 会把 exe 复制到 `%LOCALAPPDATA%\KeyVivi` 再启动。原因是实测到的沙箱行为：

| exe 位置 | 完整性级别 | 结果 |
|---|---|---|
| 代理工作区内（`dist\`） | `0x1000` Low | 只有 KeyVivi 自己的窗口有焦点时才显示按键；无托盘 |
| 工作区外（`%LOCALAPPDATA%`） | `0x2000` Medium | 全局按键正常；托盘正常 |

A/B/A/B 四次可复现，且与启动方式（直接运行、`cmd /c start`、批处理）无关——**只看 exe 在哪**。Windows 不会把更高完整性窗口的输入交给低完整性进程的全局键盘钩子；同一限制也让 systray 写不了临时图标文件。

程序启动横幅会显示状态：正常是「KeyVivi 已启动」，受限时是「KeyVivi 已启动（受限环境 0x1000，仅本窗口有效）」。不弹窗。

如果不想用 `launch.bat`，把 `dist` 整个复制到任意工作区外的目录（例如桌面上的文件夹）直接运行也一样。

另外，`go build` 每次产出新哈希，未签名的 exe 没有 SmartScreen 信誉，可能弹「Windows 已保护你的电脑」。点「更多信息 → 仍要运行」一次即可；`launch.bat` 已加 `Unblock-File`。

## 已知问题

- **字幕高度偏厚**：设计的逻辑尺寸是 1100×64，但固定尺寸窗口实际量到 1459×141 物理像素（DPI 1.25）。位置（水平居中、贴底）已确认正确。若要调整，改 `internal/ui/app.go` 的 `overlayWidth` / `overlayHeight`。
- 托盘、暂停快捷键、10 分钟稳定性与可靠退出尚未逐项验收。

## 诊断

设置 `KEYVIVI_DEBUG=1` 后运行，程序会向 stderr 输出完整性级别、钩子安装、回调计数和队列长度，用于排查“按键不显示”等问题。它只记录数量，不记录按下了哪些键。

```bash
KEYVIVI_DEBUG=1 ./dist/KeyVivi.exe
```

`KEYVIVI_SELFTEST=1` 装好钩子后注入一对 F24 并弹出结论；`KEYVIVI_DEMO=1` 在启动提示结束后注入 A、B、C，用来在没有键盘的情况下验证显示链路（会向当前焦点窗口真的输入这三个字母，仅手动排查时使用）：

```powershell
$env:KEYVIVI_SELFTEST=1; .\dist\KeyVivi.exe
```

`KEYVIVI_DEBUG` 还会在窗口标题里带上完整性级别（`KeyVivi 0x2000`），便于从外部确认启动方式是否生效。

托盘不可用时程序会静默注册 `Ctrl+Alt+Q` 作为退出兜底，并写日志而不弹窗。

`KEYVIVI_READY_HINT` 控制启动提示的秒数，设为 `0` 可关闭。

## 项目文档

- [规格与里程碑验收](<docs/project_spec.md>)
- [计划与当前进度](<docs/PROJECT.md>)
- [协作规则](<AGENTS.md>)
