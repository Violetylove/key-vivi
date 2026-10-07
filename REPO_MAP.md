# 仓库导航

更新日期：2026-10-07（Asia/Shanghai）。构建与测试命令见[贡献规范](docs/CONTRIBUTING.md)。

## 入口与实现

| 路径 | 职责 |
|---|---|
| [cmd/keyvivi/main.go](cmd/keyvivi/main.go) | 启动应用、显示运行失败提示 |
| [internal/app](internal/app) | 应用装配、会话、动画场景、YAML 配置与设置映射 |
| [internal/keyboard](internal/keyboard) | 物理键名、修饰键状态、组合与连按 |
| [internal/display](internal/display) | 输入队列、行容量、暂停、期限与动画状态 |
| [internal/render](internal/render) | 字体、键帽布局、预乘位图合成、主题与托盘图标 |
| [internal/platform](internal/platform) | Win32 钩子、消息循环、窗口、热键、托盘、设置控件与文件替换 |

调用链为 `main → app.Run → platform.Run`。输入从钩子进入会话和队列，场景调用渲染模块生成位图，再由分层窗口提交。线程与资源约束见[设计文档](docs/architecture.md)。

## 测试与发布

| 路径 | 职责 |
|---|---|
| [tests/unit](tests/unit) | 输入、队列、动画、渲染与应用单元回归 |
| [tests/unit/testdata](tests/unit/testdata) | 依赖未导出实现的 app/render 同包测试源文件 |
| [tests/integration](tests/integration) | Win32、设置窗口和实际 exe 的显式集成回归 |
| [tests/integration/testdata](tests/integration/testdata) | 设置装配的同包集成测试源文件 |
| [tests/testutil](tests/testutil) | 用 Go overlay 运行集中存放的同包测试，并继承竞态检查 |
| [.github/workflows/release.yml](.github/workflows/release.yml) | 标签触发检查、构建和 GitHub Release 上传 |
| [go.mod](go.mod)、[go.sum](go.sum) | Go 版本与依赖 |
| `dist/`、`tests/artifacts/` | 本地产物与验证证据，Git 忽略 |

生产源码目录只放实现。`testdata` 不参与 Go 的常规包枚举，测试入口通过 overlay 临时将其加入原包，不复制到 `internal`，也不引入测试专用产品接口。

## 文档

- [README](README.md)：功能、下载和使用。
- [项目规格](docs/project_spec.md)：需求、进度与已执行验证。
- [设计文档](docs/architecture.md)：架构和实现约束。
- [贡献规范](docs/CONTRIBUTING.md)：开发、检查、构建、发布和提交。
- [窗口排查](docs/issue-layered-window.md)：分层窗口诊断方法。
- [协作规则](AGENTS.md)：协作与文档维护要求。
