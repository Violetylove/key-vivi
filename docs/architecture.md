# KeyVivi 架构设计：弃用 Fyne，自绘叠加层

**状态**：已评审，按此开发  
**日期**：2026-10-02  
**取代**：`internal/ui`（Fyne 装配层）  
**验收依据**：[project_spec.md](<project_spec.md>) 的 M05、M10 两组用例

---

## 1. 目标与非目标

**目标**

1. 字幕支持**真正的整窗半透明**、**圆角**、**按内容自适应宽度**。
2. 支持**真正的淡入淡出**（淡入 100ms、停留 1500ms、淡出 300ms）。
3. 把 exe 从 23.3 MB 降到约 4 MB 量级。
4. 不再向 `%APPDATA%\fyne`、`%LOCALAPPDATA%\fyne`、`%TEMP%` 写入任何文件，绿色分发不再需要妥协条款。
5. 渲染逻辑纯 Go、可单元测试——不再依赖桌面才能验证。

**非目标（本阶段不做）**

- 配置界面、可选风格、鼠标可视化、多显示器跟随（属 v2.0）。
- 代码签名。
- 把 `launch.bat` 的环境约束去掉（那是 DSH 沙箱行为，与本设计无关）。

---

## 2. 为什么弃用 Fyne

Fyne 当初是作为 walk 的替代品引入的，理由是「API 简洁、内置透明与动画」。其中**「内置透明」对叠加层这个场景不成立**：Fyne v2.8.1 没有任何整窗透明 API，而整窗半透明与淡入淡出只能靠 `WS_EX_LAYERED`，该样式会让 GLFW 的 OpenGL 交换不被合成——实测窗口自报 `IsWindowVisible=True`、所有 Win32 调用成功，屏幕上一个像素都不画。

Fyne 现在只承担托盘图标与窗口管理两件事，而这两件的 Win32 管道本项目已经写好（消息循环、钩子、热键、显示器几何、DPI）。它留下的成本是：

| 成本 | 量化 |
|---|---|
| 体积 | 23.3 MB，而自绘约 4 MB，直接违背「轻量」 |
| 用户目录写入 | 违反「绿色」，规格里为此写了妥协条款 |
| 无法实现 v1.0 视觉 | 「美观」卡死 |

**结论**：收益只剩省编码功夫，成本却压在三条核心要求上，故移除。

---

## 3. 保留 / 替换 / 删除

| 模块 | 处置 | 说明 |
|---|---|---|
| `internal/keyboard` | **保留** | 输入状态机，纯逻辑，已覆盖单测 |
| `internal/display` | **保留** | FIFO 队列与暂停状态，纯逻辑，已覆盖单测 |
| `internal/platform` 的钩子、热键、完整性检测 | **保留** | `keyboard_windows.go`、`hotkey_windows.go`、`token_windows.go` 不动 |
| `internal/platform/overlay_windows.go` | **重写** | 改为分层窗口原语，样式设置与定位逻辑复用 |
| `internal/platform` 新增 | **新增** | 消息窗口、托盘、定时器 |
| `internal/ui` | **删除** | 整体由 `internal/app` + `internal/render` 取代 |
| Fyne 依赖 | **删除** | 最后一步 `go mod tidy` |

---

## 4. 目标架构

```
cmd/keyvivi/main.go                程序入口，只调用 app.Run()

internal/keyboard/                 输入状态机（不变）
  state.go

internal/display/                  显示状态（不变）
  queue.go       FIFO 队列，各项独立到期
  controller.go  暂停、快捷键过滤、把事件转成待显示文本

internal/render/                   纯 Go 位图渲染，无 Win32，可单测
  theme.go       颜色、字号、内边距、圆角、间距
  text.go        字体加载、按 DPI 缓存 face、测量与绘制
  surface.go     RGBA 画布：圆角矩形、整体 alpha 混合
  bar.go         由队列内容生成字幕位图（对外唯一入口）

internal/platform/                 Win32 原语
  keyboard_windows.go   低级键盘钩子（不变）
  hotkey_windows.go     全局热键（不变）
  token_windows.go      完整性级别（不变）
  window_windows.go     隐藏消息窗口 + UI 线程消息循环（新增）
  layered_windows.go    分层叠加窗口：Apply 位图 / 定位 / 隐藏（重写自 overlay）
  tray_windows.go       Shell_NotifyIcon + 弹出菜单（新增）
  overlay_windows.go    显示器工作区与 DPI 查询（保留定位计算）

internal/app/                     装配层，不含 Fyne
  app.go          装配与主循环
  animation.go    淡入淡出状态机
  options.go      KEYVIVI_* 环境开关
```

**依赖方向**（单向，无环）：

```
cmd ─▶ app ─▶ display ─▶ keyboard
        │        │
        │        └────────▶ (无)
        ├────▶ render      （纯计算，不 import 其它内部包）
        └────▶ platform ─▶ keyboard
```

`render` 不 import `platform`，`platform` 不 import `render`：前者产出 `*image.RGBA`，后者只接收位图与坐标。

---

## 5. 关键设计决策

### 5.1 叠加层用分层窗口 + `UpdateLayeredWindow`

**决策**：窗口样式 `WS_EX_LAYERED | WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE | WS_EX_TRANSPARENT | WS_EX_TOPMOST`，内容由我们自己提供的 32 位 ARGB 位图通过 `UpdateLayeredWindow` 一次性提交。

**依据**：这是 Windows 上唯一能同时提供逐像素 alpha、点击穿透、不抢焦点的组合。`SetLayeredWindowAttributes` 只能整窗统一 alpha，不够；且不能和 GL 共用（本项目的原始问题）。

**注意**：`UpdateLayeredWindow` 要求传入源 DC 与位图，尺寸/位置在同一次调用里生效，因此**改内容、改大小、改位置都走同一个入口**，不存在窗口与位图不同步的中间态。

### 5.2 文字与图形全部用 Go 光栅化

**决策**：用 `golang.org/x/image/font/opentype` 解析系统字体，`x/image/font` 的 `Drawer` 画字；圆角矩形自己画。

**依据**：不引入 GDI+ 绑定，渲染成为纯函数 → 可单测、可离线比对像素。代价是新增一个直接依赖 `golang.org/x/image`（Fyne 移除后总体依赖反而变少）。

**字体**：按顺序尝试 `%WINDIR%\Fonts\Deng.ttf`、`msyh.ttc`、`simhei.ttf`，第一个解析成功者生效；全部失败则退回内置的极简位图字形，保证不崩。

### 5.3 单个 UI 线程拥有全部窗口与状态

**决策**：主 goroutine `LockOSThread`，创建隐藏消息窗口与分层叠加窗口，跑 `MsgWaitForMultipleObjects` 消息循环。队列状态、动画状态、托盘状态全部只在该线程读写。

**依据**：Win32 窗口有线程亲和性；上一版因跨协程更新而反复出问题（`fyne.Do`、channel 语义、`doShowAgain` 覆盖坐标）。单线程模型把这类问题从根上消除。

键盘钩子仍在其自己的线程（系统要求），只向 UI 线程投递事件，不持有状态。

### 5.4 单一时钟驱动动画与到期

**决策**：需要刷新时启动 16ms 定时器（`SetTimer`），每个 tick 依次：排空钩子事件 → 过期队列项 → 推进动画 → 重绘。无事可做时关闭定时器并隐藏窗口。

**依据**：避免"到期用 30ms ticker、动画另起 60fps"的双时钟竞争；也是唯一的 CPU 占用点，空闲时为 0。

### 5.5 托盘用 `Shell_NotifyIcon`

**决策**：`NIM_ADD/NIM_MODIFY/NIM_DELETE` + `CreatePopupMenu`/`TrackPopupMenu`，回调消息发到隐藏消息窗口。

**依据**：Fyne 的 systray 在受限令牌下写临时图标文件失败——这条依赖被彻底去掉。图标数据用内存位图 `CreateIconIndirect` 生成，不落盘。

---

## 6. 接口草案

```go
// internal/render
type Theme struct {
    TextColor color.RGBA // 默认 #FFFFFF
    BarColor  color.RGBA // 默认 rgba(10,13,18,0.75)，按直通 alpha 书写
    FontSize  float64    // 逻辑像素，默认 18
    PaddingX  float64    // 左右内边距，默认 24
    PaddingY  float64    // 上下内边距，默认 12
    Radius    float64    // 圆角半径，默认 8
    Separator string     // 队列项之间的分隔符，默认 "  |  "
    MinWidth  float64    // 宽度下限，默认 200
}

// Bar 把要显示的内容渲染成一张带 alpha 的字幕位图，宽度按内容自适应。
// scale 为 DPI 缩放（1.0 = 96dpi），alpha 用于淡入淡出。
func Bar(items []string, th Theme, alpha, scale float64) (*image.RGBA, error)

// Measure 只算尺寸不绘制，供窗口提前定位。
func Measure(items []string, th Theme, scale float64) (width, height int, err error)

// Fit 返回能在 maxWidth 内放下的队尾子集：超宽时从最旧的一项开始丢弃，
// 因为最新输入最重要。宽度上限由调用方按工作区宽度给出。
func Fit(items []string, th Theme, scale, maxWidth float64) []string

var ErrNoItems error // 没有可显示内容


// internal/platform
type LayeredWindow struct{ /* … */ }

// NewLayeredWindow 必须在 UI 线程调用。
func NewLayeredWindow() (*LayeredWindow, error)

// Apply 同时提交位图、位置与尺寸；传 nil 隐藏窗口。
func (w *LayeredWindow) Apply(img *image.RGBA, x, y int) error
func (w *LayeredWindow) Destroy()

// Loop 在调用线程跑消息循环，并驱动 16ms 时钟。
type Loop struct{ /* … */ }

// Run 阻塞直到 Quit。tick 返回 true 表示仍需刷新，false 则关闭时钟。
func Run(onReady func(), tick func(now time.Time) bool, onQuit func()) error


// internal/app
func Run() // 装配全部组件并进入 UI 循环
```

---

## 7. 渲染规范

| 项 | 值 | 来源 |
|---|---|---|
| 背景色 | `rgba(10, 13, 18, 0.75)` | 规格 3.2 |
| 文字色 | `#FFFFFF` | 规格 3.2 |
| 字号 | 18 逻辑像素 | 规格 3.2 |
| 圆角 | 8 逻辑像素 | 规格 3.2 |
| 内边距 | 上下 12、左右 24 逻辑像素 | 规格 3.2 |
| 队列项间隔 | 24 逻辑像素，中间用 `\|` 分隔 | 本项目约定 |
| 宽度 | 按内容自适应，下限 200、上限为工作区宽度减两侧各 120 | 本设计 |
| 高度 | 字号 + 上下内边距 | 本设计 |
| 位置 | 主显示器工作区底部居中，距底 80 逻辑像素 | 规格 3.1 |

**DPI**：所有「逻辑像素」乘以 `GetDpiForWindow/96` 后取整；文字用同样的 scale 请求 face，保证清晰。`WM_DPICHANGED` 或 `WM_DISPLAYCHANGE` 时重新测量与定位。

---

## 8. 动画规范

| 阶段 | 时长 | 说明 |
|---|---|---|
| 淡入 | 100ms | alpha 0 → 1 |
| 停留 | 1500ms | alpha 1 |
| 淡出 | 300ms | alpha 1 → 0 |

- 新输入到达时：更新内容、alpha 立即置 1（或从当前值续上）、重新开始停留计时。
- 队列为空且淡出结束时：`Apply(nil)` 隐藏窗口并停表。
- 队列非空时窗口始终可见，alpha 只受淡入淡出影响。
- 动画期间每 16ms 一帧；100ms ≈ 7 帧、300ms ≈ 19 帧，肉眼连续。

---

## 9. 线程与消息模型

| 线程 | 职责 | 约束 |
|---|---|---|
| 主 goroutine（`LockOSThread`） | 隐藏消息窗口、分层窗口、定时器、托盘、队列与动画状态 | 唯一读写窗口与状态的地方 |
| 钩子线程（`LockOSThread`） | `WH_KEYBOARD_LL` + 自己的消息泵 | 只向 channel 投递事件，不碰窗口 |
| 热键线程（复用 `hotkey_windows.go`） | `RegisterHotKey` + 消息泵 | 回调只投递通知 |

**唤醒方式**：钩子/热键投递后 `SetEvent`，UI 线程在 `MsgWaitForMultipleObjects` 上等待该事件与窗口消息；被唤醒后排空 channel 并处理。

**消息窗口承载**：托盘回调（`WM_APP+1`）、热键（若后续统一到该窗口）、`WM_DISPLAYCHANGE`、`WM_DPICHANGED`、`WM_TIMER`、退出消息。

---

## 10. 生命周期

1. **启动**：解析 `KEYVIVI_*` 开关 → 校验完整性级别并记日志 → 创建消息窗口与分层窗口 → 装钩子 → 注册热键 → 添加托盘图标 → 显示启动提示（默认 1.5 秒，`KEYVIVI_READY_HINT` 可调，0 关闭）。
2. **空闲**：队列为空、提示已过、动画结束 → 隐藏窗口并停表，CPU 占用为 0。
3. **退出**：托盘"退出"、`Ctrl+Alt+Q`（托盘不可用时的兜底）或窗口关闭 → 停表 → `Shell_NotifyIcon(NIM_DELETE)` → 卸载钩子与热键 → 销毁窗口 → 退出进程。**不写任何文件。**

---

## 11. 迁移步骤

每一步都必须可编译、可验证，坏了一步就回退这一步。

| 步 | 内容 | 验证方式 |
|---|---|---|
| 1 | 新建 `internal/render`（theme/text/surface/bar）+ 单测 | `go test ./tests/unit`：尺寸随内容、圆角外透明、整体 alpha 线性、DPI 缩放、Fit 丢最旧、空输入报错 |
| 2 | `internal/platform` 新增 `window_windows.go`（消息窗口 + `Run` 循环）与 `layered_windows.go` | 集成测试：窗口样式、`Apply` 返回成功、位置在底部居中 |
| 3 | `internal/platform` 新增 `tray_windows.go` | 集成测试：图标可添加/更新/删除不报错；菜单项构造正确 |
| 4 | 新建 `internal/app`（装配 + 动画 + 开关），`cmd/keyvivi/main.go` 切到 `app.Run()`；`internal/ui` 暂留但不再被引用 | 构建成功；`launch.bat` 启动后按键、托盘、暂停、动画全部人工确认 |
| 5 | 删除 `internal/ui`；`go mod tidy` 移除 Fyne；更新文档与验收表 | exe 体积、无 AppData 写入、全量测试、规格条款回滚 |

**回退点**：第 4 步之前旧路径完全不受影响；第 4 步若失败，把 `main.go` 切回 `ui.Run()` 即可。

---

## 12. 验收映射

| 用例 | 本设计如何满足 |
|---|---|
| M05-01 托盘菜单 | `tray_windows.go`，图标不落盘 |
| M05-02 暂停 / Ctrl+Alt+K | `hotkey_windows.go` 复用，暂停清空队列 |
| M05-04 无边框、置顶、穿透、不抢焦点 | 分层窗口样式（保留现有组合，去掉 `WS_EX_LAYERED` 的旧断言改为必需） |
| M05-06 FIFO 队列 | `display` 不变，渲染改为按项自适应宽度 |
| M10-01 缩放与不截断 | 全部尺寸走 DPI scale；宽度上限为工作区宽减 240 |
| M10-02 淡入淡出 | 第 8 节动画状态机 |
| M10-03 目标机运行 | 无 Fyne、无 CGO 之外的依赖；仍需 CGO 编译，但产物无外部依赖 |
| M10-04 绿色与隐私 | 不写任何文件，规格中原「接受 Fyne 写入」条款回滚 |

**规格需要同步修改**：`project_spec.md` 第 3.4 节的「接受框架写入并如实说明」决策作废，改回「不写程序目录之外的文件」；M10-06（Fyne 路径清理）随之删除。

---

## 13. 风险与回退

| 风险 | 应对 |
|---|---|
| `UpdateLayeredWindow` 在个别机器上失败 | `Apply` 返回错误即记日志并降级为普通窗口显示（不透明但可用），不静默失败 |
| 系统字体全部解析失败 | 退回内置位图字形，只求可读 |
| 文字渲染在非整数 DPI 下偏糊 | 按 scale 请求 face（`FaceOptions.Size` 为逻辑字号、`DPI` 为 96*scale），并允许 `KEYVIVI_FONT_SCALE` 覆盖 |
| 托盘在受限令牌下仍失败 | 保留现有兜底：静默注册 `Ctrl+Alt+Q`，只写日志 |
| 迁移期间两套 UI 并存导致误改 | 第 4 步前不删除 `internal/ui`，但它必须保持不被引用；第 5 步一次性删除 |
