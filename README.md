# Sherlock

[English](#english) | [中文](#中文)

<a name="english"></a>
## Sherlock - AI-powered SSH Remote Operations Tool

Sherlock is an AI-based remote operations tool built on SSH. It enables you to interact with remote hosts using natural language commands.

### Features

1. **Natural Language Connection** - Connect to remote hosts by describing what you want in plain language
2. **Automatic SSH Key Management** - After password-based connection, automatically adds your local SSH public key to the remote host's authorized_keys for future passwordless authentication
3. **AI-powered Command Execution** - Describe what you want to do in natural language, and Sherlock will translate it to shell commands
4. **Multiple LLM Provider Support** - Works with local Ollama, DeepSeek, or OpenAI APIs using the CloudWeGo Eino framework

### Installation

#### From Source

```bash
# Clone the repository
git clone https://github.com/warm3snow/Sherlock.git
cd Sherlock

# Build
go build -o sherlock ./cmd/sherlock

# Optional: Install to $GOPATH/bin
go install ./cmd/sherlock
```

### Configuration

Sherlock uses a JSON configuration file. The default location is `~/.config/sherlock/config.json`.

```json
{
  "llm": {
    "provider": "ollama",
    "base_url": "http://localhost:11434",
    "model": "qwen2.5:7b",
    "temperature": 0.7
  },
  "ssh_key": {
    "private_key_path": "~/.ssh/id_rsa",
    "public_key_path": "~/.ssh/id_rsa.pub",
    "auto_add_to_remote": true
  }
}
```

#### LLM Providers

**Ollama (Local)**
```json
{
  "llm": {
    "provider": "ollama",
    "base_url": "http://localhost:11434",
    "model": "qwen2.5:7b"
  }
}
```

**OpenAI**
```json
{
  "llm": {
    "provider": "openai",
    "api_key": "your-api-key",
    "model": "gpt-4"
  }
}
```

**DeepSeek**
```json
{
  "llm": {
    "provider": "deepseek",
    "api_key": "your-api-key",
    "model": "deepseek-chat"
  }
}
```

### Usage

#### Start Interactive Mode

```bash
sherlock
```

#### Command Line Options

```bash
sherlock [options]

Options:
  -c, --config <path>     Path to configuration file
  -v, --version           Show version information
  -h, --help              Show help message
  --provider <provider>   LLM provider (ollama, openai, deepseek)
  --model <model>         Model name
  --base-url <url>        Base URL for LLM API
  --api-key <key>         API key for LLM provider
```

#### Interactive Commands

```
# Built-in commands
help                    Show help message
exit, quit, q           Exit Sherlock
status                  Show current status
disconnect              Disconnect from current host

# Connection (natural language)
connect to 192.168.1.100 as root
ssh user@example.com:2222
login to server 10.0.0.1 port 2222 as admin

# Execute commands (when connected)
$ls -la                 Execute command directly
show me disk usage      Natural language command
list running processes  Natural language command
```

### Examples

```
$ sherlock
sherlock> connect to 192.168.1.100 as root
Parsing connection request...
Connecting to root@192.168.1.100:22...
Password (leave empty to use SSH key): ****
Successfully connected to root@192.168.1.100:22
Adding public key to remote authorized_keys...
Public key added successfully. Future connections can use key authentication.

sherlock[root@192.168.1.100:22]> show me disk usage
Commands to execute:
  1. df -h
Description: Display disk space usage in human-readable format

$ df -h
Filesystem      Size  Used Avail Use% Mounted on
/dev/sda1        50G   20G   28G  42% /

sherlock[root@192.168.1.100:22]> $uptime
 14:30:01 up 45 days,  3:22,  2 users,  load average: 0.15, 0.10, 0.08

sherlock[root@192.168.1.100:22]> exit
Goodbye!
```

### Project Structure

```
Sherlock/
├── cmd/
│   └── sherlock/          # Main CLI application
├── internal/
│   ├── agent/             # AI agent for natural language processing
│   ├── ai/                # LLM client implementations (Ollama, OpenAI, DeepSeek)
│   └── config/            # Configuration management
├── pkg/
│   └── sshclient/         # SSH client implementation
├── go.mod
├── go.sum
└── README.md
```

### Requirements

- Go 1.18 or higher
- An LLM provider:
  - Local: [Ollama](https://ollama.ai/) with a compatible model
  - Cloud: OpenAI API key or DeepSeek API key

### License

Apache License 2.0

---

<a name="中文"></a>
## Sherlock - 基于AI的远程运维工具

Sherlock 是一个基于 AI 的远程运维工具，底层基于 SSH。它可以让您通过自然语言与远程主机进行交互。

### 主要功能

1. **自然语言连接** - 通过自然语言描述来连接远程主机
2. **自动 SSH 密钥管理** - 通过密码连接后，自动将本地 SSH 公钥添加到远程主机的 authorized_keys，实现后续免密登录
3. **AI 驱动的命令执行** - 用自然语言描述想要执行的操作，Sherlock 会将其转换为 shell 命令
4. **多种 LLM 支持** - 支持本地 Ollama、DeepSeek 或 OpenAI API，使用字节跳动 CloudWeGo Eino 框架

### 安装

#### 从源码构建

```bash
# 克隆仓库
git clone https://github.com/warm3snow/Sherlock.git
cd Sherlock

# 构建
go build -o sherlock ./cmd/sherlock

# 可选：安装到 $GOPATH/bin
go install ./cmd/sherlock
```

### 配置

Sherlock 使用 JSON 配置文件，默认位置为 `~/.config/sherlock/config.json`。

```json
{
  "llm": {
    "provider": "ollama",
    "base_url": "http://localhost:11434",
    "model": "qwen2.5:7b",
    "temperature": 0.7
  },
  "ssh_key": {
    "private_key_path": "~/.ssh/id_rsa",
    "public_key_path": "~/.ssh/id_rsa.pub",
    "auto_add_to_remote": true
  }
}
```

#### LLM 提供商配置

**Ollama (本地)**
```json
{
  "llm": {
    "provider": "ollama",
    "base_url": "http://localhost:11434",
    "model": "qwen2.5:7b"
  }
}
```

**OpenAI**
```json
{
  "llm": {
    "provider": "openai",
    "api_key": "your-api-key",
    "model": "gpt-4"
  }
}
```

**DeepSeek**
```json
{
  "llm": {
    "provider": "deepseek",
    "api_key": "your-api-key",
    "model": "deepseek-chat"
  }
}
```

### 使用方法

#### 启动交互模式

```bash
sherlock
```

#### 命令行选项

```bash
sherlock [选项]

选项:
  -c, --config <路径>     配置文件路径
  -v, --version           显示版本信息
  -h, --help              显示帮助信息
  --provider <提供商>     LLM 提供商 (ollama, openai, deepseek)
  --model <模型>          模型名称
  --base-url <URL>        LLM API 基础 URL
  --api-key <密钥>        LLM 提供商的 API 密钥
```

#### 交互式命令

```
# 内置命令
help                    显示帮助信息
exit, quit, q           退出 Sherlock
status                  显示当前状态
disconnect              断开当前连接

# 连接 (自然语言)
连接到 192.168.1.100 用户名 root
ssh user@example.com:2222
以 admin 身份登录服务器 10.0.0.1 端口 2222

# 执行命令 (连接后)
$ls -la                 直接执行命令
查看磁盘使用情况        自然语言命令
列出运行中的进程        自然语言命令
```

### 使用示例

```
$ sherlock
sherlock> 连接到 192.168.1.100 用户名 root
正在解析连接请求...
正在连接 root@192.168.1.100:22...
密码 (留空使用 SSH 密钥): ****
成功连接到 root@192.168.1.100:22
正在添加公钥到远程 authorized_keys...
公钥添加成功，后续可使用密钥认证登录。

sherlock[root@192.168.1.100:22]> 查看磁盘使用情况
将要执行的命令:
  1. df -h
描述: 以人类可读格式显示磁盘空间使用情况

$ df -h
Filesystem      Size  Used Avail Use% Mounted on
/dev/sda1        50G   20G   28G  42% /

sherlock[root@192.168.1.100:22]> $uptime
 14:30:01 up 45 days,  3:22,  2 users,  load average: 0.15, 0.10, 0.08

sherlock[root@192.168.1.100:22]> exit
再见！
```

### 项目结构

```
Sherlock/
├── cmd/
│   └── sherlock/          # 主 CLI 应用
├── internal/
│   ├── agent/             # 自然语言处理 AI 代理
│   ├── ai/                # LLM 客户端实现 (Ollama, OpenAI, DeepSeek)
│   └── config/            # 配置管理
├── pkg/
│   └── sshclient/         # SSH 客户端实现
├── go.mod
├── go.sum
└── README.md
```

### 环境要求

- Go 1.18 或更高版本
- LLM 提供商之一:
  - 本地: [Ollama](https://ollama.ai/) 及兼容模型
  - 云端: OpenAI API 密钥或 DeepSeek API 密钥

### 开源协议

Apache License 2.0