# Configuration

[简体中文](configuration.zh-CN.md) · [Usage](usage.md)

A flag overrides the environment variable. The environment variable overrides the default.

## Settings

| Flag | Environment variable | Default | Meaning |
| --- | --- | --- | --- |
| `--token` | `TELESEND_BOT_TOKEN` | none (required) | Bot token from @BotFather. |
| `--to`, `-t` | `TELESEND_CHAT_ID` | none (required) | Chat ID, or `@channelname`. |
| `--base-url` | `TELESEND_BASE_URL` | `https://api.telegram.org` | Bot API address. |
| `--album-size` | `TELESEND_ALBUM_SIZE` | `10` | Items per album, 2 to 10. |
| `--text-limit` | `TELESEND_TEXT_LIMIT` | `4096` | Characters per text message, 1 to 4096. |
| `--silent` | none | off | Send without a notification sound. |
| `--thread` | none | `0` (no topic) | Forum topic ID, for a supergroup with topics. |
| `--rpm` | none | `20` | Maximum requests per minute, 1 to 1800. |
| `--concurrency` | none | `1` | Requests in flight, 1 to 16. |
| `--retries` | none | `3` | Retries after a temporary failure. |
| `--timeout` | none | `2m` | Time limit for one attempt. |
| `--json` | none | off | Print the result as JSON. |
| `--log-level` | none | `warn` | `debug`, `info`, `warn`, or `error`. |
| `--verbose`, `-v` | none | off | Same as `--log-level debug`. |

To keep your settings, add the `export` lines to your shell profile:

```
export TELESEND_BOT_TOKEN='<token>'
export TELESEND_CHAT_ID='<chat-id>'
export TELESEND_ALBUM_SIZE=6
```

## Sending speed

Telegram allows about 1 message per second to one chat, 20 messages per minute to one group, and about 30 messages per second across all chats.

The default of 20 requests per minute stays under the group limit. For a private chat, raise `--rpm` to 60.

telesend never sends more than `rpm` messages in any 60 seconds. An album of n items counts as n messages, because Telegram counts each item as a message.

An album cannot hold more items than `--rpm`. When `--album-size` is larger, telesend makes the albums smaller. With `--rpm 1`, it sends each file alone.

telesend also spaces requests evenly: 60 ÷ `rpm` seconds apart. An album goes out at once. The next request waits for one spacing for each item of the album. At the default, a request after an album of 10 items waits 30 seconds.

When `--concurrency` is above 1, messages can arrive out of order. Keep the default when order matters.

## Failures

telesend retries a request after a `429` reply, a `5xx` reply, or a network error. After a `429` reply, all requests wait for the time that Telegram asks. Other errors wait 1, 2, and 4 seconds.

After the first failed job, telesend sends nothing more. It reports the failure and the number of unsent jobs. The exit code is 1.

A retry after a timeout can send a duplicate, when Telegram received the first request.

## Logs

Logs go to stderr. Results go to stdout.

| Level | What it shows |
| --- | --- |
| `error` | Each failed job, with its attempt count and error. |
| `warn` (default) | Retries and unsent jobs. |
| `info` | The job count, and each job that was sent. |
| `debug` | The settings, every attempt, and every HTTP status with its duration. |

telesend replaces the bot token with `<token>` in every log line and error message.
