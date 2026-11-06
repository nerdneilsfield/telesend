# telesend

[English](README.md)

命令行工具，用 Telegram 机器人向聊天发送文本、Markdown、文件、图片、相册和语音。

- 通过工作池发送。共享限流器按每分钟固定数量均匀发出请求。
- Telegram 返回 `429` 时，所有工作协程按 Telegram 要求的时间暂停。
- 对 `5xx` 响应和网络错误做指数退避重试。
- 把 CommonMark 转换为 Telegram MarkdownV2，并转义全部保留字符。
- 发送图片、视频和文档。把任意数量的文件打包成每组最多 10 个的相册，图片和视频可以混合。
- 长文本在行尾拆分为不超过 4096 个字符的消息，并按顺序发送。
- 用 `ffmpeg` 把音频转换为 OGG/Opus。
- 日志分四个级别，输出到 stderr。机器人令牌不会出现在日志和错误信息中。
- 支持自定义 API 地址，例如自建的 `telegram-bot-api` 服务。

```
telesend text --markdown "**Deploy** finished: [log](https://example.com/log)"
```

## 安装

要求：Go 1.27 或更高版本。`ffmpeg` 可选。发送非 `.ogg`、`.mp3`、`.m4a` 格式的语音时需要它。

```
go install github.com/dengqi/telesend@latest
```

从克隆的源码构建，运行 `make build`。二进制文件是 `./telesend`。

## 快速开始

1. 用 [@BotFather](https://t.me/BotFather) 创建机器人，复制机器人令牌。
2. 打开与机器人的聊天，给它发任意一条消息。机器人不能主动发起聊天。
3. 查询聊天 ID。打开 `https://api.telegram.org/bot<token>/getUpdates`，读取 `message.chat.id`。
   把 `<token>` 换成你的机器人令牌。
4. 设置令牌和聊天 ID。

   ```
   export TELESEND_BOT_TOKEN='<token>'
   export TELESEND_CHAT_ID='<chat-id>'
   ```

5. 发送消息。

   ```
   telesend text "Hello from telesend"
   ```

命令输出 `sent text (message <id>)`，消息出现在聊天中。

## 用法

每个命令发送一种内容。

| 命令 | 发送内容 |
| --- | --- |
| `telesend text [TEXT...]` | 文本消息。未给出文本时读取 `--file` 或标准输入。 |
| `telesend photo SRC...` | 一张图片，或每组最多 10 张的相册。 |
| `telesend video SRC...` | 一个视频，或每组最多 10 个的视频相册。 |
| `telesend media SRC...` | 图片和视频放在同一个相册中。 |
| `telesend file SRC...` | 以文档形式发送文件，不重新压缩。多个文件作为文档相册发送。 |
| `telesend voice SRC` | 把音频文件作为可播放的语音消息发送。 |

`SRC` 是本地路径或 `http(s)` URL。

`telesend text --file notes.md` 读取文件，并把文件内容作为消息文本发送。它不会把文件作为附件发送。要发送附件，使用 `telesend file`。

### 相册

`photo`、`video`、`media` 和 `file` 把多个文件按给定顺序打包成每组最多 10 个的相册。
例如，25 张图片生成三个相册：10 张、10 张和 5 张。剩下的单个文件作为普通消息发送。
说明文字显示在第一个相册的第一个条目上。

`media` 根据文件扩展名区分图片和视频。

| 类型 | 扩展名 |
| --- | --- |
| 图片 | `.jpg` `.jpeg` `.png` `.webp` `.bmp` |
| 视频 | `.mp4` `.mov` `.mkv` `.webm` `.avi` `.m4v` |

Telegram 只允许文档与文档放在同一个相册。因此 `file` 不会与图片或视频混合。

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

加上 `--json`，每个任务输出一个 JSON 对象。对象包含 `name`、`ok`、`message_ids`、`attempts` 和 `error`。

运行 `telesend <command> --help` 查看单个命令的参数和示例。

### Markdown

`--markdown`（`-m`）按 CommonMark 解析输入，输出 MarkdownV2。

| 输入 | Telegram 显示 |
| --- | --- |
| `# Title`、`## Title` | 粗体行 |
| `**bold**`、`*italic*`、`~~strike~~` | 粗体、斜体、删除线 |
| `` `code` `` 和围栏代码块 | 等宽字体 |
| `- item`、`1. item` | 无序列表、有序列表 |
| `> quote` | 引用 |
| `[text](url)` | 链接 |

MarkdownV2 没有表格、任务列表和图片的对应格式。图片变成链接，表格变成纯文本。

### telesend 强制执行的限制

| 限制 | 值 | 行为 |
| --- | --- | --- |
| 文本长度 | 4096 个字符 | 在行尾或 Markdown 块之间拆分。 |
| 说明文字长度 | 1024 个字符 | 报错并停止。 |
| 相册大小 | 10 个条目 | 开始新的相册。 |

Markdown 模式下，超过 4096 个字符的单行会报错并停止。请拆分该行。

## 配置

命令行参数覆盖环境变量。环境变量覆盖默认值。

| 参数 | 环境变量 | 默认值 | 含义 |
| --- | --- | --- | --- |
| `--token` | `TELESEND_BOT_TOKEN` | 无（必填） | 机器人令牌。 |
| `--to`、`-t` | `TELESEND_CHAT_ID` | 无（必填） | 聊天 ID，或 `@channelname`。 |
| `--base-url` | `TELESEND_BASE_URL` | `https://api.telegram.org` | Bot API 地址。 |
| `--rpm` | 无 | `20` | 每分钟最大请求数，范围 1 到 1800。 |
| `--concurrency` | 无 | `1` | 同时进行的请求数，范围 1 到 16。 |
| `--retries` | 无 | `3` | `429`、`5xx` 或网络错误后的重试次数。 |
| `--timeout` | 无 | `2m` | 单次尝试的时间限制。 |
| `--log-level` | 无 | `warn` | stderr 的日志级别：`debug`、`info`、`warn` 或 `error`。 |
| `--verbose`、`-v` | 无 | 关 | 等同于 `--log-level debug`。 |
| `--json` | 无 | 关 | 在 stdout 以 JSON 输出结果。 |

### 速率限制

Telegram 允许向单个聊天每秒约 1 条消息，向单个群组每分钟 20 条消息，所有聊天合计每秒约 30 条消息。
默认的每分钟 20 个请求低于群组限制。
向私聊发送时，可把 `--rpm` 提高到 60。

`--rpm` 决定请求间隔：60 ÷ `rpm` 秒。限流器立即发出第一个请求。
包含 n 个条目的相册占用 n 个 `--rpm` 额度，因为 Telegram 把每个条目算作一条消息。
默认值 20 时，一个 10 个条目的相册约需 27 秒才开始发送。

`--concurrency` 大于 1 时，消息可能乱序到达。需要保持顺序时，使用默认值 1。

第一个任务失败后，telesend 不再启动新任务。它报告失败和未发送的任务数，并以退出码 1 退出。

超时后重试，可能重复发送：Telegram 可能已经收到第一次请求。

### 日志

日志输出到 stderr，结果输出到 stdout。

| 级别 | 显示内容 |
| --- | --- |
| `error` | 每个失败的任务，包含尝试次数和错误。 |
| `warn`（默认） | 重试和未发送的任务。 |
| `info` | 任务数量和每个已发送的任务。 |
| `debug` | 解析后的设置、每次尝试，以及每个 HTTP 状态码和耗时。 |

`telesend` 在每行日志和每条错误信息中，把机器人令牌替换为 `<token>`。

### 语音格式

Telegram 把 `.ogg`（Opus）、`.mp3` 和 `.m4a` 文件作为语音消息播放。telesend 原样上传这些文件。
其他格式用 `ffmpeg -c:a libopus -b:a 32k -ar 48000 -ac 1` 转换。
内容为 Vorbis 的 `.ogg` 文件会作为文档到达，不是语音消息。

## 与 AI Agent 配合使用

`skills/telesend/SKILL.md` 是 Claude Code skill。它把自然语言请求映射为 `telesend` 命令。

1. 安装 `telesend`，并设置环境变量。
2. 把 skill 复制到 skills 目录。

   ```
   cp -r skills/telesend ~/.claude/skills/
   ```

3. 向 Agent 提出请求，例如：“把 build.log 发到 Telegram。”

## 开发

```
make check
```

`make check` 运行格式检查、`golangci-lint`、`go vet`、测试和 `goreleaser check`。
测试使用模拟 Telegram 服务器，不需要令牌和网络。

## 许可证

[MIT](LICENSE)
