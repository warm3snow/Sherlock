# Sherlock

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.18+-00ADD8?logo=go)](https://golang.org/)
[![GitHub stars](https://img.shields.io/github/stars/warm3snow/sherlock?style=social)](https://github.com/warm3snow/sherlock/stargazers)

[English](README.md) | [中文](README_zh.md)

## 🔍 AI-Powered SSH Remote Operations Tool

Sherlock is an AI-based remote operations tool built on SSH. Interact with remote hosts using **natural language** - no need to memorize complex shell commands.

### ✨ Key Features

| Category | Features |
|----------|----------|
| **🗣️ Natural Language** | Describe tasks in plain English/Chinese, AI translates to shell commands |
| **🤖 AI Operations** | Intelligent advisor, auto-diagnosis, one-click fixes, automated playbooks |
| **🧠 AI Enhanced** | Multi-turn dialogue with memory, proactive analysis, command prediction, Tool Calling |
| **📊 Visualization** | Terminal ASCII dashboard, multi-host status matrix, health scoring |
| **⚡ Batch Operations** | Execute commands across multiple hosts with progress tracking |
| **🔐 Security** | Auto SSH key management, encrypted credentials, operation audit logs |
| **📁 File Transfer** | SFTP with progress tracking, resume capability, recursive operations |

### 🏗️ Architecture

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
│  │  │(NLP+Mem)│  │(Proact.)│  │ (批量)  │  │ (监控)  │           │        │
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

### 🚀 Quick Start

```bash
# Install
git clone https://github.com/warm3snow/sherlock.git
cd sherlock && go build -o sherlock ./cmd/sherlock

# Run
./sherlock

# Connect (natural language)
sherlock> connect to 192.168.1.100 as root

# Execute commands (natural language)
sherlock[root@192.168.1.100]> show me disk usage
sherlock[root@192.168.1.100]> find large files over 1GB

# AI Operations
sherlock> advisor              # AI intelligent advisor
sherlock> inspect              # Health inspection
sherlock> playbook run daily-inspect  # Run playbook
sherlock> dashboard            # Visual dashboard
sherlock> audit stats          # Audit statistics

# AI Enhanced Features
sherlock> predict              # Get AI command predictions
sherlock> memory status        # View conversation memory
sherlock> memory clear         # Clear current session
sherlock> playbook generate deploy a nginx service  # AI generate playbook
sherlock> playbook template    # List playbook templates
sherlock> playbook improve daily-inspect  # AI improve existing playbook
```

### ⚙️ Configuration

Create `~/.config/sherlock/config.json`:

#### Ollama (Local, No API Key Required)
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

#### Other Providers (Qwen, Moonshot, Zhipu, etc.)
```json
{
  "llm": {
    "provider": "qwen",
    "api_key": "your-api-key",
    "model": "qwen-plus"
  }
}
```

#### Full Configuration Example
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

**Supported LLM Providers:**

| Provider | API Key Required | Default Model | Base URL |
|----------|-----------------|---------------|----------|
| `ollama` | ❌ No | `qwen2.5:latest` | `http://localhost:11434` |
| `deepseek` | ✅ Yes | `deepseek-chat` | `https://api.deepseek.com` |
| `openai` | ✅ Yes | `gpt-4o` | `https://api.openai.com/v1` |
| `qwen` | ✅ Yes | `qwen-plus` | `https://dashscope.aliyuncs.com/compatible-mode/v1` |
| `moonshot` | ✅ Yes | `moonshot-v1-8k` | `https://api.moonshot.cn/v1` |
| `zhipu` | ✅ Yes | `glm-4` | `https://open.bigmodel.cn/api/paas/v4` |
| `groq` | ✅ Yes | `llama-3.1-70b-versatile` | `https://api.groq.com/openai/v1` |
| `together` | ✅ Yes | `Meta-Llama-3.1-70B` | `https://api.together.xyz/v1` |
| `openai_compatible` | ✅ Yes | (custom) | (custom) |

**AI Enhanced Config:**

| Option | Default | Description |
|--------|---------|-------------|
| `enable_memory` | `true` | Multi-turn dialogue with session memory |
| `enable_proactive_analysis` | `true` | Auto-analyze command output on errors |
| `enable_prediction` | `true` | Command intent prediction |
| `enable_tool_calling` | `false` | Allow AI to call SSH tools autonomously |
| `memory_window_size` | `20` | Sliding window size (messages) |

### 📋 Commands Reference

| Command | Description |
|---------|-------------|
| `connect <host>` | Connect to host (natural language supported) |
| `$<cmd>` | Execute shell command directly |
| `advisor` | AI intelligent operations advisor |
| `inspect [group]` | Batch health inspection |
| `playbook [run <name>]` | Manage/run automated playbooks |
| `dashboard` | Visual resource dashboard |
| `audit [stats]` | Operation audit logs |
| `quickfix` | AI-driven quick fix |
| `batch <cmd>` | Execute on multiple hosts |
| `tunnel` | SSH tunnel management |
| `snippet` | Command template management |
| `predict` | AI command intent prediction |
| `memory [status\|clear\|new\|history]` | Conversation memory management |
| `playbook generate <desc>` | AI generate playbook from description |
| `playbook template` | List predefined playbook templates |
| `playbook improve <name>` | AI suggest improvements for playbook |

### 📁 Project Structure

```
sherlock/
├── cmd/sherlock/      # CLI application
├── internal/
│   ├── advisor/       # 🆕 AI intelligent advisor
│   ├── agent/         # NLP + Memory + Predictor + Tool Calling
│   ├── ai/            # LLM client (Ollama/OpenAI/DeepSeek)
│   ├── analyzer/      # Output analysis & proactive diagnosis
│   ├── audit/         # 🆕 Operation audit logs
│   ├── batch/         # Batch operations
│   ├── dashboard/     # 🆕 Visual dashboard
│   ├── healthcheck/   # 🆕 Health inspection
│   ├── playbook/      # 🆕 Automated playbooks
│   ├── session/       # Multi-session management
│   ├── tunnel/        # SSH tunneling
│   └── ...
└── pkg/sshclient/     # SSH client implementation
```

### 📄 License

Apache License 2.0

---

<p align="center">
  <a href="https://github.com/warm3snow/sherlock/stargazers">⭐ Star us on GitHub</a> •
  <a href="https://github.com/warm3snow/sherlock/issues">🐛 Report Bug</a> •
  <a href="https://github.com/warm3snow/sherlock/pulls">🔧 Contribute</a>
</p>
