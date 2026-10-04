# KeyVivi 协作规则

更新日期：2026-10-05（Asia/Shanghai）。

## 文档职责

- [README](README.md) 介绍项目与基本使用，不承载实现细节或开发日志。
- [设计文档](docs/architecture.md) 维护架构、实现决策和技术约束。
- [项目规格](docs/project_spec.md) 维护需求、里程碑、进度和验收证据。
- [贡献规范](docs/CONTRIBUTING.md) 维护目录、编码、构建、测试和提交要求。
- [窗口排查](docs/issue-layered-window.md) 维护诊断步骤；结论与验证状态写入规格。
- 文档简洁、准确、职责单一。同类信息只维护一份，通过链接引用。

## 开发约束

- 技术路线为 Go + Win32 + Go 位图渲染，入口为 `cmd/keyvivi/main.go` → `internal/app.Run`。
- 修改前读取文件，增量修改；未经授权不得删除代码重写。
- 注释使用精炼中文，解释原因与约束；日志使用英文，不记录按键内容。
- 遵循[贡献规范](docs/CONTRIBUTING.md)，完成一个功能或收工时统一提交。
- 输入状态、非阻塞钩子、UI 线程亲和性、显式唤醒、预乘位图及资源清理遵循[设计文档](docs/architecture.md)。
- 构建使用 `build.ps1`，仅修正产物 exe 的 Medium 标签；保留工作区安全设置，不自行提权。

## 验证约束

- 提交前执行 `gofmt -w .`、`go vet ./...`、`go test -race ./...`，检查 `git diff --cached`。
- 原生集成测试显式执行，会注入 F24 并验证 Low 进程拒绝；命令见贡献规范。
- 编译、API 成功和离屏预览不能替代真实桌面验收；旧实现反馈不能证明当前实现通过。
- 进度使用规格中的 M1.1–M3.2 编号，必需场景全部通过才标记完成。
- 日期采用 Asia/Shanghai；只记录实际测量与验证结果，不编造耗时、资源占用或完成状态。
