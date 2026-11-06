# telesend

[简体中文](README.zh-CN.md)

A command-line tool that sends text, Markdown, files, photos, albums, and voice messages from a Telegram bot to a chat.

- Sends through a worker pool. A shared limiter spaces requests at a fixed rate per minute.
- Pauses all workers when Telegram answers `429`, for the time that Telegram asks.
- Retries `5xx` replies and network errors with exponential backoff.
- Converts CommonMark to Telegram MarkdownV2 and escapes every reserved character.
- Sends photos, videos, and documents. Packs any number of files into albums of up to 10, and mixes photos with videos.
- Splits a long text at line ends into messages of at most 4096 characters, and sends them in order.
- Converts audio to OGG/Opus with `ffmpeg`.
- Logs at four levels on stderr. The bot token never appears in logs or errors.
- Works with a custom API address, for example a self-hosted `telegram-bot-api` server.

```
telesend text --markdown "**Deploy** finished: [log](https://example.com/log)"
```

## Install

Requirements: Go 1.27 or later. `ffmpeg` is optional. `telesend voice` needs it for files that are not `.ogg`, `.mp3`, or `.m4a`.

```
go install github.com/dengqi/telesend@latest
```

To build from a clone, run `make build`. The binary is `./telesend`.

## Quick start

1. Create a bot with [@BotFather](https://t.me/BotFather). Copy the bot token.
2. Open a chat with the bot and send it any message. A bot cannot start a chat.
3. Find your chat ID. Open `https://api.telegram.org/bot<token>/getUpdates` and read `message.chat.id`.
   Replace `<token>` with your bot token.
4. Set the token and the chat ID.

   ```
   export TELESEND_BOT_TOKEN='<token>'
   export TELESEND_CHAT_ID='<chat-id>'
   ```

5. Send a message.

   ```
   telesend text "Hello from telesend"
   ```

The command prints `sent text (message <id>)`. The message appears in the chat.

## Usage

Each command sends one kind of content.

| Command | What it sends |
| --- | --- |
| `telesend text [TEXT...]` | A text message. Reads `--file` or stdin when you give no text. |
| `telesend photo SRC...` | One photo, or albums of up to 10 photos. |
| `telesend video SRC...` | One video, or albums of up to 10 videos. |
| `telesend media SRC...` | Photos and videos in the same albums. |
| `telesend file SRC...` | Files as documents, without recompression. Several files go out as document albums. |
| `telesend voice SRC` | An audio file as a playable voice message. |

`SRC` is a local path or an `http(s)` URL.

`telesend text --file notes.md` reads the file and sends its content as message text. It does not attach the file. To attach a file, use `telesend file`.

### Albums

`photo`, `video`, `media`, and `file` pack many files into albums of up to 10 items, in the order that you give.
For example, 25 photos make three albums: 10, 10, and 5 photos. A single leftover item goes out as a normal message.
The caption appears on the first item of the first album.

`media` tells photos from videos by the file extension.

| Kind | Extensions |
| --- | --- |
| Photo | `.jpg` `.jpeg` `.png` `.webp` `.bmp` |
| Video | `.mp4` `.mov` `.mkv` `.webm` `.avi` `.m4v` |

Telegram accepts documents in an album only with other documents. `file` therefore never mixes with photos or videos.

```
telesend text --markdown "# Report\n\n- **ok**: 12\n- failed: 0"
telesend text --file notes.md --markdown
git log -5 --oneline | telesend text
telesend photo a.jpg b.jpg c.jpg --caption "Trip"
telesend video demo.mp4
telesend media shot1.png shot2.png demo.mp4 --caption "Release 2"
telesend photo ./shots/*.png
telesend file report.pdf --caption "Q3 report"
telesend voice meeting.wav
```

Add `--json` to print one JSON object per job. The object has `name`, `ok`, `message_ids`, `attempts`, and `error`.

Run `telesend <command> --help` for the flags and examples of one command.

### Markdown

`--markdown` (`-m`) parses the input as CommonMark and writes MarkdownV2.

| Input | Telegram shows |
| --- | --- |
| `# Title`, `## Title` | Bold line |
| `**bold**`, `*italic*`, `~~strike~~` | Bold, italic, strikethrough |
| `` `code` `` and fenced code blocks | Monospace |
| `- item`, `1. item` | Bullet list, numbered list |
| `> quote` | Quote |
| `[text](url)` | Link |

Tables, task lists, and images have no MarkdownV2 form. Images become links. A table becomes plain text.

### Limits that telesend enforces

| Limit | Value | Behavior |
| --- | --- | --- |
| Text length | 4096 characters | Splits at line ends, or between Markdown blocks. |
| Caption length | 1024 characters | Stops with an error. |
| Album size | 10 items | Starts a new album. |

A line longer than 4096 characters in Markdown mode stops with an error. Break the line up.

## Configuration

A flag overrides the environment variable. The environment variable overrides the default.

| Flag | Environment variable | Default | Meaning |
| --- | --- | --- | --- |
| `--token` | `TELESEND_BOT_TOKEN` | none (required) | Bot token. |
| `--to`, `-t` | `TELESEND_CHAT_ID` | none (required) | Chat ID, or `@channelname`. |
| `--base-url` | `TELESEND_BASE_URL` | `https://api.telegram.org` | Bot API address. |
| `--rpm` | none | `20` | Maximum requests per minute, 1 to 1800. |
| `--concurrency` | none | `1` | Requests in flight, 1 to 16. |
| `--retries` | none | `3` | Retries after `429`, `5xx`, or a network error. |
| `--timeout` | none | `2m` | Time limit for one attempt. |
| `--log-level` | none | `warn` | Log level on stderr: `debug`, `info`, `warn`, or `error`. |
| `--verbose`, `-v` | none | off | Same as `--log-level debug`. |
| `--json` | none | off | Print results as JSON on stdout. |

### Rate limits

Telegram allows about 1 message per second to one chat, 20 messages per minute to one group, and about 30 messages per second across all chats.
The default of 20 requests per minute stays under the group limit.
For a private chat, raise `--rpm` to 60.

`--rpm` sets the spacing between requests: 60 ÷ `rpm` seconds. The limiter sends the first request at once.
An album of n items uses n of the `--rpm` budget, because Telegram counts each item as a message.
At the default of 20, an album of 10 items takes about 27 seconds to start.

With `--concurrency` above 1, messages can arrive out of order. Keep the default of 1 when order matters.

After the first failed job, telesend starts no more jobs. It reports the failure and the number of unsent jobs, and it exits with code 1.

A retry after a timeout can send a duplicate when Telegram received the first request.

### Logs

Logs go to stderr. Results go to stdout.

| Level | What it shows |
| --- | --- |
| `error` | Each failed job, with its attempt count and error. |
| `warn` (default) | Retries and unsent jobs. |
| `info` | The job count, and each job that was sent. |
| `debug` | The resolved settings, every attempt, and every HTTP status with its duration. |

`telesend` replaces the bot token with `<token>` in every log line and error message.

### Voice formats

Telegram plays `.ogg` (Opus), `.mp3`, and `.m4a` files as voice messages. telesend uploads these files as they are.
It converts every other format with `ffmpeg -c:a libopus -b:a 32k -ar 48000 -ac 1`.
An `.ogg` file that holds Vorbis audio arrives as a document, not as a voice message.

## Use with an AI agent

`skills/telesend/SKILL.md` is a Claude Code skill. It maps natural-language requests to `telesend` commands.

1. Install `telesend` and set the environment variables.
2. Copy the skill to your skills directory.

   ```
   cp -r skills/telesend ~/.claude/skills/
   ```

3. Ask the agent, for example: "Send build.log to Telegram."

## Development

```
make check
```

`make check` runs the format check, `golangci-lint`, `go vet`, the tests, and `goreleaser check`.
The tests use a mock Telegram server. They need no token and no network.

## License

[MIT](LICENSE)
