# telesend

[简体中文](README.zh-CN.md)

Send text, files, photos, videos, and voice messages to Telegram from the command line.

```
telesend text "Build finished"
telesend photo a.jpg b.jpg c.jpg --caption "Trip"
```

## Install

You need Go 1.27 or later.

```
go install github.com/nerdneilsfield/telesend@latest
```

## Quick start

1. Create a bot with [@BotFather](https://t.me/BotFather). Copy the bot token.
2. Open a chat with the bot. Send it any message.
3. Open `https://api.telegram.org/bot<token>/getUpdates` in a browser. Replace `<token>` with your bot token.
4. Find `message.chat.id` in the reply. This number is your chat ID.
5. Set the token and the chat ID.

   ```
   export TELESEND_BOT_TOKEN='<token>'
   export TELESEND_CHAT_ID='<chat-id>'
   ```

6. Send a message.

   ```
   telesend text "Hello from telesend"
   ```

The message appears in the chat. The command prints one line with the message ID.

## Common tasks

```
telesend text --file notes.md --markdown
git log -5 --oneline | telesend text
telesend photo ./shots/*.png
telesend media shot.png demo.mp4 --caption "Release 2"
telesend file report.pdf
telesend voice meeting.wav
```

In a terminal, telesend shows a progress bar with the speed and the time left. Add `--json` when a program reads the output.

Run `telesend --help` or `telesend <command> --help` for all commands and flags.

## Use with an AI agent

Install the agent skill. The `skills` tool installs it for Claude Code, Codex, Cursor, OpenCode, Gemini CLI, and other agents that load `SKILL.md` skills. You need Node.js.

```
npx skills add nerdneilsfield/telesend --skill telesend
```

The command finds the agents on your machine and lets you choose. Then ask the agent: "Send build.log to Telegram."

## Learn more

- [Usage](docs/usage.md): every command, Markdown, albums, voice
- [Configuration](docs/configuration.md): flags, environment variables, sending speed, logs
- [Use with an AI agent](docs/agent.md): the agent skill and the JSON output
- [Internals](docs/internals.md): for contributors

## License

[MIT](LICENSE)
