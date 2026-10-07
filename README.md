# KeyVivi

Windows 按键可视化工具，适用于录屏、演示和教学。以半透明键帽展示单键、组合键和重复次数，提供托盘控制与可视化设置。

## 功能

- 连续输入按行显示，自动换行与淡入淡出。
- 字号、颜色、间距、屏幕位置和动画可配置。
- 设置窗口提供 Catppuccin Mocha / Latte 主题、颜色选择和实时预览。
- 托盘管理暂停、设置和退出，`Ctrl+Alt+K` 切换暂停。
- 配置保存在本地，不记录或上传按键内容。

## 使用

目标平台为 Windows 10/11 x64。可从[发行页面](https://github.com/Violetylove/key-vivi/releases)下载正式版 `KeyVivi.exe`，无需安装或管理员权限。

下载后直接运行 `KeyVivi.exe`，源码构建产物位于 `dist`。程序在 exe 同目录生成 `keyvivi.yaml`，可通过托盘打开设置窗口调整；手动修改文件后需重启。

暂停可停止后续按键展示，敏感输入前应主动暂停。

## 开发与文档

项目使用 Go 与 Win32，通过位图渲染键帽，保持原生窗口和单 exe 交付。

- [贡献规范](docs/CONTRIBUTING.md)：构建、测试、编码和提交。
- [仓库导航](REPO_MAP.md)：源码、测试与发布入口。
- [设计文档](docs/architecture.md)：模块职责、数据流和实现约束。
- [项目规格](docs/project_spec.md)：需求、里程碑、进度和验收。
- [窗口排查](docs/issue-layered-window.md)：分层窗口的诊断步骤。
