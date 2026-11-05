# logwriter

**[中文](README.md) | English**

[![CI](https://github.com/great-magician-01/logwriter/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/great-magician-01/logwriter/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/great-magician-01/logwriter.svg)](https://pkg.go.dev/github.com/great-magician-01/logwriter)

An asynchronous Go logging library using only the standard library. It supports custom formatters, console and file outputs, and log rotation by day and file size.

## Features

- Custom `Formatter` support with a built-in plain-text formatter and optional ANSI level colors.
- Console output, file output, or both, with independent formatters for each output.
- Rotation by day, size, or both.
- A bounded queue and one background goroutine that serializes formatting, output, and rotation.
- No mutex around file writes; only the queue consumer accesses file handles, sizes, and indexes.
- Log levels, structured fields, and caller information.
- Append after restarts with numeric recovery of the highest file index.
- `Close()` stops accepting entries, drains the queue, and closes the current file.
- Background errors are reported through `OnError` and the error returned by `Close()`.
- GitHub Actions for formatting, static analysis, race-enabled tests, and builds.
- Subsequent releases managed by release-please using Conventional Commits.

## Installation

Requires Go 1.21 or later.

```bash
go get github.com/great-magician-01/logwriter@latest
```

Applications only need to import the root package:

```go
import "github.com/great-magician-01/logwriter"
```

## Quick start

Write to both the console and files, rotate daily, and limit each file to 200 MiB:

```go
package main

import (
    "log"

    "github.com/great-magician-01/logwriter"
)

func main() {
    logger, err := logwriter.New(logwriter.Config{
        Level:            logwriter.InfoLevel,
        Console:          true,
        ConsoleFormatter: logwriter.TextFormatter{Color: true},
        File:             true,
        Directory:        "logs",
        RotateByDay:      true,
        MaxSize:          200 * 1024 * 1024,
        QueueSize:        1024,
        OnError: func(err error) {
            log.Printf("logger error: %v", err)
        },
    })
    if err != nil {
        log.Fatal(err)
    }

    if err := logger.Info(
        "user logged in",
        logwriter.Field{Key: "user_id", Value: 10086},
    ); err != nil {
        _ = logger.Close()
        log.Fatal(err)
    }

    if err := logger.Close(); err != nil {
        log.Fatal(err)
    }
}
```

Default plain-text file output (only the console level name has ANSI color; the file contains no color escapes):

```text
2026-11-04 15:30:20.123 [INFO] user logged in user_id=10086
```

A `nil` return from `Info()` means the entry was accepted into the queue, not that it has reached the disk. Call `Close()` before exiting to drain the queue and receive background write errors.

## Examples

The repository contains complete runnable examples:

- [examples/basic/main.go](examples/basic/main.go): colored console output, plain file output, and day-and-size rotation.
- [examples/custom_formatter/main.go](examples/custom_formatter/main.go): a custom JSON formatter, structured fields, and caller information.

Run them from the repository root:

```bash
go run ./examples/basic
go run ./examples/custom_formatter
```

The `basic` example creates `logs` in the current directory. The `custom_formatter` example writes one JSON line to the console.

## Custom Formatter

Implement `Formatter` to control the complete output bytes:

```go
type Formatter interface {
    Format(logwriter.Entry) ([]byte, error)
}
```

Use `FormatterFunc` to adapt an ordinary function:

```go
formatter := logwriter.FormatterFunc(func(entry logwriter.Entry) ([]byte, error) {
    line := fmt.Sprintf(
        "%s [%s] %s",
        entry.Time.Format(time.RFC3339Nano),
        entry.Level,
        entry.Message,
    )
    return []byte(line + "\n"), nil
})

logger, err := logwriter.New(logwriter.Config{
    Formatter: formatter,
    Console:   true,
    File:      true,
    Directory: "logs",
})
```

Formatters are called serially by the background consumer and do not need to be concurrency-safe. A formatter must add its own `\n` when a newline is desired. File rotation uses the actual formatted byte length.

### Colored console, plain file

`ConsoleFormatter` and `FileFormatter` override the formatter for their respective outputs. An output without an override continues to use `Formatter`. The built-in `TextFormatter` has a `Color` option that adds ANSI colors to level names: cyan for DEBUG, green for INFO, yellow for WARN, and red for ERROR.

```go
logger, err := logwriter.New(logwriter.Config{
    // Formatter may be omitted; it defaults to a shared, colorless TextFormatter.
    Formatter:        logwriter.TextFormatter{},
    ConsoleFormatter: logwriter.TextFormatter{Color: true},
    Console:          true,
    File:             true,
    Directory:        "logs",
})
```

Here, the file has no `FileFormatter`, so it uses the plain `Formatter`, while the console uses the colored formatter. A custom `FileFormatter` can also give the file an entirely different format, such as JSON.

When neither output-specific formatter is set, the shared formatter is called once per entry and the same bytes go to every output. Setting either override makes the outputs format independently, and an error returned by one output's formatter does not block the other. File rotation uses the byte length produced by the file's own formatter. `Color` is explicit and does not perform terminal detection; disable it as needed when redirecting console output.

## File naming

These examples assume `Directory` is `logs` and `FilePrefix` uses its default value, `app`.

### No rotation

```go
Config{File: true, Directory: "logs"}
```

```text
logs/app.log
```

### Daily rotation only

```go
Config{File: true, Directory: "logs", RotateByDay: true}
```

```text
logs/20261104/20261104.log
logs/20261105/20261105.log
```

### Size rotation only

```go
Config{File: true, Directory: "logs", MaxSize: 200 * 1024 * 1024}
```

```text
logs/app_1.log
logs/app_2.log
```

### Combined day and size rotation

```go
Config{
    File:        true,
    Directory:   "logs",
    RotateByDay: true,
    MaxSize:     200 * 1024 * 1024,
}
```

```text
logs/20261104/20261104_1.log
logs/20261104/20261104_2.log
logs/20261105/20261105_1.log
```

Rotation behavior:

- Size is checked before writing. If the current size plus the next entry exceeds `MaxSize`, the writer rotates first.
- The index resets to `1` for a new day.
- Day rollover happens when the next entry is processed; no empty file is created at midnight.
- An entry larger than `MaxSize` is kept intact and may exceed the limit in its own file.
- After a restart, the writer appends to the latest non-full file or creates the next index when that file is full.
- `MaxSize` is measured in bytes; `200 * 1024 * 1024` means 200 MiB.

## Queue and concurrency

`QueueSize` defaults to `1024`. The queue is bounded:

- Multiple application goroutines may call logging methods concurrently.
- A full queue blocks producers to apply backpressure rather than silently dropping logs.
- Atomic operations coordinate submission and shutdown; file writing and rotation use no mutex.
- `Close()` is idempotent, supports concurrent callers, and waits for all accepted entries.
- After shutdown starts, new logging calls return `logwriter.ErrClosed`.

`OnError` runs synchronously in the background consumer. It should return quickly and must not call logging methods or `Close()` on the same Logger. Logging from the callback can block when the queue is full, and calling `Close()` from the consumer waits on itself.

The background consumer recovers panics raised while processing an entry (for example from the `Formatter` or `ConsoleWriter`), converts them into errors reported through `OnError` and `Close()`, and keeps running; a panicking `OnError` is contained as well and joined into the error returned by `Close()`.

## Configuration

| Field | Default | Description |
|---|---|---|
| `Level` | `DebugLevel` | Minimum output level |
| `Formatter` | `TextFormatter{}` | Shared formatter when an output-specific formatter is not set |
| `ConsoleFormatter` | `nil` | Console formatter; falls back to `Formatter` when `nil` |
| `FileFormatter` | `nil` | File formatter; falls back to `Formatter` when `nil` |
| `Console` | `false` | Enable console output |
| `ConsoleWriter` | `os.Stdout` | Console destination; may be another `io.Writer` |
| `File` | `false` | Enable file output |
| `Directory` | `logs` | Log root directory |
| `FilePrefix` | `app` | Base file name for non-daily mode; paths are rejected |
| `RotateByDay` | `false` | Create local-date directories and rotate daily |
| `MaxSize` | `0` | Maximum file size in bytes; `0` disables size rotation |
| `QueueSize` | `1024` | Queue capacity; must be non-negative |
| `Caller` | `false` | Capture the calling file and line |
| `OnError` | `nil` | Callback for background formatting, output, and close errors |

At least one of `Console` or `File` must be enabled.

## Package layout

```text
logwriter.go               Public facade
internal/record/           Log model and levels
internal/formatter/        Formatter and default text format
internal/writer/           File output and rotation
internal/core/             Queue, consumer, shutdown, and errors
examples/                  Runnable examples
.github/workflows/         CI and release-please workflows
```

Packages under `internal` are private to this module. External applications import only the root package.

## CI

[ci.yml](.github/workflows/ci.yml) runs on pushes to `master`, pull requests targeting `master`, and manual dispatches:

1. Uses the Go version declared in `go.mod`.
2. Checks that `go fmt` produces no changes.
3. Runs `go vet ./...`.
4. Runs `go test -race -coverprofile=coverage.out ./...`.
5. Builds the library and all examples.

Local race-enabled tests require CGO and a C compiler. GitHub's Ubuntu runners provide both.

## Releases

Release configuration:

- [release-please.yml](.github/workflows/release-please.yml): runs after `master` is updated.
- [release-please-config.json](release-please-config.json): Go project, tag, and CHANGELOG rules.
- [.release-please-manifest.json](.release-please-manifest.json): starts from the `1.0.0` baseline.

### First version: publish v1.0.0 manually

release-please does not create `v1.0.0`. While the repository has no initial commit, establish the baseline in this order:

```bash
git add .
git commit -m "feat: initial logwriter implementation"
git tag -a v1.0.0 -m "v1.0.0"
git push origin v1.0.0
```

Create the first Release from `v1.0.0` on GitHub's **Releases** page, then push the default branch:

```bash
git push -u origin master
```

This order prevents release-please from treating the initial implementation as pending work before its baseline tag and Release exist. If the repository already has commits, create and push `v1.0.0` on the intended first-release commit, then create its GitHub Release.

### Later versions: managed automatically by release-please

Use [Conventional Commits](https://www.conventionalcommits.org/en/):

| Commit example | Version change |
|---|---|
| `fix: handle file reopen failure` | Patch, such as `v1.0.0 -> v1.0.1` |
| `feat: add hourly rotation` | Minor, such as `v1.0.1 -> v1.1.0` |
| `feat!: redesign formatter API` | Major, such as `v1.1.0 -> v2.0.0` |

After changes are merged into `master`, release-please creates or updates a Release PR containing the version, `.release-please-manifest.json`, and `CHANGELOG.md` changes. **Merging that Release PR** causes release-please to create the corresponding Git tag and GitHub Release.

In the repository's **Settings → Actions → General**, allow GitHub Actions to create pull requests. The workflow already declares `contents: write` and `pull-requests: write` permissions.

## Local checks

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
go build ./...
```
