# KeyVivi

简洁的 Windows 按键可视化工具，采用 Go、Win32 分层窗口与 Go 位图渲染。目标为单 exe、免安装、半透明键帽、托盘控制。

**当前为 M2.1 开发版本，配置与设置已接入，正式桌面及发布验收待完成。** 默认在主屏工作区左下角显示，距左侧、底部各24逻辑像素；最多3组，每组最多8个完整输入。连续输入向右追加，停顿700ms或行满换组，旧组向上移动；两次连按保留两个键帽，第三次合并计数。半透明键帽置顶、穿透、不抢焦点。详细规则与进度见 [项目规格](docs/project_spec.md)。

托盘“设置…”打开原生窗口，分为**显示区域、外观与布局、行为与动画**三组，提供九宫格位置、偏移、布局及计时等16项设置，附位置示意和键帽预览。保存后立即应用并关闭窗口，取消或关闭放弃草稿，恢复默认须保存才生效；保存失败保留草稿和原配置。窗口支持Tab导航、回车保存、Esc取消，空间不足可滚动，DPI变化时重新排布。

启动时从实际exe同目录读取 `keyvivi.yaml`，首次缺失会尝试生成带中文注释的默认文件；错误配置提示后使用默认值，保留原文件。手动编辑文件后重启加载。开发时通过 `launch.bat` 运行，配置位于 `%LOCALAPPDATA%\KeyVivi\keyvivi.yaml`，更新exe不会覆盖配置。字段、范围及保存规则见 [规格第3.5节](docs/project_spec.md#35-yaml-配置与设置窗口)。

## 目录

| 路径 | 职责 |
|---|---|
| `cmd/keyvivi/` | 程序入口 |
| `internal/app/` | 应用装配与生命周期 |
| `internal/keyboard/` | 修饰键、组合键、连按状态 |
| `internal/display/` | 三组 FIFO、组内追加与整体续期、暂停与控制快捷键过滤 |
| `internal/render/` | 字体、分组键帽布局、完整键帽快照与预乘 alpha 合成 |
| `internal/platform/` | Win32 消息循环、分层窗口、键盘钩子、热键、托盘与显示器几何 |
| `tests/unit/` | 输入、队列与渲染测试 |
| `tests/integration/` | Windows API 与交互桌面测试 |
| `tests/artifacts/` | 本地测试证据，Git 忽略 |
| `docs/` | 项目规格与开发计划、实现设计、贡献规范及诊断记录 |
| `dist/` | 构建产物，Git 忽略 |

## 构建与测试

目标平台为 Windows 10/11 x64。使用 [go.mod](go.mod) 指定的 Go 版本，普通构建不需要 CGO 或 C 编译器。直接依赖为 `golang.org/x/image`、`golang.org/x/sys` 与 `go.yaml.in/yaml/v3`。

在 Windows PowerShell 中执行：

~~~powershell
go vet ./...
go test -race ./...
$env:CGO_ENABLED = '0'
go build -ldflags="-s -w -H=windowsgui" -o dist/KeyVivi.exe ./cmd/keyvivi
Remove-Item Env:CGO_ENABLED
~~~

`-race` 在 Windows 上需要启用 CGO 和可用的 C 编译器，因此在设置 `CGO_ENABLED=0` 之前运行；该要求只针对竞态检查。

集成测试会注册全局热键、创建窗口并注入 F24 按下/释放事件，需在交互桌面显式运行：

~~~powershell
go test -race -tags integration ./internal/app ./tests/integration
~~~

本地渲染预览可显式设置 `KEYVIVI_ANIMATION_PREVIEW=1` 后执行 `go test ./internal/app -run 'Preview$' -count=1`；纵向分组预览 `vertical-groups-preview.png`、明暗背景键帽预览、动画 GIF、关键帧和 DPI/超宽 PNG 写入忽略的 `tests/artifacts/`。预览来自实际渲染器，不创建桌面窗口，不能代替真实桌面的穿透、焦点和动画观感验收。

测量消息循环帧间隔可设置 `KEYVIVI_MEASURE_CADENCE=1`，执行 `go test -tags integration ./tests/integration -run '^TestLoopCadence$' -count=1 -v -timeout=15s`。输出 120 个间隔的平均值和分位数，不包含内容渲染与 DWM 合成。

## 启动与运行环境

在本代理工作区内，之前的 A/B/A/B 实测发现 exe 运行位置会影响完整性级别：工作区内为 Low（`0x1000`），工作区外为 Medium（`0x2000`）。Low 进程无法可靠采集更高完整性应用的输入。此结论是本开发环境记录，不是所有 Windows 机器的普遍规则。

开发时使用 [launch.bat](launch.bat)，它将 `dist\KeyVivi.exe` 复制到 `%LOCALAPPDATA%\KeyVivi` 再启动。这份副本属于启动脚本的写入；卸载时关闭进程并删除该目录。发布验收应将 exe 复制到普通目录，由资源管理器启动。

托盘提供暂停/继续、设置、关于、退出；`Ctrl+Alt+K` 切换暂停。快捷键占用时提示改用托盘；托盘不可用时注册 `Ctrl+Alt+Q` 退出兜底。无法建立必要控制入口时清理资源并结束，显示错误说明。上述路径已接入，实际菜单观感和 Explorer 重启恢复仍需真实桌面验证。

## 存储与隐私

运行不记录、不上传按键内容，不设置自启动、不创建服务、不写注册表配置；字幕、字体缓存、托盘图标在内存中处理，系统字体从Windows字体目录只读加载。仅为设置保存写入实际exe同目录的 `keyvivi.yaml` 和同目录临时文件，不写按键记录或缓存。

存储范围、退出清理及目标机器运行仍需发布验收，详见 [规格](docs/project_spec.md)。卸载时退出程序并删除exe及配置文件；使用启动器时还需删除开发副本目录。暂停可以避免后续敏感输入被展示，不能删除已录入视频的字幕，也不自动识别密码框。

## 诊断

- `KEYVIVI_DEBUG=1`：现有平台层输出钩子和位图提交信息，不输出键名或文本。
- `KEYVIVI_READY_HINT`：启动提示到期秒数，可用小数；到期后淡出 300ms，0 关闭，默认 1.5 秒，非法值或超出 0–3600 秒回退默认值。关闭提示后仍正常唤醒输入。
- `KEYVIVI_SELFTEST=1`：显式注入一次 F24，随后提示钩子回调计数检查结果；默认不注入。

代理启动窗口的截图和 `GetPixel` 结果可能受桌面隔离影响；Win32 API 返回成功仅证明调用与几何，真实可见性、穿透和焦点须人工确认。排查记录见 [叠加层诊断](docs/issue-layered-window.md)。

## 项目文档

- [项目规格与开发计划](docs/project_spec.md)：需求、里程碑、进度和验证记录。
- [设计文档](docs/architecture.md)：架构、模块与实现方式。
- [编码与提交规范](docs/CONTRIBUTING.md)
- [协作规则](AGENTS.md)
