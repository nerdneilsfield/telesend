---
name: telesend
description: Send text, Markdown, long messages, files, photos, videos, mixed albums, or voice messages to a Telegram chat with the telesend CLI. Use when the user asks to send, share, notify, or upload something to Telegram.
allowed-tools: Bash(telesend:*)
---

# telesend

Use `telesend` to send content from a Telegram bot to a chat.

## Before you send

1. Run `telesend version`. If the command is missing, tell the user to install it. Stop.
2. Check that `TELESEND_BOT_TOKEN` and `TELESEND_CHAT_ID` are set: `test -n "$TELESEND_BOT_TOKEN" && test -n "$TELESEND_CHAT_ID"`.
   If one is empty, ask the user to set it. Do not ask the user to type the token into the chat.
3. Send to the default chat. Pass `--to <id>` only when the user names another recipient.

Never print, log, or echo the bot token. For debugging, add `-v`. The log masks the token.

## Map the request to a command

| User asks to send | Command |
| --- | --- |
| A short message | `telesend text "<text>" --json` |
| Formatted text (bold, lists, links, code) | `telesend text --markdown "<markdown>" --json` |
| The content of a text or Markdown file, as a message | `telesend text --file <path> [--markdown] --json` |
| Command output as text | `<command> \| telesend text --json` |
| One photo | `telesend photo <path-or-url> --json` |
| Several photos as an album | `telesend photo <p1> <p2> ... --json` |
| One video, or several videos | `telesend video <path-or-url>... --json` |
| Photos and videos together | `telesend media <p1> <v1> ... --json` |
| A document, archive, log, or original-quality image, attached | `telesend file <path-or-url>... --json` |
| An audio recording as a voice message | `telesend voice <path-or-url> --json` |
| Any media with a caption | Add `--caption "<text>"`. Add `--markdown` for a Markdown caption. |

Rules:

1. Always add `--json`.
2. "Send this file as a message" means the file content is the message: use `text --file`. "Attach", "upload", or "send the file" means a document: use `file`.
3. Pass every file in one command. `telesend` splits long text at 4096 characters and packs files into albums of 10. Do not split them yourself.
4. Send an image with `file` when the user wants the original quality or the image is larger than 10 MB.
5. Change `--text-limit` (1 to 4096) or `--album-size` (2 to 10) only when the user asks.
6. Quote every text argument. Use single quotes when the text contains `$` or backticks.
7. Do not change `--rpm` or `--concurrency` unless the user asks.

## Read the result

`--json` prints one JSON object on stdout. These fields matter:

| Field | Meaning |
| --- | --- |
| `ok` | `true` when every message was sent. |
| `error` | The reason, when the run failed before it sent anything (`results` is empty). |
| `sent`, `failed`, `skipped` | Counts of jobs. A job is one message or one album. |
| `results[].name` | The job, for example `photo a.jpg` or `album 1-10 of 25`. |
| `results[].message_ids` | The IDs of the messages that Telegram created. |
| `results[].error` | The error. Present only when the job failed. |
| `stats.elapsed_ms`, `stats.avg_speed_bps` | Total time and average upload speed. |

1. Exit code 0 means `ok` is `true`. Report `sent` and the `message_ids`.
2. Exit code 1 means a failure. Report the top-level `error`, or the `error` of the failed job. Report `skipped`, the number of unsent jobs.
   If stdout is empty, read stderr. It holds the usage message of a wrong command line.
3. Do not retry on your own. `telesend` already retries `429`, `5xx`, and network errors.

## Errors

| Error text | Cause | Action |
| --- | --- | --- |
| `no bot token` / `no recipient` | Environment variable not set | Ask the user to set it. |
| `400 ... chat not found` | Wrong chat ID, or the bot never received a message from that chat | Ask the user to check the ID and to message the bot first. |
| `403 ... bot was blocked` | The user blocked the bot | Report it. |
| `400 ... can't parse entities` | Invalid Markdown input | Send again without `--markdown`. |
| `ffmpeg is not installed` | Local voice file needs conversion | Ask the user to install `ffmpeg`, or send an `.ogg`, `.mp3`, or `.m4a` file. |
| `cannot tell if ... photo or a video` | `media` got a file with an unknown extension | Use `telesend photo` or `telesend video`, or `telesend file`. |
| `converts local files only` | A voice URL has an unsupported format | Download the file, then pass the local path. |
| `caption has N characters` | Caption over 1024 characters | Shorten the caption, or send the text with `telesend text`. |

## Examples

User: "Tell me on Telegram when the build is done."

```
telesend text "Build finished" --json
```

User: "Send the three screenshots in ./shots to Telegram with the title Release 2."

```
telesend photo ./shots/1.png ./shots/2.png ./shots/3.png --caption "Release 2" --json
```

User: "Send report.pdf to chat -1001234567890."

```
telesend file report.pdf --to -1001234567890 --json
```
