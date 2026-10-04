# KeyVivi

简洁的 Windows 按键可视化工具，采用 Go、Win32 分层窗口与 Go 位图渲染。目标为单 exe、免安装、半透明键帽、托盘控制。

**当前为 M2.1 开发版本，配置与设置已接入，正式桌面及发布验收待完成。** 默认在主屏工作区左下角显示，距左侧、底部各24逻辑像素；最多3行，不限制每行元素数量。连续输入向右追加，停顿700ms或超出最大行宽时换行，旧行向上移动；两次连按保留两个键帽，第三次合并计数。半透明键帽置顶、穿透、不抢焦点。详细规则与进度见 [项目规格](docs/project_spec.md)。

托盘“设置…”打开原生窗口，**显示区域、外观与布局、行为与动画**按三个小节纵向排列，每行一个配置项，左侧名称与说明、右侧控件，共16项。位置使用六项下拉菜单（顶部与底部各三项）；水平、垂直边距始终向内留白，上中/下中忽略水平值，切换位置保留数字；设置主题提供Catppuccin Mocha深色与Latte浅色，圆角输入框、下拉菜单和胶囊开关统一配色，切换先预览、保存后持久化。顶部位置示意与键帽预览、底部操作栏固定，配置内容独立滚动，不显示系统滚动条，滚轮与Tab聚焦滚动仍可用。底部“恢复默认”旁提供“打开配置文件”按钮，不展示配置路径。保存后立即应用并保持窗口打开，显示“设置已保存”；取消或关闭仅放弃后续未保存的草稿，恢复默认须保存才生效；保存失败保留草稿和原配置。窗口支持Tab导航、回车保存、Esc取消，焦点移入不可见项时自动滚动，DPI变化时重新排布。

文字与背景颜色通过可点击色块选择，旁边显示当前色值。点击打开系统RGB选色器，确认后更新示例预览，取消保留原色；保存设置后才写入YAML。主题字段为 `appearance.settings_theme: mocha/latte`，缺失时使用Mocha。位置仅支持顶部和底部六项。

**直接双击 `dist\KeyVivi.exe` 启动，日常运行只需要exe。** 启动时从实际exe同目录读取 `keyvivi.yaml`，首次缺失会尝试生成带中文注释的默认文件；错误配置提示后使用默认值，保留原文件。手动编辑文件后重启加载，更新exe不会覆盖配置。字段、范围及保存规则见 [规格第3.5节](docs/project_spec.md#35-yaml-配置与设置窗口)。

## 目录

| 路径 | 职责 |
|---|---|
| `cmd/keyvivi/` | 程序入口 |
| `internal/app/` | 应用装配与生命周期 |
| `internal/keyboard/` | 修饰键、组合键、连按状态 |
| `internal/display/` | 三行 FIFO、行内追加与整体续期、暂停与控制快捷键过滤 |
| `internal/render/` | 字体、按行键帽布局、完整键帽快照与预乘 alpha 合成 |
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
.\build.ps1
~~~

`-race` 在 Windows 上需要启用 CGO 和可用的 C 编译器；该要求只针对竞态检查。`build.ps1` 仅用于开发构建，临时关闭CGO并恢复环境，再将单个产物exe的文件完整性标签设为Medium。它不启动程序，也不复制到其他目录；生成的exe可直接双击运行。

集成测试会注册全局热键、创建窗口并注入 F24 按下/释放事件，需在交互桌面显式运行：

~~~powershell
go test -race -tags integration ./internal/app ./tests/integration
~~~

本地渲染预览可显式设置 `KEYVIVI_ANIMATION_PREVIEW=1` 后执行 `go test ./internal/app -run 'Preview$' -count=1`；纵向行布局预览 `vertical-groups-preview.png`、明暗背景键帽预览、动画 GIF、关键帧和 DPI/超宽 PNG 写入忽略的 `tests/artifacts/`。预览来自实际渲染器，不创建桌面窗口，不能代替真实桌面的穿透、焦点和动画观感验收。

测量消息循环帧间隔可设置 `KEYVIVI_MEASURE_CADENCE=1`，执行 `go test -tags integration ./tests/integration -run '^TestLoopCadence$' -count=1 -v -timeout=15s`。输出 120 个间隔的平均值和分位数，不包含内容渲染与 DWM 合成。

## 启动与运行环境

历史A/B/A/B记录中的工作区内Low（`0x1000`）、工作区外Medium（`0x2000`）差异，已定位为构建exe继承了工作区的Low文件标签。普通桌面启动也会降为Low，表现为托盘不可用、全局按键不显示；[Windows完整性规则](https://learn.microsoft.com/en-us/windows/win32/secauthz/mandatory-integrity-control)解释了这种启动降权。构建流程只修正产物exe，保留工作区目录的安全设置。

启动入口统一为 `KeyVivi.exe`，旧启动脚本已移除。程序在装配前检查实际令牌；低于Medium时显示明确原因并退出，避免继续以“托盘失效但后台存活”的状态运行。已在本机从其他工作目录直接运行 `dist\KeyVivi.exe`，确认Medium、托盘/热键注册及F24钩子自检通过；真实桌面外观与完整发布验收仍待完成。历史 `%LOCALAPPDATA%\KeyVivi` 副本不会自动清理或更新，可退出后自行删除；该目录中的配置不会自动覆盖当前exe同目录配置。

托盘提供暂停/继续、设置、关于、退出；`Ctrl+Alt+K` 切换暂停。快捷键占用时提示改用托盘；托盘不可用时注册 `Ctrl+Alt+Q` 退出兜底。无法建立必要控制入口时清理资源并结束，显示错误说明。上述路径已接入，实际菜单观感和 Explorer 重启恢复仍需真实桌面验证。

## 存储与隐私

运行不记录、不上传按键内容，不设置自启动、不创建服务、不写注册表配置；字幕、字体缓存、托盘图标在内存中处理，系统字体从Windows字体目录只读加载。仅为设置保存写入实际exe同目录的 `keyvivi.yaml` 和同目录临时文件，不写按键记录或缓存。

存储范围、退出清理及目标机器运行仍需发布验收，详见 [规格](docs/project_spec.md)。卸载时退出程序并删除exe及配置文件。暂停可以避免后续敏感输入被展示，不能删除已录入视频的字幕，也不自动识别密码框。

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
