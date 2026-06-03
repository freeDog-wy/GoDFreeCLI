# GoDFreeCLI

> 一个用 Go 编写的轻量级、可扩展的自主 AI 代理框架，在终端中运行。

GoDFreeCLI 是一个在终端中运行的 AI 代理。它通过 Claude API 驱动的 ReAct（推理 + 行动）循环，能够自主执行 shell 命令、操作文件、控制浏览器、加载按需技能、委派子代理，并接入 MCP 生态系统的外部工具。

## ✨ 特色

### 🧠 模型无关的 LLM 抽象

不直接耦合 Anthropic SDK。核心定义了一套通用的 `Provider` 接口和事件系统，只需实现接口即可接入任意 LLM 提供者（OpenAI、本地模型等）。事件系统包含 13 种细粒度事件类型，覆盖文本流、工具调用、思考过程、服务器端工具结果等全部流程。

### 🔄 内联 ReAct 循环

与大多数 CLI 框架在主循环中处理工具执行不同，GoDFreeCLI 将完整的 ReAct 循环实现在 Provider 内部。调用者只需消费一个扁平化的事件流，工具调用的循环、重试、轮次管理全部由 Provider 内部处理，极大简化了上层代码。

### 🔧 特殊工具拦截

`ask_user`（询问用户）、`use_skill`（激活技能）、`delegate_task`（委派子代理）三种特殊工具不会被直接执行，而是被框架拦截为特定事件并暂停循环。消费者处理完毕后将结果注入下一轮对话——对模型来说，它们和普通工具毫无区别，但对框架来说却能执行用户交互、技能加载和子代理调度等特殊逻辑。

### 📁 基于文件的技能系统

技能是自包含的能力模块，可为代理注入系统提示和额外工具。除了代码中注册的内置技能外，框架还能从 `skills/` 目录动态加载技能——每个技能是一个包含 YAML frontmatter + Markdown 正文的 `skill.md` 文件。非开发者无需触碰 Go 代码即可扩展代理能力：

```markdown
---
name: hello-world
description: A friendly greeting skill
enabled: true
---

When the user greets you, respond warmly and offer to help with their tasks.
```

### 🤖 子代理委派

通过 `delegate_task` 工具，模型可将工作卸载到拥有独立上下文和可选受限工具集的隔离子代理。子代理在独立 goroutine 中运行自己的 ReAct 循环，仅返回最终摘要。适用于：

- **并行工作**：多个子代理同时处理独立子任务
- **上下文隔离**：防止工具调用噪音膨胀主对话
- **令牌节省**：子代理使用更少轮次和过滤后的工具集

### 🔌 MCP（模型上下文协议）集成

实现了完整的 MCP 客户端，可连接任何兼容 MCP 的工具服务器。通过 `MCP_SERVERS` 环境变量配置外部服务，其工具会被自动发现并注册。这使 GoDFreeCLI 能无缝接入 MCP 生态中的文件系统、数据库、API 网关等各类工具服务器。

```bash
$env:MCP_SERVERS = '[{"command":"npx","args":["-y","@anthropic/mcp-server-filesystem","."]}]'
```

### 🪝 生命周期钩子系统

5 个可注入的回调覆盖 LLM 调用全生命周期：

| 钩子 | 时机 |
|------|------|
| `BeforeRequest` | 每次 API 调用前，可修改或中止请求 |
| `AfterResponse` | 响应完成后，记录用量和停止原因 |
| `OnEvent` | 每个事件产生时的副作用处理 |
| `BeforeToolCall` | 工具执行前，可修改输入或跳过执行 |
| `AfterToolCall` | 工具执行后，记录结果 |

钩子支持按请求级别覆盖，字段级合并——调试、日志、监控全部可插拔。

### 🌐 浏览器自动化

基于 `chromedp` 提供完整的 Chrome 浏览器控制能力，包含 10 个工具：导航、截图、内容提取、执行 JS、点击、输入、页面信息、等待、滚动、标签页管理。支持无头/可见模式切换、并发安全标签页操作、模拟人类延迟。

### 🖥️ 跨平台 Shell 执行

自动检测操作系统：Windows 上使用 `cmd /c`，Unix 上使用 `sh -c`。内置超时管理（默认 2 分钟，最大 10 分钟）、退出码处理、结构化输出。大文件搜索自动跳过超过 10MB 的文件。

### 🗜️ 轻量级零依赖配置

无 `config.yaml`、无 `.env` 模板、无 JSON 配置文件。所有配置通过环境变量完成，下载二进制即可运行。

## 🚀 快速开始

### 前置条件

- Go 1.26+
- 一个 [Anthropic API Key](https://console.anthropic.com/)

### 安装与运行

```bash
# 克隆仓库
git clone https://github.com/freeDog-wy/GoDFreeCLI.git
cd GoDFreeCLI

# 编译
go build -o gdf ./cmd/gdf

# 设置 API Key
$env:ANTHROPIC_API_KEY = "sk-ant-..."   # Windows PowerShell
export ANTHROPIC_API_KEY="sk-ant-..."    # Linux/macOS

# 启动
./gdf
```

### 对话

```
You: 帮我看看当前目录下有哪些文件
Claude: 我来帮你列出当前目录的文件...
```

输入 `/exit` 或 `/quit` 退出。

### 启用详细日志

```bash
./gdf --verbose
# 或
./gdf -v
```

详细模式会打印每次 API 调用前后的请求/响应信息、工具执行日志、思考过程，以及已注册的工具和技能数量。

### 接入 MCP 服务器

```bash
$env:MCP_SERVERS = '[{"command":"npx","args":["-y","@anthropic/mcp-server-filesystem","/path/to/dir"]}]'
./gdf
```

### 使用其他 API 端点

```bash
$env:ANTHROPIC_BASE_URL = "https://your-proxy.com/v1"
./gdf
```

## 📁 项目结构

```
GoDFreeCLI/
├── cmd/gdf/main.go              # CLI 入口点，REPL 交互循环
├── internal/
│   ├── llm/                     # LLM 抽象层（Provider 接口、事件系统、钩子、Session）
│   │   ├── anthropic/           # Anthropic Provider 实现
│   │   ├── llm.go               # 核心类型定义
│   │   ├── hook.go              # 生命周期钩子
│   │   └── session.go           # 多轮对话管理
│   ├── tool/                    # 工具系统（定义 + 执行器）
│   │   ├── bash.go / bash_exec.go
│   │   ├── file.go / file_exec.go
│   │   ├── browser.go / browser_exec.go
│   │   ├── delegate.go
│   │   └── executor.go
│   ├── agent/                   # 子代理引擎
│   ├── registry/                # 统一工具注册表
│   ├── skill/                   # 技能系统
│   │   ├── skill.go             # Skill 接口
│   │   ├── loader.go            # 基于文件的技能加载器
│   │   ├── registry.go          # 技能注册表
│   │   └── builtin/             # 内置技能
│   └── mcp/                     # MCP 客户端
├── skills/                      # 基于文件的技能目录
│   └── hello-world/skill.md     # 示例技能
├── go.mod
└── go.sum
```

## 🛠️ 技术栈

| 组件 | 技术 |
|------|------|
| 语言 | Go 1.26 |
| LLM SDK | `anthropic-sdk-go` v1.46 |
| 浏览器自动化 | `chromedp` v0.15 |
| MCP 协议 | `modelcontextprotocol/go-sdk` v1.6 |
| 配置解析 | `yaml.v3` |

## 📝 许可证

MIT License

---

🤖 由 [Claude Code](https://claude.com/claude-code) 辅助构建。
