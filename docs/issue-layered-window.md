# 分层窗口排查

本文维护诊断方法。位图契约见[设计文档](architecture.md#分层窗口与定位)，已确认结论与验证状态见[项目规格](project_spec.md#验证证据)。

## 证据边界

| 检查 | 能确认 | 不能确认 |
|---|---|---|
| `UpdateLayeredWindow` 成功 | 系统接受位图提交 | 用户桌面已可见 |
| 窗口尺寸、坐标和可见标记 | 窗口几何与状态 | 桌面合成结果 |
| DIB 像素回读 | 位图内存内容 | 可见性、穿透和焦点 |
| 真实桌面人工操作 | 当前环境的实际表现 | 所有目标机器均可用 |

代理截图或 `GetPixel` 缺失时，先用同环境普通窗口作对照，不据此移除分层样式或推断渲染错误。

## 排查步骤

1. 记录启动路径、Windows 版本、窗口站和 DPI。
2. 检查样式、API 返回值、DIB 尺寸、预乘 BGRA、工作区和坐标。
3. 检查事件唤醒、帧时钟及资源是否提前释放。
4. 从普通桌面直接运行 exe，人工确认可见性、透明度、穿透与焦点。
5. 在规格中记录通过、失败、环境受限或未测试，附步骤和证据。

## 验证入口

```powershell
go test -race -tags integration -run 'TestLayeredWindowAcceptsRenderedBar|TestOverlayPositionUsesWorkAreaAndDPI' ./tests/integration
```

原生回归确认提交、样式与几何；实际可见性通过普通桌面的人工操作确认。
