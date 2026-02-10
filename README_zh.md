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
| **🧠 AI 深度增强** | 多轮对话记忆、主动分析预警、命令意图预测 |
| **📊 可视化仪表盘** | 终端 ASCII 图表、多主机状态矩阵、健康评分 |
| **⚡ 批量操作** | 多主机并行执行命令，支持进度跟踪 |
| **🔐 安全管理** | 自动 SSH 密钥管理、凭据加密、操作审计 |
| **📁 文件传输** | 上传/下载支持进度显示，支持 `--host=<id|别名>` 指定目标 |

### 🏗️ 系统架构

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

# 文件传输（支持 --host=<id|别名>）
sherlock> upload local.txt /root/ --host=myserver
sherlock> download /var/log/app.log ./logs/ --host=1

# AI 运维功能
sherlock> advisor                          # AI 智能运维助手
sherlock> inspect                          # 一键健康巡检
sherlock> playbook run daily-inspect       # 执行运维剧本
sherlock> playbook generate 部署 nginx     # AI 生成运维剧本
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

### 📋 命令速查

| 命令 | 说明 |
|------|------|
| `connect <主机>` | 连接主机（支持自然语言） |
| `$<命令>` | 直接执行 shell 命令 |
| `upload/download` | 文件传输，支持 `--host=<id\|别名>` |
| `advisor` | AI 智能运维助手 |
| `inspect` | 健康巡检 |
| `playbook` | 自动化运维剧本 |
| `dashboard` | 可视化仪表盘 |
| `batch <命令>` | 多主机批量执行 |
| `tunnel` | SSH 隧道管理 |
| `host add/list/del` | 主机管理 |

### 📄 开源协议

Apache License 2.0

---

<p align="center">
  <a href="https://github.com/warm3snow/sherlock/stargazers">⭐ 给项目点个 Star</a> •
  <a href="https://github.com/warm3snow/sherlock/issues">🐛 反馈问题</a> •
  <a href="https://github.com/warm3snow/sherlock/pulls">🔧 参与贡献</a>
</p>
