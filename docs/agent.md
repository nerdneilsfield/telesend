# Use with an AI agent

[简体中文](agent.zh-CN.md) · [Usage](usage.md)

`skills/telesend/SKILL.md` is a Claude Code skill. It turns natural-language requests into `telesend` commands.

## Install the skill

1. Install `telesend`.
2. Set `TELESEND_BOT_TOKEN` and `TELESEND_CHAT_ID` in the environment of the agent.
3. Copy the skill to your skills directory.

   ```
   cp -r skills/telesend ~/.claude/skills/
   ```

4. Ask the agent: "Send build.log to Telegram."

The agent runs `telesend file build.log --json` and reports the message IDs.

## JSON output

Add `--json` to get one JSON object on stdout. In this mode, telesend prints no progress bar and no summary line. Logs stay on stderr.

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

| Field | Meaning |
| --- | --- |
| `ok` | `true` when every job was sent. |
| `results[].name` | The job, for example `photo a.jpg` or `album 1-10 of 25`. |
| `results[].message_ids` | The IDs of the messages that Telegram created. |
| `results[].attempts` | The number of requests, including retries. |
| `results[].took_ms` | The time of the last request. |
| `results[].bytes` | The file bytes of the job. |
| `results[].error` | The error. Present only when the job failed. |
| `results[].skipped` | `true` when an earlier failure stopped the job before it started. |
| `stats.avg_speed_bps` | Uploaded bytes per second of request time. |

The exit code is 0 when `ok` is `true`. The exit code is 1 otherwise. The message `error: ...` goes to stderr.
