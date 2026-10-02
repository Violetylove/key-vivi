# 求助：`UpdateLayeredWindow` 返回成功，但窗口在屏幕上不可见

**一句话**：Go 通过 `NewLazyProc` 直接调 Win32，用 `WS_EX_LAYERED` + `UpdateLayeredWindow` 做逐像素 alpha 的桌面叠加层；所有 API 都返回成功、`GetWindowRect` 几何正确、`IsWindowVisible` 为真、DIB 内存内容确认正确，**但屏幕上一个像素都不出现**。

---

## 1. 环境

| 项 | 值 |
|---|---|
| 系统 | Windows |
| 显示缩放 | 125%（DPI 120） |
| 语言/调用方式 | Go，`windows.NewLazyDLL` + `NewProc`，无 CGO |
| 进程 DPI 感知 | 未声明（DPI unaware） |
| 渲染方式 | **没有 OpenGL / DirectX**，画面完全由我们自己写入 DIB 后提交 |

---

## 2. 目标

一个覆盖在其它窗口之上的字幕条：无边框、置顶、点击穿透、不抢焦点、背景半透明、圆角。这需要逐像素 alpha，因此选 `UpdateLayeredWindow`。

---

## 3. 关键代码

窗口创建（在一条 `runtime.LockOSThread()` 的 UI 线程上，该类有正常消息循环）：

```go
hwnd, _, _ := createWindowEx.Call(
    wsExLayered|wsExToolWindow|wsExNoActivate|wsExTransparent|wsExTopmost, // 0x80000|0x80|0x08000000|0x20|0x8
    uintptr(unsafe.Pointer(className)), // "KeyViviWindow"
    0,
    wsPopup,          // 0x80000000，创建时没有 WS_VISIBLE，尺寸 0x0
    0, 0, 0, 0,
    0, 0, 0, 0,
)

window.screenDC, _, _ = getDC.Call(0)                     // GetDC(NULL)
window.memDC, _, _    = createCompatibleDC.Call(window.screenDC)
```

提交画面：

```go
type bitmapInfoHeader struct {
    size          uint32 // 40
    width         int32  // w
    height        int32  // -h  → top-down
    planes        uint16 // 1
    bitCount      uint16 // 32
    compression   uint32 // BI_RGB = 0
    sizeImage     uint32
    xPelsPerMeter int32
    yPelsPerMeter int32
    clrUsed       uint32
    clrImportant  uint32
}
type bitmapInfo struct {
    header bitmapInfoHeader
    colors [1]uint32
}

var bits unsafe.Pointer
bitmap, _, _ := createDIBSection.Call(
    window.screenDC, uintptr(unsafe.Pointer(&info)), 0 /*DIB_RGB_COLORS*/,
    uintptr(unsafe.Pointer(&bits)), 0, 0)
oldBitmap, _, _ := selectObject.Call(window.memDC, bitmap)
// …把预乘 BGRA 逐行写入 bits（B,G,R,A 字节序）…

type blendFunction struct { blendOp, blendFlags, sourceConstantAlpha, alphaFormat byte }
blend := blendFunction{blendOp: 0 /*AC_SRC_OVER*/, sourceConstantAlpha: 255, alphaFormat: 1 /*AC_SRC_ALPHA*/}
dst  := point{x, y}
size := winSize{cx: w, cy: h}
src  := point{}   // 0,0
ret, _, err := updateLayeredWindow.Call(
    hwnd, window.screenDC,
    uintptr(unsafe.Pointer(&dst)),
    uintptr(unsafe.Pointer(&size)),
    window.memDC,
    uintptr(unsafe.Pointer(&src)),
    0,                                   // crKey
    uintptr(unsafe.Pointer(&blend)),
    2,                                   // ULW_ALPHA
)   // ret == 1，err == "The operation completed successfully."

showWindow.Call(hwnd, 4) // SW_SHOWNOACTIVATE
setWindowPos.Call(hwnd, ^uintptr(0) /*HWND_TOPMOST*/, x, y, w, h, 0x0010|0x0040 /*NOACTIVATE|SHOWWINDOW*/)
```

窗口过程只透传：

```go
func windowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
    switch message {
    case wmAppWake, 0x0113 /*WM_TIMER*/, 0x007E, 0x02E0: /* 自定义唤醒、定时器、显示变化 */
        return 0
    }
    ret, _, _ := defWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
    return ret
}
```

---

## 4. 观测到的证据

| 检查 | 结果 |
|---|---|
| `UpdateLayeredWindow` 返回值 | **1（成功）** |
| `GetLastError` | 0 |
| `GetWindowRect(hwnd)` | 与传入的 `pptDst` / `psize` **完全一致**（说明调用确实生效了） |
| `IsWindowVisible(hwnd)` | 真 |
| 扩展样式 `GetWindowLongW(hwnd, -20)` | 含 `WS_EX_LAYERED / TOOLWINDOW / NOACTIVATE / TRANSPARENT / TOPMOST` |
| DIB 内存回读（中心像素） | `[B=13, G=9, R=7, A=191]` —— **正是我们写入的预乘半透明背景色** |
| 屏幕上的像素 | **没有任何变化**（改前改后逐位相同） |
| 人工目视 | 看不到任何东西 |

---

## 5. 已排除的变量

1. **窗口样式**：把扩展样式减到只剩 `WS_EX_LAYERED`，症状不变（不是某个样式位造成的）。
2. **`ShowWindow` 与 `UpdateLayeredWindow` 的顺序**：两种顺序都试过。
3. **额外强制显示**：补 `SetWindowPos(HWND_TOPMOST, …, SWP_SHOWWINDOW)` 无效。
4. **`SetLayeredWindowAttributes`**：从未调用，也不打算调用（要和 `UpdateLayeredWindow` 二选一）。
5. **位置与尺寸**：改成固定坐标 + 纯不透明纯红位图，仍然不可见（排除 alpha 太低的可能）。
6. **DPI**：这条路径不涉及任何缩放计算，且用固定坐标复现。
7. **位图内容**：DIB 回读确认就是我们写的像素（见上表）。
8. **`BLENDFUNCTION`**：`{AC_SRC_OVER, 0, 255, AC_SRC_ALPHA}` 配合 `ULW_ALPHA`，是标准写法。
9. **消息循环**：UI 线程确实在跑 `GetMessage`/`DispatchMessage`。
10. **窗口类**：`RegisterClassExW` 成功（`cbSize` = 80，`hbrBackground` 为 NULL）。

---

## 6. 具体想请教的问题

1. 是否必须**先让窗口可见**（`ShowWindow` / `WS_VISIBLE`）**之后**才能 `UpdateLayeredWindow`？还是相反？我们两种顺序都试了，但想确认哪一种才是契约。
2. `CreateDIBSection` 的 `hdc` 参数是否必须是 `CreateCompatibleDC(NULL)` 的 DC，而**不能**是 `GetDC(NULL)` 的屏幕 DC？
3. `WS_EX_TRANSPARENT` 是否与分层窗口的逐像素 alpha 冲突？
4. `WS_EX_LAYERED` 是否必须在窗口**首次显示之前**设置？（我们的窗口是先 `CreateWindowEx` 带上该样式，再 `UpdateLayeredWindow`，最后 `ShowWindow`。）
5. DIB 是否必须是 bottom-up（正 `biHeight`）？我们用 top-down（负值）。
6. 是否遗漏了某个前置调用（例如 `SetWindowLongW` 重设样式、`DwmEnableBlurBehindWindow`、per-monitor DPI 感知声明）？
7. 有没有可能问题出在 **进程 DPI unaware + 125% 缩放**这个组合上（窗口被放到缩放后的坐标系外）？用固定坐标也没显示，所以直觉上不是，但请指教怎么验证。

---

## 7. 最小复现

仓库内有一个自校验探针，**同一进程里并排放两个窗口**：

- 一个**普通窗口**，背景刷为纯红（`CreateSolidBrush(0x000000FF)`）——**对照组**；
- 一个**分层窗口**，位图为纯红。

然后各自回读屏幕像素。这样能区分两种截然不同的结论：如果连普通窗口都读不到红色，是**探测方法**不可靠；如果普通窗口能读到、分层窗口读不到，则分层窗口确实没画到屏幕上。

```powershell
$env:KEYVIVI_SHOW_OVERLAY="1"
go test -tags integration -run TestOverlayProbeSelfCheck -v ./tests/integration
```

（这一步的结果我们还没拿到——之前的验证里我用错过一次对照，把"别的窗口的白色像素"当成了"我的窗口可见"，因此特意把对照组做进了同一个测试。）
