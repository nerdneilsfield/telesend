# Usage

[简体中文](usage.zh-CN.md) · [Configuration](configuration.md) · [Use with an AI agent](agent.md)

Each command sends one kind of content. `SRC` is a local path or an `http(s)` URL.

| Command | What it sends |
| --- | --- |
| `telesend text [TEXT...]` | A text message. |
| `telesend photo SRC...` | One photo, or albums of photos. |
| `telesend video SRC...` | One video, or albums of videos. |
| `telesend media SRC...` | Photos and videos in the same albums. |
| `telesend file SRC...` | Files as documents. Several files make document albums. |
| `telesend voice SRC` | An audio file as a voice message. |

Run `telesend <command> --help` to see the flags and examples of one command.

## Text

Pass the text as arguments, read it from a file, or pipe it in.

```
telesend text "Build finished"
telesend text --file notes.md
git log -5 --oneline | telesend text
```

`--file` sends the content of the file as the message text. It does not attach the file. To attach a file, use `telesend file`.

A long text is cut into several messages. The messages arrive in order. The default maximum is 4096 characters per message. Set a lower value with `--text-limit`.

Plain text is cut at line ends. Markdown text is cut between paragraphs, between list items, between the words of a paragraph, and between the lines of a code block. A heading, a list item, a quote, or a link that is longer than the limit stops with an error. Shorten it, or raise `--text-limit`.

### Markdown

Add `--markdown` (`-m`) to format the text. telesend writes the Markdown in the form that Telegram needs, and it escapes the characters that Telegram rejects.

| You write | Telegram shows |
| --- | --- |
| `# Title`, `## Title` | A bold line |
| `**bold**`, `*italic*`, `~~strike~~` | Bold, italic, strikethrough |
| `` `code` `` and fenced code blocks | Monospace text |
| `- item`, `1. item` | A bullet list, a numbered list |
| `> quote` | A quote |
| `[text](url)` | A link |

Tables, task lists, and images have no Telegram form. A table shows as plain text. An image shows as a link.

## Photos, videos, and files

Give many files in one command. telesend packs them into albums and sends the albums in the order that you give.

```
telesend photo a.jpg b.jpg c.jpg --caption "Trip"
telesend video demo.mp4
telesend media shot1.png shot2.png demo.mp4 --caption "Release 2"
telesend file report.pdf logs.zip
telesend photo ./shots/*.png
```

An album holds 10 items by default. Set another size with `--album-size`. For example, 25 photos make three albums: 10, 10, and 5 photos. A single leftover item goes out as a normal message.

The caption appears on the first item of the first album. A caption has up to 1024 characters.

`media` tells photos from videos by the file extension.

| Kind | Extensions |
| --- | --- |
| Photo | `.jpg` `.jpeg` `.png` `.webp` `.bmp` |
| Video | `.mp4` `.mov` `.mkv` `.webm` `.avi` `.m4v` |

Telegram accepts documents in an album only with other documents. Use `file` for documents, and `media` for photos and videos.

A local path must name a regular file. A directory, a pipe, or a device stops with an error.

Telegram recompresses photos and limits them to 10 MB. To keep the original image, send it with `telesend file`.

Bots upload up to 50 MB per file through `api.telegram.org`. A self-hosted Bot API server accepts larger files.

## Voice

```
telesend voice note.ogg
telesend voice meeting.wav --caption "Standup"
```

Telegram plays `.ogg` (Opus), `.mp3`, and `.m4a` files as voice messages. telesend sends these files as they are.

For any other local format, telesend converts the file to OGG/Opus first. This step needs `ffmpeg`. An `.ogg` file that holds Vorbis audio arrives as a document, not as a voice message.

The Docker image does not include `ffmpeg`.

telesend converts local files only. A URL to a `.wav` or another unsupported format stops with an error. Download the file, and pass the local path.

## What you see

In a terminal, telesend shows a progress bar on stderr. The bar shows the percentage, the transfer speed, and the time left. It tracks bytes for uploads, and it tracks messages for text.

After each send, telesend prints one line with the message ID and the time that the request took. At the end, it prints a summary with the total time, the average speed, and the average request time.

```
sent photo a.jpg  message 41  0.8 s  2.1 MB at 2.6 MB/s
sent photo b.jpg  message 42  1.1 s  3.0 MB at 2.7 MB/s
Sent 2 of 2 in 6.4 s. 5.1 MB at 2.7 MB/s on average. Average request time 0.9 s.
```

Add `--json` for machine-readable output. In JSON mode, telesend prints no progress bar and no summary line. See [Use with an AI agent](agent.md).
