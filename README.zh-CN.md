# telesend

[English](README.md)

在命令行中向 Telegram 发送文本、文件、图片、视频和语音消息。

```
telesend text "Build finished"
telesend photo a.jpg b.jpg c.jpg --caption "Trip"
```

## 安装

需要 Go 1.27 或更高版本。

```
go install github.com/dengqi/telesend@latest
```

## 快速开始

1. 用 [@BotFather](https://t.me/BotFather) 创建机器人，复制机器人令牌。
2. 打开与机器人的聊天，给它发任意一条消息。
3. 在浏览器中打开 `https://api.telegram.org/bot<token>/getUpdates`，把 `<token>` 换成你的机器人令牌。
4. 在返回内容中找到 `message.chat.id`。这个数字就是你的聊天 ID。
5. 设置令牌和聊天 ID。

   ```
   export TELESEND_BOT_TOKEN='<token>'
   export TELESEND_CHAT_ID='<chat-id>'
   ```

6. 发送消息。

   ```
   telesend text "Hello from telesend"
   ```

消息出现在聊天中。命令输出一行，包含消息 ID。

## 常用操作

```
telesend text --file notes.md --markdown
git log -5 --oneline | telesend text
telesend photo ./shots/*.png
telesend media shot.png demo.mp4 --caption "Release 2"
telesend file report.pdf
telesend voice meeting.wav
```

在终端中，telesend 显示进度条，包含速度和剩余时间。程序读取输出时，加上 `--json`。

运行 `telesend --help` 或 `telesend <command> --help`，查看全部命令和参数。

## 了解更多

- [用法](docs/usage.zh-CN.md)：全部命令、Markdown、相册、语音
- [配置](docs/configuration.zh-CN.md)：参数、环境变量、发送速度、日志
- [与 AI Agent 配合使用](docs/agent.zh-CN.md)：Claude Code skill 和 JSON 输出
- [内部实现](docs/internals.md)：面向贡献者（仅英文）

## 许可证

[MIT](LICENSE)
