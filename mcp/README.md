# Sherlock MCP Server

[Model Context Protocol (MCP)](https://modelcontextprotocol.io/) 服务器，允许将 Sherlock 作为工具集成到 Claude Desktop 等支持 MCP 的 AI 应用中。

## 功能

通过 MCP 暴露以下 Sherlock 功能：

| 工具 | 描述 |
|------|------|
| `sherlock_execute` | 在远程主机执行 Shell 命令 |
| `sherlock_analyze` | AI 分析命令输出/日志 |
| `sherlock_diagnose` | AI 诊断错误和问题 |
| `sherlock_batch_execute` | 批量执行命令 |
| `sherlock_health_check` | 主机健康检查 |
| `sherlock_upload` | 上传文件到远程主机 |
| `sherlock_download` | 从远程主机下载文件 |
| `sherlock_hosts_list` | 列出配置的主机 |
| `sherlock_hosts_add` | 添加新主机 |
| `sherlock_tunnel_create` | 创建 SSH 隧道 |
| `sherlock_tunnel_list` | 列出活动隧道 |
| `sherlock_tunnel_close` | 关闭隧道 |
| `sherlock_playbook_list` | 列出运维剧本 |
| `sherlock_playbook_run` | 执行运维剧本 |
| `sherlock_advisor` | AI 运维顾问 |
| `sherlock_snippet_list` | 列出命令片段 |
| `sherlock_snippet_run` | 执行命令片段 |
| `sherlock_database_list` | 列出数据库连接 |
| `sherlock_database_query` | 执行 SQL 查询 |
| `sherlock_cache_list` | 列出缓存连接 |
| `sherlock_cache_command` | 执行 Redis 命令 |

## 编译

```bash
# 在项目根目录执行
make build-mcp

# 或手动编译
cd mcp && go build -o sherlock-mcp .
```

## 配置 Claude Desktop

编辑 Claude Desktop 配置文件：

**macOS**: `~/Library/Application Support/Claude/claude_desktop_config.json`

**Windows**: `%APPDATA%\Claude\claude_desktop_config.json`

添加 Sherlock MCP 服务器配置：

```json
{
  "mcpServers": {
    "sherlock": {
      "command": "/path/to/sherlock-mcp",
      "args": [],
      "env": {
        "SHERLOCK_CONFIG": "/path/to/.sherlock/config.yaml"
      }
    }
  }
}
```

## 环境变量

| 变量 | 描述 | 默认值 |
|------|------|--------|
| `SHERLOCK_CONFIG` | Sherlock 配置文件路径 | `~/.sherlock/config.yaml` |
| `SHERLOCK_PATH` | Sherlock 二进制文件路径 | 自动查找 |

## 使用示例

配置完成后，在 Claude 中可以直接使用 Sherlock 功能：

```
User: 检查我的服务器 web-server-1 的磁盘使用情况

Claude: [调用 sherlock_execute]
服务器 web-server-1 的磁盘使用情况如下...

User: 分析一下这个错误日志

Claude: [调用 sherlock_analyze]
根据日志分析，问题原因是...建议...
```

## 架构

```
┌─────────────────┐     ┌──────────────┐     ┌───────────────┐
│  Claude Desktop │────▶│ sherlock-mcp │────▶│   sherlock    │
│    (MCP Client) │◀────│ (MCP Server) │◀────│   (CLI/Core)  │
└─────────────────┘     └──────────────┘     └───────────────┘
         │                     │                     │
         │ JSON-RPC 2.0        │ subprocess          │ SSH/SFTP
         │ over stdio          │ invocation          │
         ▼                     ▼                     ▼
    ┌─────────┐          ┌─────────┐          ┌─────────┐
    │  User   │          │  Tools  │          │ Remote  │
    │ Request │          │Execution│          │ Servers │
    └─────────┘          └─────────┘          └─────────┘
```

## 协议版本

- MCP 版本: 2024-11-05
- 传输方式: stdio (JSON-RPC 2.0)

## 开发

### 添加新工具

1. 在 `getAvailableTools()` 中添加工具定义
2. 在 `executeTool()` 的 switch 中添加处理分支
3. 实现对应的工具函数

### 调试

```bash
# 手动测试 MCP 服务器
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | ./sherlock-mcp
echo '{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}' | ./sherlock-mcp
```

## License

MIT
