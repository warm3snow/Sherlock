# Sherlock

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.18+-00ADD8?logo=go)](https://golang.org/)
[![GitHub stars](https://img.shields.io/github/stars/warm3snow/sherlock?style=social)](https://github.com/warm3snow/sherlock/stargazers)

[English](README.md) | [中文](README_zh.md)

## 🔍 基于 AI 的 SSH 远程运维工具

Sherlock 是一款 AI 驱动的远程运维工具，底层基于 SSH。使用**自然语言**与远程主机交互，无需记忆复杂的 shell 命令。

### ✨ 核心特性

| 类别 | 功能 |
|------|------|
| **🗣️ 自然语言交互** | 用中文/英文描述任务，AI 自动转换为 shell 命令 |
| **🤖 AI 智能运维** | 智能诊断、问题分析、一键修复、自动化剧本 |
| **🧠 AI 深度增强** | 多轮对话记忆、主动分析预警、命令意图预测、Tool Calling |
| **📊 可视化仪表盘** | 终端 ASCII 图表、多主机状态矩阵、健康评分 |
| **⚡ 批量操作** | 多主机并行执行命令，支持进度跟踪 |
| **🔐 安全管理** | 自动 SSH 密钥管理、凭据加密、操作审计 |
| **📁 文件传输** | SFTP 支持进度显示、断点续传、递归操作 |

### 🏗️ 系统架构

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Sherlock CLI                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐        │
│  │   Advisor   │  │  Playbook   │  │  Dashboard  │  │    Audit    │        │
│  │ (AI分析诊断) │  │ (自动化剧本) │  │ (可视化面板) │  │  (操作审计)  │        │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘        │
│         │                │                │                │               │
│  ┌──────┴────────────────┴────────────────┴────────────────┴──────┐        │
│  │                        Core Engine                              │        │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐           │        │
│  │  │  Agent  │  │ Analyzer│  │  Batch  │  │ Monitor │           │        │
│  │  │(NLP+记忆)│  │(主动分析)│  │ (批量)  │  │ (监控)  │           │        │
│  │  └────┬────┘  └────┬────┘  └────┬────┘  └────┬────┘           │        │
│  └───────┼────────────┼────────────┼────────────┼────────────────┘        │
│          │            │            │            │                          │
│  ┌───────┴────────────┴────────────┴────────────┴────────────────┐        │
│  │                     Infrastructure Layer                       │        │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐           │        │
│  │  │SSH Client│  │ Tunnel  │  │Transfer │  │ Session │           │        │
│  │  └─────────┘  └─────────┘  └─────────┘  └─────────┘           │        │
│  └────────────────────────────────────────────────────────────────┘        │
└─────────────────────────────────────────────────────────────────────────────┘
         │                              │                              │
         ▼                              ▼                              ▼
┌─────────────────┐          ┌─────────────────┐          ┌─────────────────┐
│  Remote Hosts   │          │   LLM Services  │          │   Local Storage │
│  (SSH/SFTP)     │          │ Ollama/OpenAI/  │          │  (SQLite/JSON)  │
│                 │          │    DeepSeek     │          │                 │
└─────────────────┘          └─────────────────┘          └─────────────────┘
```

### 🚀 快速开始

```bash
# 安装
git clone https://github.com/warm3snow/sherlock.git
cd sherlock && go build -o sherlock ./cmd/sherlock

# 运行
./sherlock

# 连接主机（自然语言）
sherlock> 连接到 192.168.1.100 用户名 root

# 执行命令（自然语言）
sherlock[root@192.168.1.100]> 查看磁盘使用情况
sherlock[root@192.168.1.100]> 找出大于1GB的文件

# AI 运维功能
sherlock> advisor              # AI 智能运维助手
sherlock> inspect              # 一键健康巡检
sherlock> playbook run daily-inspect  # 执行运维剧本
sherlock> dashboard            # 可视化仪表盘
sherlock> audit stats          # 审计统计

# AI 增强功能
sherlock> predict              # AI 命令意图预测
sherlock> memory status        # 查看对话记忆状态
sherlock> memory clear         # 清除当前会话记忆
sherlock> playbook generate 部署一个 nginx 服务  # AI 生成运维剧本
sherlock> playbook template    # 查看预置剧本模板
sherlock> playbook improve daily-inspect  # AI 优化已有剧本
```

### ⚙️ 配置

创建配置文件 `~/.config/sherlock/config.json`：

#### Ollama（本地部署，无需 API Key）
```json
{
  "llm": {
    "provider": "ollama",
    "base_url": "http://localhost:11434",
    "model": "qwen2.5:7b"
  }
}
```

#### DeepSeek
```json
{
  "llm": {
    "provider": "deepseek",
    "api_key": "sk-your-deepseek-api-key",
    "model": "deepseek-chat"
  }
}
```

#### OpenAI
```json
{
  "llm": {
    "provider": "openai",
    "api_key": "sk-your-openai-api-key",
    "model": "gpt-4o"
  }
}
```

#### 其他国产大模型（通义千问、Moonshot、智谱等）
```json
{
  "llm": {
    "provider": "qwen",
    "api_key": "your-api-key",
    "model": "qwen-plus"
  }
}
```

#### 完整配置示例
```json
{
  "llm": {
    "provider": "deepseek",
    "api_key": "sk-your-api-key",
    "base_url": "https://api.deepseek.com",
    "model": "deepseek-chat",
    "temperature": 0.7
  },
  "ssh_key": {
    "private_key_path": "~/.ssh/id_ed25519",
    "public_key_path": "~/.ssh/id_ed25519.pub",
    "auto_add_to_remote": true
  },
  "ui": {
    "theme": "dracula"
  },
  "ai_enhanced": {
    "enable_memory": true,
    "enable_proactive_analysis": true,
    "enable_prediction": true,
    "enable_tool_calling": false,
    "memory_window_size": 20,
    "max_command_history": 100,
    "analyze_on_error": true,
    "analyze_on_warning": true
  }
}
```

**支持的 LLM 提供商：**

| 提供商 | 需要 API Key | 默认模型 | API 地址 |
|--------|-------------|----------|----------|
| `ollama` | ❌ 否 | `qwen2.5:latest` | `http://localhost:11434` |
| `deepseek` | ✅ 是 | `deepseek-chat` | `https://api.deepseek.com` |
| `openai` | ✅ 是 | `gpt-4o` | `https://api.openai.com/v1` |
| `qwen` | ✅ 是 | `qwen-plus` | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| `moonshot` | ✅ 是 | `moonshot-v1-8k` | `https://api.moonshot.cn/v1` |
| `zhipu` | ✅ 是 | `glm-4` | `https://open.bigmodel.cn/api/paas/v4` |
| `baichuan` | ✅ 是 | `Baichuan4` | `https://api.baichuan-ai.com/v1` |
| `minimax` | ✅ 是 | `abab6.5s-chat` | `https://api.minimax.chat/v1` |
| `yi` | ✅ 是 | `yi-large` | `https://api.lingyiwanwu.com/v1` |
| `groq` | ✅ 是 | `llama-3.1-70b-versatile` | `https://api.groq.com/openai/v1` |
| `together` | ✅ 是 | `Meta-Llama-3.1-70B` | `https://api.together.xyz/v1` |
| `openai_compatible` | ✅ 是 | (自定义) | (自定义) |

**AI 增强配置项：**

| 配置项 | 默认值 | 说明 |
|--------|--------|------|
| `enable_memory` | `true` | 多轮对话记忆，支持跨会话上下文 |
| `enable_proactive_analysis` | `true` | 命令执行后自动分析异常输出 |
| `enable_prediction` | `true` | 基于历史和上下文的命令意图预测 |
| `enable_tool_calling` | `false` | 允许 AI 自主调用 SSH 工具（需手动开启） |
| `memory_window_size` | `20` | 对话滑动窗口大小（消息数） |

### 📋 命令参考

| 命令 | 说明 |
|------|------|
| `connect <主机>` | 连接主机（支持自然语言） |
| `$<命令>` | 直接执行 shell 命令 |
| `advisor` | AI 智能运维助手 |
| `inspect [分组]` | 批量健康巡检 |
| `playbook [run <名称>]` | 管理/执行自动化剧本 |
| `dashboard` | 可视化资源仪表盘 |
| `audit [stats]` | 操作审计日志 |
| `quickfix` | AI 驱动的快速修复 |
| `batch <命令>` | 多主机批量执行 |
| `tunnel` | SSH 隧道管理 |
| `snippet` | 命令模板管理 |
| `predict` | AI 命令意图预测 |
| `memory [status\|clear\|new\|history]` | 对话记忆管理 |
| `playbook generate <描述>` | AI 根据描述生成运维剧本 |
| `playbook template` | 查看预置运维剧本模板 |
| `playbook improve <名称>` | AI 优化建议已有剧本 |

### 📁 项目结构

```
sherlock/
├── cmd/sherlock/      # CLI 应用入口
├── internal/
│   ├── advisor/       # 🆕 AI 智能运维助手
│   ├── agent/         # NLP + 对话记忆 + 意图预测 + Tool Calling
│   ├── ai/            # LLM 客户端 (Ollama/OpenAI/DeepSeek)
│   ├── analyzer/      # 输出分析与主动诊断
│   ├── audit/         # 🆕 操作审计日志
│   ├── batch/         # 批量操作
│   ├── dashboard/     # 🆕 可视化仪表盘
│   ├── healthcheck/   # 🆕 健康巡检
│   ├── playbook/      # 🆕 自动化运维剧本
│   ├── session/       # 多会话管理
│   ├── tunnel/        # SSH 隧道
│   └── ...
└── pkg/sshclient/     # SSH 客户端实现
```

### 📄 开源协议

Apache License 2.0

---

<p align="center">
  <a href="https://github.com/warm3snow/sherlock/stargazers">⭐ 给项目点个 Star</a> •
  <a href="https://github.com/warm3snow/sherlock/issues">🐛 反馈问题</a> •
  <a href="https://github.com/warm3snow/sherlock/pulls">🔧 参与贡献</a>
</p>
