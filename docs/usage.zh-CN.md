# 用法

[English](usage.md) · [配置](configuration.zh-CN.md) · [与 AI Agent 配合使用](agent.zh-CN.md)

每个命令发送一种内容。`SRC` 是本地路径或 `http(s)` URL。

| 命令 | 发送内容 |
| --- | --- |
| `telesend text [TEXT...]` | 文本消息。 |
| `telesend photo SRC...` | 一张图片，或图片相册。 |
| `telesend video SRC...` | 一个视频，或视频相册。 |
| `telesend media SRC...` | 图片和视频放在同一个相册中。 |
| `telesend file SRC...` | 以文档形式发送文件。多个文件组成文档相册。 |
| `telesend voice SRC` | 把音频文件作为语音消息发送。 |

运行 `telesend <command> --help`，查看单个命令的参数和示例。

## 文本

把文本作为参数传入，或从文件读取，或通过管道传入。

```
telesend text "Build finished"
telesend text --file notes.md
git log -5 --oneline | telesend text
```

要发送以 `-` 开头的文本，在它前面加 `--`：`telesend text -- "-1 degree"`。

`--file` 把文件内容作为消息文本发送，不会把文件作为附件发送。要发送附件，使用 `telesend file`。

长文本会拆分为多条消息，消息按顺序到达。每条消息默认最多 4096 个字符。用 `--text-limit` 设置更小的值。

纯文本在行尾拆分。Markdown 文本在段落之间、列表项之间、段落内的单词之间和代码块的行之间拆分。比限制更长的标题、列表项、引用或链接会报错并停止。请缩短它，或提高 `--text-limit`。

### Markdown

加上 `--markdown`（`-m`）格式化文本。telesend 把 Markdown 写成 Telegram 需要的形式，并转义 Telegram 会拒绝的字符。

| 你写的内容 | Telegram 显示 |
| --- | --- |
| `# Title`、`## Title` | 粗体行 |
| `**bold**`、`*italic*`、`~~strike~~` | 粗体、斜体、删除线 |
| `` `code` `` 和围栏代码块 | 等宽文本 |
| `- item`、`1. item` | 无序列表、有序列表 |
| `> quote` | 引用 |
| `[text](url)` | 链接 |

表格、任务列表和图片在 Telegram 中没有对应形式。表格显示为纯文本，图片显示为链接。

## 图片、视频和文件

在一条命令中给出多个文件。telesend 把它们打包成相册，并按你给出的顺序发送。

```
telesend photo a.jpg b.jpg c.jpg --caption "Trip"
telesend video demo.mp4
telesend media shot1.png shot2.png demo.mp4 --caption "Release 2"
telesend file report.pdf logs.zip
telesend photo ./shots/*.png
```

相册默认包含 10 个条目。用 `--album-size` 设置其他大小。例如，25 张图片生成三个相册：10 张、10 张和 5 张。剩下的单个条目作为普通消息发送。

说明文字显示在第一个相册的第一个条目上。说明文字最多 1024 个字符。

`media` 根据文件扩展名区分图片和视频。

| 类型 | 扩展名 |
| --- | --- |
| 图片 | `.jpg` `.jpeg` `.png` `.webp` `.bmp` |
| 视频 | `.mp4` `.mov` `.mkv` `.webm` `.avi` `.m4v` |

Telegram 只允许文档与文档放在同一个相册。文档用 `file` 发送，图片和视频用 `media` 发送。

本地路径必须是普通文件。目录、管道或设备会报错并停止。

Telegram 会重新压缩图片，并把图片限制在 10 MB。要保留原图，用 `telesend file` 发送。

机器人通过 `api.telegram.org` 上传的单个文件最大 50 MB。自建的 Bot API 服务器接受更大的文件。

## 语音

```
telesend voice note.ogg
telesend voice meeting.wav --caption "Standup"
```

Telegram 把 `.ogg`（Opus）、`.mp3` 和 `.m4a` 文件作为语音消息播放。telesend 原样发送这些文件。

其他本地格式先由 telesend 转换为 OGG/Opus。这一步需要 `ffmpeg`。内容为 Vorbis 的 `.ogg` 文件会作为文档到达，不是语音消息。

Docker 镜像不包含 `ffmpeg`。

telesend 只转换本地文件。指向 `.wav` 或其他不支持格式的 URL 会报错并停止。请先下载文件，再传入本地路径。

## 你看到的输出

在终端中，telesend 在 stderr 显示进度条。进度条显示百分比、传输速度和剩余时间。上传文件时按字节计算，发送文本时按消息数计算。

每次发送后，telesend 输出一行，包含消息 ID 和请求耗时。结束时，它输出汇总，包含总耗时、平均速度和平均请求耗时。

```
sent photo a.jpg  message 41  0.8 s  2.1 MB at 2.6 MB/s
sent photo b.jpg  message 42  1.1 s  3.0 MB at 2.7 MB/s
Sent 2 of 2 in 6.4 s. 5.1 MB at 2.7 MB/s on average. Average request time 0.9 s.
```

加上 `--json` 得到机器可读的输出。JSON 模式下，telesend 不显示进度条，也不输出汇总行。见[与 AI Agent 配合使用](agent.zh-CN.md)。
