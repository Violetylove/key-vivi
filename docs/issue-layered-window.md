# 分层叠加窗口的可见性诊断记录

更新日期：2026-10-04（Asia/Shanghai）。本记录保留自绘窗口调试中的观测和验证限制，不代表产品缺陷已定位或桌面验收已通过。

## 当前实现

使用 Go 直接调用 Win32：`WS_EX_LAYERED` 分层窗口接收 `UpdateLayeredWindow` 提交的 top-down、32 位预乘 BGRA 位图。窗口创建与提交位于固定 UI 线程，有原生消息循环，不使用额外图形渲染引擎。

目标行为为半透明字幕、圆角、置顶、鼠标穿透和不抢焦点。分层样式是该方案的必需项，不能因截图里没有画面就移除。

## 已有观测与结论边界

| 观测 | 可以证明 | 不能证明 |
|---|---|---|
| `UpdateLayeredWindow` 返回成功 | 系统接受了该次位图提交 | 真实桌面上已可见 |
| `GetWindowRect` 与提交尺寸/坐标一致 | 窗口几何正确 | 位置在用户的实际显示区域内 |
| `IsWindowVisible` 为真 | 窗口可见状态已设置 | 桌面合成结果或用户目视结果 |
| DIB 中心像素回读符合 BGRA | 位图内存有预期内容 | 画面被合成到用户桌面 |
| 同一代理进程的普通窗口也不出现在捕获中 | 捕获方法存在环境限制 | 分层窗口绘制失败 |
| 人在普通桌面启动后观察 | 当前运行环境中的真实外观 | 所有目标机器均可用 |

此前的捕获对照记录显示：代理启动的普通窗口和分层窗口都可能无法被截图或 `GetPixel` 看见。具体隔离机制尚未完全确认。因此撤回“API 全部成功但分层窗口一定没画”的推断，改以普通桌面人工验收为准。

## 排查顺序

1. 记录启动路径、完整性级别、窗口站、Windows 版本和缩放；本代理工作区的运行位置限制见 [README](../README.md)。
2. 检查窗口样式、API 返回值、DIB 尺寸、预乘 BGRA 字节、工作区与 DPI 坐标。
3. 验证事件确实唤醒 UI、定时器未意外停止、资源未在初始化回调返回时提前销毁。
4. 在普通桌面由资源管理器运行，人工确认可见性、半透明、焦点与点击穿透。
5. 记录为“通过 / 失败 / 环境受限 / 未测试”，保留步骤和证据；环境受限不能改记为功能通过。

## 仓库内验证入口

`TestLayeredWindowAcceptsRenderedBar` 验证创建、样式、位图提交、几何与隐藏。`TestOverlayPositionUsesWorkAreaAndDPI` 验证定位计算。`TestOverlayPreview` 为人工预览，需显式设置 `KEYVIVI_SHOW_OVERLAY`。

`TestOverlayProbeSelfCheck` 是可选屏幕像素诊断探针，不能在隔离桌面中充当产品可见性验收；其探测结果需与普通窗口对照及真实桌面观测一起解释。

~~~powershell
go test -race -tags integration -run 'TestLayeredWindowAcceptsRenderedBar|TestOverlayPositionUsesWorkAreaAndDPI' ./tests/integration
~~~

真实产品验收用例、进度和验证记录统一见 [项目规格](project_spec.md)。
