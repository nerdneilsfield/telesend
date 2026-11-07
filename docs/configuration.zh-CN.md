# 配置

[English](configuration.md) · [用法](usage.zh-CN.md)

命令行参数覆盖环境变量。环境变量覆盖默认值。

## 设置项

| 参数 | 环境变量 | 默认值 | 含义 |
| --- | --- | --- | --- |
| `--token` | `TELESEND_BOT_TOKEN` | 无（必填） | 来自 @BotFather 的机器人令牌。 |
| `--to`、`-t` | `TELESEND_CHAT_ID` | 无（必填） | 聊天 ID，或 `@channelname`。 |
| `--base-url` | `TELESEND_BASE_URL` | `https://api.telegram.org` | Bot API 地址。 |
| `--album-size` | `TELESEND_ALBUM_SIZE` | `10` | 每个相册的条目数，范围 2 到 10。 |
| `--text-limit` | `TELESEND_TEXT_LIMIT` | `4096` | 每条文本消息的字符数，范围 1 到 4096。 |
| `--silent` | 无 | 关 | 发送时不提示声音。 |
| `--thread` | 无 | `0`（无话题） | 论坛话题 ID，用于开启话题的超级群。 |
| `--rpm` | 无 | `20` | 每分钟最大请求数，范围 1 到 1800。 |
| `--concurrency` | 无 | `1` | 同时进行的请求数，范围 1 到 16。 |
| `--retries` | 无 | `3` | 临时失败后的重试次数。 |
| `--timeout` | 无 | `2m` | 单次尝试的时间限制。 |
| `--json` | 无 | 关 | 以 JSON 输出结果。 |
| `--log-level` | 无 | `warn` | `debug`、`info`、`warn` 或 `error`。 |
| `--verbose`、`-v` | 无 | 关 | 等同于 `--log-level debug`。 |

要长期保存设置，把 `export` 行加入 shell 配置文件：

```
export TELESEND_BOT_TOKEN='<token>'
export TELESEND_CHAT_ID='<chat-id>'
export TELESEND_ALBUM_SIZE=6
```

## 发送速度

Telegram 允许向单个聊天每秒约 1 条消息，向单个群组每分钟 20 条消息，所有聊天合计每秒约 30 条消息。

默认的每分钟 20 个请求低于群组限制。向私聊发送时，可把 `--rpm` 提高到 60。

在任意 60 秒内，telesend 发送的消息不会超过 `rpm` 条。包含 n 个条目的相册算作 n 条消息，因为 Telegram 把每个条目算作一条消息。

相册包含的条目数不能超过 `--rpm`。`--album-size` 更大时，telesend 会把相册缩小。`--rpm 1` 时，每个文件单独发送。

telesend 还会均匀地发出请求，间隔为 60 ÷ `rpm` 秒。相册立即发出，下一个请求按相册的条目数等待相应个间隔。使用默认值时，10 个条目的相册之后的下一个请求要等 30 秒。

`--concurrency` 大于 1 时，消息可能乱序到达。需要保持顺序时，使用默认值。

## 失败处理

收到 `429` 响应、`5xx` 响应或网络错误后，telesend 会重试。收到 `429` 后，所有请求等待 Telegram 要求的时间。其他错误依次等待 1 秒、2 秒和 4 秒。

第一个任务失败后，telesend 不再发送。它报告失败和未发送的任务数，退出码为 1。

超时后重试可能重复发送，因为 Telegram 可能已经收到第一次请求。

## 日志

日志输出到 stderr，结果输出到 stdout。

| 级别 | 显示内容 |
| --- | --- |
| `error` | 每个失败的任务，包含尝试次数和错误。 |
| `warn`（默认） | 重试和未发送的任务。 |
| `info` | 任务数量和每个已发送的任务。 |
| `debug` | 设置、每次尝试，以及每个 HTTP 状态码和耗时。 |

telesend 在每行日志和每条错误信息中，把机器人令牌替换为 `<token>`。
