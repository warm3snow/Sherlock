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
```

### ⚙️ 配置

创建配置文件 `~/.config/sherlock/config.json`：

```json
{
  "llm": {
    "provider": "ollama",
    "base_url": "http://localhost:11434",
    "model": "qwen2.5:7b"
  }
}
```

**支持的 LLM 提供商：** Ollama（本地）、OpenAI、DeepSeek

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

### 📁 项目结构

```
sherlock/
├── cmd/sherlock/      # CLI 应用入口
├── internal/
│   ├── advisor/       # 🆕 AI 智能运维助手
│   ├── agent/         # 自然语言处理
│   ├── ai/            # LLM 客户端 (Ollama/OpenAI/DeepSeek)
│   ├── analyzer/      # 输出分析与诊断
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
