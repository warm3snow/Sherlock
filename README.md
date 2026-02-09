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
│  │  │ (NLP)   │  │ (诊断)  │  │ (批量)  │  │ (监控)  │           │        │
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

### 📁 Project Structure

```
sherlock/
├── cmd/sherlock/      # CLI application
├── internal/
│   ├── advisor/       # 🆕 AI intelligent advisor
│   ├── agent/         # Natural language processing
│   ├── ai/            # LLM client (Ollama/OpenAI/DeepSeek)
│   ├── analyzer/      # Output analysis & diagnosis
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
