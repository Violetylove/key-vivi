# AGENTS.md - AI 协作开发记录

## 当前有效协作规则（优先于下方历史记录）

- Git 提交信息（标题与正文）全部使用英文，遵循 Conventional Commits：`type(scope): description`。规范见 [CONTRIBUTING.md](<docs/CONTRIBUTING.md>)；提交前检查暂存差异，不提交产物或测试截图。

**校订时间**: 2026-09-30 22:40:28（UTC+08:00，来自本机系统时间）

- 目标与各里程碑验收以 [project_spec.md](<docs/project_spec.md>) 为准；进度和验收证据维护在 [PROJECT.md](<docs/PROJECT.md>)。
- v0.1 核心验证、v0.5 基本可用、v1.0 最小版本发布、v2.0 增强版分别验收；编译成功不能标记里程碑完成。
- 用户已确认按键正常；单元测试和 Windows 钩子集成测试通过。完整里程碑仍按逐项证据验收，下方历史“已完成”标记不作为有效进度。
- 源码放在 cmd/keyvivi 与 internal 下并按 keyboard、platform、ui 模块分工；所有测试放在 tests/unit 或 tests/integration，测试证据在 tests/artifacts，文档在 docs，产物在 dist。禁止将源码、测试或 exe 堆在根目录。
- 普通测试用 go test -race ./...；真实桌面钩子测试显式使用 go test -race -tags integration ./tests/integration，会注入 F24 事件。
- 按下/释放状态决定组合输入，不用 300ms 超时猜测意图；左右修饰键分别追踪，组合输入后保留仍按住的键。
- 后台协程更新 Fyne UI 使用 `fyne.Do` 等调度机制；channel 和 Refresh 不会自动保证 UI 线程安全。
- Fyne 桌面构建涉及 CGO/C 编译器。透明、置顶、穿透需要实测，不能沿用下方历史表格的“原生支持”结论。
- 配置、可选风格、鼠标与多显示器在 v2.0 规划，不作为 C1 最小版本发布门槛。
- 文件修改前读取内容，增量修改；不得未经授权删除代码重写。实际耗时无记录时不编造，日期使用系统时间。

下方为历史协作记录，其中过时方案、未经验证的原因推断和完成声明仅作历史参考，不作为实现指令。

## 项目信息

**项目名称**: KeyVivi  
**开发模式**: AI-Human 协作  
**AI 模型**: claude-opus-5-5  
**开发工具**: DeepSeek Harness  
**开始日期**: 2026-09-30  

## 协作流程

### 开发阶段

#### 阶段 1: 需求分析与技术选型（约 30 分钟）

**人类需求**:
- 构建按键可视化程序
- 显示简洁、美观
- 程序轻量、绿色（无依赖、单 exe）

**AI 贡献**:
1. 头脑风暴三个维度的技术方案：
   - 技术栈：Rust、C# WPF、Tauri
   - 显示风格：胶囊流、键帽风、极简字幕
   - 功能范围：最小可用、加配置、完整版

2. 根据反馈调整方案：
   - 人类拒绝 Rust（环境复杂）
   - 提议 Go + walk 库
   - AI 评估后发现 walk 库限制较多

3. 再次调整：
   - 推荐 Go + Fyne 框架
   - 解释 Fyne 优势：纯 Go、API 简洁、文档完善

**决策结果**:
- 技术栈：Go + Fyne
- 显示风格：极简字幕（可扩展）
- 功能范围：v1.0 最小可用版本

#### 阶段 2: 架构设计与文档编写（约 20 分钟）

**AI 贡献**:
1. 编写详细的 `project_spec.md` 设计文档，包含：
   - 核心功能定义
   - 技术实现细节
   - 配置文件格式
   - 时间预估

2. 设计关键技术方案：
   - 全局键盘钩子（Windows Hook API）
   - 修饰键智能识别逻辑
   - 组合键合并算法
   - 连按检测机制

**输出物**:
- [`project_spec.md`](<docs/project_spec.md>) - 完整设计规格文档

#### 阶段 3: 代码实现（约 2 小时）

**第一次尝试：walk 库**
- 问题：walk 库是 Win32 的薄封装，需要大量手写 syscall
- 遇到的困难：
  - API 不完整（`SetLayeredWindowAttributes` 等需要手动调用）
  - 类型转换复杂（uintptr、常量溢出）
  - 重复声明全局变量
- 决策：放弃 walk，切换到 Fyne

**第二次尝试：Fyne 框架**
- 初始实现：
  - 全局键盘钩子成功
  - UI 窗口创建成功
  - 编译通过（24 MB）

**遇到的问题与解决**:

1. **问题：窗口空白，按键不显示**
   - 原因：键盘钩子回调在钩子线程，Fyne UI 必须在主线程更新
   - 解决：引入 channel 通道，将按键事件发送到主线程处理
   ```go
   // 钩子线程 → channel → 处理协程 → 主线程 UI
   keyChan <- keyEvent{vkCode, isDown}
   ```

2. **问题：修饰键（Ctrl、Shift）不显示**
   - 原因：修饰键被视为"只在组合时显示"
   - 人类反馈：用户意图是关键，单按 Ctrl 也要显示
   - 解决：实现智能延迟判断逻辑
     - 修饰键按下时启动 300ms 定时器
     - 300ms 内按下普通键 → 显示组合键（如 `Ctrl+C`）
     - 300ms 超时 → 显示单个修饰键（如 `Ctrl`）

3. **问题：关闭 bash 后程序还存活**
   - 原因：没有托盘图标，程序在后台运行
   - 临时解决：添加"退出"按钮
   - 待完成：实现托盘图标和全局快捷键

**当前代码结构**:
```
main.go
├── 全局变量和常量定义
├── KeyViviApp 结构体
│   ├── Fyne 窗口和标签
│   ├── 键盘钩子句柄
│   ├── 按键状态（modifiers、lastKey、repeatCount）
│   └── 定时器（modifierTimer、hideTimer）
├── main() - 程序入口
├── createWindow() - 创建 Fyne 窗口
├── installKeyboardHook() - 安装 Windows 键盘钩子
├── keyboardProc() - 钩子回调函数
├── processKeys() - 按键事件处理协程
├── handleKeyDown() - 按键按下处理
├── handleKeyUp() - 按键释放处理
└── 辅助函数（getKeyName、isModifier、contains 等）
```

#### 阶段 4: 测试与迭代（当前）

**人类反馈**:
1. ✅ 能看到按键显示
2. ✅ 有重复按键计数（如 `F ×5`）
3. ❌ 修饰键单独不显示
4. ✅ 窗口 1.5 秒后正常消失

**AI 响应**:
- 解释修饰键逻辑（组合键优先）
- 询问用户意图：单独显示 vs 组合显示
- 人类明确：两种情况都要正确识别
- 实现智能延迟判断逻辑

**最新修改**:
- 添加按键释放事件监听（`WM_KEYUP`、`WM_SYSKEYUP`）
- 修饰键按下时启动定时器
- 普通键按下时取消定时器
- 修饰键释放时从状态列表移除

## 技术决策记录

### 为什么选择 Fyne 而不是 walk？

| 对比项 | walk | Fyne |
|-------|------|------|
| API 封装 | Win32 薄封装，需大量 syscall | 高级抽象，API 简洁 |
| 透明窗口 | 需手写 `SetLayeredWindowAttributes` | 原生支持 |
| 淡入淡出 | 需手动实现 Alpha 动画 | 内置动画系统 |
| 文档质量 | 较少，主要靠源码 | 完善的官方文档 |
| 体积 | 约 3–5 MB | 约 24 MB（内嵌渲染引擎） |
| 开发速度 | 慢，调试困难 | 快，类型安全 |

**结论**: Fyne 的开发效率远高于 walk，体积增加可接受。

### 修饰键智能识别的设计

**挑战**: 如何区分用户的真实意图？
- 单独按 `Ctrl` → 用户想测试 Ctrl 键
- 按住 `Ctrl` 再按 `C` → 用户想复制

**方案对比**:

| 方案 | 优点 | 缺点 |
|------|------|------|
| 立即显示修饰键 | 简单 | 组合键会显示两次（Ctrl、Ctrl+C）|
| 只在组合时显示 | 无重复 | 单按修饰键无反馈 |
| 延迟判断（300ms） | 准确识别意图 | 轻微延迟 |

**选择**: 延迟判断方案，300ms 延迟人类几乎无感知。

### 线程安全设计

**问题**: 键盘钩子在系统钩子线程，Fyne UI 在主线程。

**错误做法**:
```go
// ❌ 在钩子回调直接更新 UI（线程不安全）
func keyboardProc(...) {
    app.label.Text = "A"
    app.label.Refresh()  // 崩溃或无效
}
```

**正确做法**:
```go
// ✅ 通过 channel 传递到主线程
func keyboardProc(...) {
    app.keyChan <- keyEvent{vkCode, isDown}
}

func (app *KeyViviApp) processKeys() {
    for event := range app.keyChan {
        // 这里的代码在独立 goroutine
        app.handleKeyDown(event.vkCode)
    }
}

func (app *KeyViviApp) showTextInternal(text string) {
    // Fyne 自动调度到主线程
    app.label.Text = text
    app.label.Refresh()
}
```

## AI 开发策略

### 增量开发
1. 先实现核心功能（键盘钩子 + 基础显示）
2. 编译并测试
3. 根据人类反馈迭代
4. 逐步添加次要功能（托盘、快捷键）

### 错误处理
- 遇到编译错误时逐个修复，不重写整个文件
- API 不可用时及时切换方案（walk → Fyne）
- 保持代码结构清晰，避免重复声明

### 文档驱动
- 先写设计文档（`project_spec.md`）明确目标
- 编写项目计划（`PROJECT.md`）追踪进度
- 记录协作过程（`AGENTS.md`，本文档）

## 当前状态

### 已实现功能
- ✅ 全局键盘监听（Windows Hook API）
- ✅ 按键实时显示（Fyne UI）
- ✅ 组合键合并（如 `Ctrl+Shift+S`）
- ✅ 连按计数（如 `A ×3`）
- ✅ 修饰键智能识别（300ms 延迟判断）
- ✅ 自动隐藏（1.5 秒）
- ✅ 线程安全的 UI 更新

### 待实现功能
- ⏸ 托盘图标和右键菜单
- ⏸ 全局快捷键 `Ctrl+Alt+K` 暂停/继续
- ⏸ 去除窗口边框（无装饰窗口）
- ⏸ 淡入淡出动画
- ⏸ 配置文件读写

### 已知限制
1. Fyne 窗口默认有标题栏和边框（需 Win32 API 修改）
2. Fyne 窗口无法点击穿透（需 `WS_EX_TRANSPARENT` 样式）
3. 左右修饰键不区分（VK 代码相同）

## 经验总结

### 对人类开发者的建议
1. **明确需求再动手**: 头脑风暴阶段避免过早进入编码
2. **选对工具很重要**: walk 库虽然轻量，但开发效率低
3. **用户意图是核心**: 修饰键的显示逻辑要贴近真实使用场景

### 对 AI 协作的建议
1. **多方案对比**: 提供技术选型时列出优缺点和时间成本
2. **快速验证**: 遇到架构问题时果断切换方案，不死磕
3. **增量迭代**: 核心功能先跑通，再添加次要功能
4. **及时反馈**: 编译成功后立即让人类测试，不要堆积功能

## 下一步计划

1. 测试修改后的修饰键逻辑
2. 实现托盘图标和菜单
3. 添加全局快捷键暂停功能
4. 通过 Win32 API 去除窗口边框
5. 实现淡入淡出动画
6. 添加配置文件支持

## 附录

### 相关文件
- [`project_spec.md`](<docs/project_spec.md>) - 设计规格文档
- [`PROJECT.md`](<docs/PROJECT.md>) - 项目计划书
- [`main.go`](<cmd/keyvivi/main.go>) - 主程序代码
- [`go.mod`](go.mod) - Go 模块依赖

### 依赖库
```
fyne.io/fyne/v2 v2.8.1
golang.org/x/sys/windows
```

### 编译命令
```bash
go build -ldflags="-s -w -H=windowsgui" -o dist/KeyVivi.exe ./cmd/keyvivi
```

### 测试步骤
1. 双击 `KeyVivi.exe` 启动
2. 测试单个字母键（如 `A`、`S`）
3. 测试组合键（如 `Ctrl+C`、`Ctrl+Shift+S`）
4. 测试单独修饰键（如单按 `Ctrl`）
5. 测试连按（如快速按 3 次 `A`）
6. 观察 1.5 秒自动隐藏

---

**文档版本**: v0.1  
**最后更新**: 2026-09-30  
**维护者**: AI (claude-opus-5-5) + Human (Winter)
