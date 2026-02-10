# Sherlock

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
[![Go Version](https://img.shields.io/badge/Go-1.18+-00ADD8?logo=go)](https://golang.org/)
[![GitHub stars](https://img.shields.io/github/stars/warm3snow/sherlock?style=social)](https://github.com/warm3snow/sherlock/stargazers)

[English](README.md) | [中文](README_zh.md)

## 🔍 AI-Powered SSH Remote Operations Tool

Sherlock is an AI-driven remote operations tool built on SSH. Interact with remote hosts using **natural language** - no need to memorize complex shell commands.

### ✨ Key Features

| Category | Features |
|----------|----------|
| **🗣️ Natural Language** | Describe tasks in plain English/Chinese, AI translates to shell commands |
| **🤖 AI Operations** | Intelligent advisor, auto-diagnosis, one-click fixes, automated playbooks |
| **🧠 AI Enhanced** | Multi-turn dialogue memory, proactive analysis, command prediction |
| **📊 Visualization** | Terminal ASCII dashboard, multi-host status matrix, health scoring |
| **⚡ Batch Operations** | Execute commands across multiple hosts with progress tracking |
| **🔐 Security** | Auto SSH key management, encrypted credentials, operation audit logs |
| **📁 File Transfer** | Upload/download with progress, support `--host=<id|alias>` targeting |

### 🏗️ Architecture

```
┌────────────────────────────────────────────────────────────────────────┐
│                           Sherlock CLI                                 │
├────────────────────────────────────────────────────────────────────────┤
│                                                                        │
│   ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐   │
│   │ Advisor  │ │ Playbook │ │Dashboard │ │  Audit   │ │ Inspector│   │
│   │  (AI诊断) │ │ (自动剧本) │ │ (可视化)  │ │ (审计)   │ │  (巡检)  │   │
│   └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘   │
│        └────────────┴────────────┴────────────┴────────────┘          │
│                                  │                                     │
│   ┌──────────────────────────────┴──────────────────────────────┐     │
│   │                      Core Engine                             │     │
│   │  ┌────────────┐  ┌────────────┐  ┌────────────┐             │     │
│   │  │   Agent    │  │  Analyzer  │  │   Batch    │             │     │
│   │  │ NLP+Memory │  │  Proactive │  │  Executor  │             │     │
│   │  └─────┬──────┘  └─────┬──────┘  └─────┬──────┘             │     │
│   └────────┴───────────────┴───────────────┴────────────────────┘     │
│                                  │                                     │
│   ┌──────────────────────────────┴──────────────────────────────┐     │
│   │                   Infrastructure                             │     │
│   │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐        │     │
│   │  │SSH Client│ │ Transfer │ │  Tunnel  │ │  Hosts   │        │     │
│   │  │  (连接)   │ │ (文件传输) │ │  (隧道)  │ │  (主机库) │        │     │
│   │  └──────────┘ └──────────┘ └──────────┘ └──────────┘        │     │
│   └─────────────────────────────────────────────────────────────┘     │
│                                                                        │
└────────────────────────────────────────────────────────────────────────┘
        │                         │                         │
        ▼                         ▼                         ▼
┌───────────────┐       ┌───────────────┐       ┌───────────────┐
│ Remote Hosts  │       │  LLM Service  │       │ Local Storage │
│   SSH/SFTP    │       │ Ollama/OpenAI │       │ SQLite + JSON │
└───────────────┘       └───────────────┘       └───────────────┘
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

# File transfer (support --host=<id|alias>)
sherlock> upload local.txt /root/ --host=myserver
sherlock> download /var/log/app.log ./logs/ --host=1

# AI Operations
sherlock> advisor                          # AI intelligent advisor
sherlock> inspect                          # Health inspection
sherlock> playbook run daily-inspect       # Run playbook
sherlock> playbook generate deploy nginx   # AI generate playbook
```

### ⚙️ Configuration

Create `~/.config/sherlock/config.json`:

```json
{
  "llm": {
    "provider": "ollama",
    "base_url": "http://localhost:11434",
    "model": "qwen2.5:7b"
  }
}
```

**Supported LLM Providers:** Ollama (local), OpenAI, DeepSeek

### 📋 Commands

| Command | Description |
|---------|-------------|
| `connect <host>` | Connect to host (natural language supported) |
| `$<cmd>` | Execute shell command directly |
| `upload/download` | File transfer with `--host=<id\|alias>` |
| `advisor` | AI intelligent advisor |
| `inspect` | Health inspection |
| `playbook` | Automated playbooks |
| `dashboard` | Visual dashboard |
| `batch <cmd>` | Execute on multiple hosts |
| `tunnel` | SSH tunnel management |
| `host add/list/del` | Host management |

### 📄 License

Apache License 2.0

---

<p align="center">
  <a href="https://github.com/warm3snow/sherlock/stargazers">⭐ Star us on GitHub</a> •
  <a href="https://github.com/warm3snow/sherlock/issues">🐛 Report Bug</a> •
  <a href="https://github.com/warm3snow/sherlock/pulls">🔧 Contribute</a>
</p>
