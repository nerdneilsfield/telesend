# Internals

This page is for contributors. It is available in English only.

## Layout

| Path | Role |
| --- | --- |
| `cmd/` | The cobra commands. Each command builds a list of jobs and calls `runSend`. |
| `internal/config` | Resolves flags, environment variables, and defaults. |
| `internal/telegram` | The Bot API client: multipart upload, error type, send methods. |
| `internal/mdv2` | CommonMark to MarkdownV2, and the text splitter. |
| `internal/dispatch` | The limiter, the worker pool, and the retry loop. |
| `internal/audio` | The `ffmpeg` conversion for voice files. |
| `internal/logging` | The leveled logger and the token mask. |

## Send pipeline

1. A command reads its input and checks that local files exist.
2. The command builds jobs. A job is one API request: a text part, a single file, or an album.
3. `dispatch.Run` feeds the jobs, in order, to a pool of workers.
4. A worker waits for the limiter, calls the job, and retries a temporary failure.
5. `runSend` prints the results.

## Limiter and pool

The limiter keeps a slot clock. One slot is 60 s ÷ `rpm`. A job of weight n starts at the next free slot, and then it closes the next n slots. The weight of an album is its item count. The burst of an album is paid for after it, so the long-run rate is `rpm` messages per minute. A single 60 s window can exceed `rpm` by less than the size of one album.

On a `429` reply, the worker calls `Limiter.Pause` with the `retry_after` value. A slot that falls inside the pause is void, and its worker reserves a new slot after the pause. The workers then restart one slot apart.

The first permanent failure cancels the run. A job that did not start becomes `Skipped`. A job with zero attempts and a canceled context also becomes `Skipped`.

## Retry rules

| Error | Retry |
| --- | --- |
| `429` | Yes, after `retry_after`. |
| `5xx`, network error, request timeout, reply cut in the middle | Yes, after 1 s × 2^attempt plus up to 25% jitter. |
| Other `4xx` | No. |
| Local file read error | No. |

## Upload

The client streams the multipart body through an `io.Pipe`. A file is never held in memory.

Before it sends, the client computes the exact body length from the field sizes and the file sizes, and it sets `Content-Length`. Some Bot API servers do not accept chunked bodies. A file that changes size during the upload fails the request.

A job opens its files again on every attempt, so a retry starts from byte 0.

## Token mask

The token is part of the request URL. Two layers hide it:

1. The client removes the token from the URL of a transport error and from the description of an API error.
2. The logger core replaces the token, and the part of the token after the colon, in every message and field.

## Tests

The tests use `httptest` as a mock Telegram server. They need no token and no network.

The dispatch tests check the peak concurrency, the spacing between requests, the retry rules, and goroutine leaks.
