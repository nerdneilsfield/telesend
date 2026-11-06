---
name: telesend
description: Send text, Markdown, files, photos, photo albums, or voice messages to a Telegram chat with the telesend CLI. Use when the user asks to send, share, notify, or upload something to Telegram.
allowed-tools: Bash(telesend:*)
---

# telesend

Use `telesend` to send content from a Telegram bot to a chat.

## Before you send

1. Run `telesend version`. If the command is missing, tell the user to install it. Stop.
2. Check that `TELESEND_BOT_TOKEN` and `TELESEND_CHAT_ID` are set: `test -n "$TELESEND_BOT_TOKEN" && test -n "$TELESEND_CHAT_ID"`.
   If one is empty, ask the user to set it. Do not ask the user to type the token into the chat.
3. Send to the default chat. Pass `--to <id>` only when the user names another recipient.

Never print, log, or echo the bot token.

## Map the request to a command

| User asks to send | Command |
| --- | --- |
| A short message | `telesend text "<text>" --json` |
| Formatted text (bold, lists, links, code) | `telesend text --markdown "<markdown>" --json` |
| The content of a file or command output as text | `<command> \| telesend text --json` |
| One photo | `telesend photo <path-or-url> --json` |
| Several photos as an album | `telesend photo <p1> <p2> ... --json` |
| A document, archive, log, or original-quality image | `telesend file <path-or-url>... --json` |
| An audio recording as a voice message | `telesend voice <path-or-url> --json` |
| Any media with a caption | Add `--caption "<text>"`. Add `--markdown` for a Markdown caption. |

Rules:

1. Always add `--json`.
2. Send an image with `file` when the user wants the original quality or the image is larger than 10 MB.
3. Quote every text argument. Use single quotes when the text contains `$` or backticks.
4. Do not change `--rpm` or `--concurrency` unless the user asks.

## Read the result

`--json` prints a list. Each item has `name`, `ok`, `message_ids`, `attempts`, `error`, and `skipped`.

1. Exit code 0 means every item has `ok: true`. Report the count and the `message_ids`.
2. Exit code 1 means a failure. Report the `error` of the failed item. Report how many items were skipped.
3. Do not retry on your own. `telesend` already retries `429`, `5xx`, and network errors.

## Errors

| Error text | Cause | Action |
| --- | --- | --- |
| `no bot token` / `no recipient` | Environment variable not set | Ask the user to set it. |
| `400 ... chat not found` | Wrong chat ID, or the bot never received a message from that chat | Ask the user to check the ID and to message the bot first. |
| `403 ... bot was blocked` | The user blocked the bot | Report it. |
| `400 ... can't parse entities` | Invalid Markdown input | Send again without `--markdown`. |
| `ffmpeg is not installed` | Voice file needs conversion | Ask the user to install `ffmpeg`, or send an `.ogg`, `.mp3`, or `.m4a` file. |
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
