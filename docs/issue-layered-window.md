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

1. 记录启动路径、exe 与进程完整性级别、Windows 版本、窗口站和 DPI。
2. 检查样式、API 返回值、DIB 尺寸、预乘 BGRA、工作区和坐标。
3. 检查事件唤醒、帧时钟及资源是否提前释放。
4. 从普通桌面直接运行 exe，人工确认可见性、透明度、穿透与焦点。
5. 在规格中记录通过、失败、环境受限或未测试，附步骤和证据。

## 低完整性启动

Git 不传输 Windows 完整性标签。从远端克隆到普通目录后，标准构建无需调整权限；如果克隆或输出目录本身带 Low 标签，新 exe 仍可能继承 Low。

使用 `icacls .`、`icacls .\dist` 和 `icacls .\dist\KeyVivi.exe` 检查文件标签。若只有本地受限工作区造成问题，可将输出路径改为普通用户目录；需要保留原路径时，只修正已确认的单个产物：

```powershell
icacls .\dist\KeyVivi.exe /setintegritylevel M
```

标签修正不提高当前进程权限，仍需从普通桌面重新启动。不要调整工作区目录安全设置或通过管理员运行绕过检查。Windows 规则见[强制完整性控制](https://learn.microsoft.com/en-us/windows/win32/secauthz/mandatory-integrity-control)。

## 验证入口

```powershell
go test -race -tags integration -run 'TestLayeredWindowAcceptsRenderedBar|TestOverlayPositionUsesWorkAreaAndDPI' ./tests/integration
```

`KEYVIVI_SHOW_OVERLAY=1` 可启用 `TestOverlayPreview` 和 `TestOverlayProbeSelfCheck`。像素探针结果需结合普通窗口对照与人工操作解释，不能替代产品验收。
