# 与 AI Agent 配合使用

[English](agent.md) · [用法](usage.zh-CN.md)

`skills/telesend/SKILL.md` 是 Claude Code skill。它把自然语言请求转换为 `telesend` 命令。

## 安装 skill

1. 安装 `telesend`。
2. 在 Agent 的环境中设置 `TELESEND_BOT_TOKEN` 和 `TELESEND_CHAT_ID`。
3. 把 skill 复制到 skills 目录。

   ```
   cp -r skills/telesend ~/.claude/skills/
   ```

4. 对 Agent 说：“把 build.log 发到 Telegram。”

Agent 运行 `telesend file build.log --json`，并报告消息 ID。

## JSON 输出

加上 `--json`，stdout 得到一个 JSON 对象。此模式下，telesend 不显示进度条，也不输出汇总行。日志仍在 stderr。

```json
{
  "ok": true,
  "sent": 1,
  "failed": 0,
  "skipped": 0,
  "results": [
    {
      "name": "photo a.jpg",
      "ok": true,
      "message_ids": [41],
      "attempts": 1,
      "took_ms": 812,
      "bytes": 2097152
    }
  ],
  "stats": {
    "requests": 1,
    "bytes": 2097152,
    "elapsed_ms": 812,
    "avg_request_ms": 812,
    "avg_speed_bps": 2582000
  }
}
```

| 字段 | 含义 |
| --- | --- |
| `ok` | 所有任务都已发送时为 `true`。 |
| `results[].name` | 任务，例如 `photo a.jpg` 或 `album 1-10 of 25`。 |
| `results[].message_ids` | Telegram 创建的消息 ID。 |
| `results[].attempts` | 请求次数，包含重试。 |
| `results[].took_ms` | 最后一次请求的耗时。 |
| `results[].bytes` | 该任务的文件字节数。 |
| `results[].error` | 错误。仅在任务失败时出现。 |
| `results[].skipped` | 前面的失败使该任务未能开始时为 `true`。 |
| `stats.avg_speed_bps` | 每秒请求时间内上传的字节数。 |

`ok` 为 `true` 时退出码为 0，否则为 1。`error: ...` 信息输出到 stderr。
