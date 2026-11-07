# 与 AI Agent 配合使用

[English](agent.md) · [用法](usage.zh-CN.md)

`skills/telesend/SKILL.md` 是 Agent skill。它把自然语言请求转换为 `telesend` 命令。任何加载 `SKILL.md` 的 Agent 都可以使用它。

## 安装 skill

需要 Node.js 才能使用 `npx`。

1. 安装 `telesend`。
2. 在 Agent 的环境中设置 `TELESEND_BOT_TOKEN` 和 `TELESEND_CHAT_ID`。
3. 把 skill 安装到当前项目。

   ```
   npx skills add nerdneilsfield/telesend --skill telesend
   ```

   命令会检测你机器上的 Agent，并让你选择。要跳过选择，用 `-a` 指定 Agent，例如 `-a claude-code`、`-a codex`、`-a cursor`、`-a opencode` 或 `-a gemini-cli`。

   要为你的所有项目安装 skill，加上 `-g`。

4. 对 Agent 说：“把 build.log 发到 Telegram。”

安装前想先看看有哪些 skill，运行 `npx skills add nerdneilsfield/telesend --list`。

可选：不用 `npx` 安装。把目录复制到你的 Agent 的 skills 目录。Claude Code 的 skills 目录是 `~/.claude/skills/`。

```
cp -r skills/telesend ~/.claude/skills/
```

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
| `error` | 运行在发送任何内容之前失败时的原因。 |
| `results[].name` | 任务，例如 `photo a.jpg` 或 `album 1-10 of 25`。 |
| `results[].message_ids` | Telegram 创建的消息 ID。 |
| `results[].attempts` | 请求次数，包含重试。 |
| `results[].took_ms` | 最后一次请求的耗时。 |
| `results[].bytes` | 该任务的文件字节数。 |
| `results[].error` | 错误。仅在任务失败时出现。 |
| `results[].skipped` | 前面的失败使该任务未能开始时为 `true`。 |
| `stats.avg_speed_bps` | 每秒请求时间内上传的字节数。 |

`ok` 为 `true` 时退出码为 0，否则为 1。`error: ...` 信息输出到 stderr。

在发送任何内容之前发现错误时，例如文件不存在或设置错误，telesend 仍然输出一个 JSON 对象。`ok` 为 `false`，`results` 为空，顶层 `error` 字段包含错误信息。

命令行错误，例如未知参数或缺少参数，只在 stderr 输出用法信息。如果 stdout 为空且退出码为 1，请读取 stderr。
