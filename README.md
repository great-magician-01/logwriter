# logwriter

**中文 | [English](README.en.md)**

[![CI](https://github.com/great-magician-01/logwriter/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/great-magician-01/logwriter/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/great-magician-01/logwriter.svg)](https://pkg.go.dev/github.com/great-magician-01/logwriter)

一个只依赖 Go 标准库的异步日志库，支持自定义 Formatter、控制台与文件输出，以及按天、按大小切割日志文件。

## 特性

- 自定义 `Formatter`，内置普通文本格式和可选的 ANSI 级别颜色。
- 控制台、文件或两者同时输出；两个输出可以使用不同的 Formatter。
- 按输出逐条过滤日志：`FileOnly` 标记的日志只写入文件、不进控制台。
- 按天、按大小，或按天和大小组合切割；也可按天分目录并保留原文件名。
- 有界队列接收日志，单个后台 goroutine 串行执行格式化、输出和切割。
- 文件写入不使用互斥锁；文件句柄、大小和序号仅由队列消费者访问。
- 支持日志级别、结构化字段和调用位置。
- 程序重启后继续追加，按数字恢复最大文件序号。
- `Close()` 停止接收新日志，排空队列并关闭文件。
- 后台错误通过 `OnError` 回调和 `Close()` 返回值报告。
- GitHub Actions 自动执行格式检查、静态分析、竞态测试和构建。
- 使用 release-please 根据 Conventional Commits 管理后续版本。

## 安装

要求 Go 1.21 及以上。

```bash
go get github.com/great-magician-01/logwriter@latest
```

业务代码只需导入根包：

```go
import "github.com/great-magician-01/logwriter"
```

## 快速开始

同时输出到控制台和文件，按天切割，单文件最大 200 MiB：

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

文件中的默认文本输出（控制台仅级别名带 ANSI 颜色，文件不包含颜色转义符）：

```text
2026-11-04 15:30:20.123 [INFO] user logged in user_id=10086
```

`Info()` 返回 `nil` 表示日志已被接受并入队，不表示已经落盘。程序退出前应调用 `Close()`，确保队列排空并接收后台写入错误。

## 示例代码

仓库包含可以直接运行的完整示例：

- [examples/basic/main.go](examples/basic/main.go)：控制台有色、文件无色的双输出，以及按天和大小切割。
- [examples/custom_formatter/main.go](examples/custom_formatter/main.go)：自定义 JSON Formatter、结构化字段和调用位置。

在仓库根目录运行：

```bash
go run ./examples/basic
go run ./examples/custom_formatter
```

`basic` 示例会在当前目录创建 `logs`；`custom_formatter` 示例向控制台输出一行 JSON。

## 自定义 Formatter

实现 `Formatter` 接口即可完全控制日志字节：

```go
type Formatter interface {
    Format(logwriter.Entry) ([]byte, error)
}
```

普通函数可以直接使用 `FormatterFunc`：

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

Formatter 在后台消费者中串行调用，不要求其实现支持并发。需要换行时，由 Formatter 自行添加 `\n`；文件大小按照格式化后的实际字节数计算。

### 控制台有色、文件无色

`ConsoleFormatter` 和 `FileFormatter` 分别覆盖控制台与文件使用的 Formatter；未设置的输出继续使用 `Formatter`。内置 `TextFormatter` 的 `Color` 开关会为级别名添加 ANSI 颜色：DEBUG 为青色、INFO 为绿色、WARN 为黄色、ERROR 为红色。

```go
logger, err := logwriter.New(logwriter.Config{
    // Formatter 可省略，默认是两个输出共用的无色 TextFormatter。
    Formatter:        logwriter.TextFormatter{},
    ConsoleFormatter: logwriter.TextFormatter{Color: true},
    Console:          true,
    File:             true,
    Directory:        "logs",
})
```

上例中，文件未设置 `FileFormatter`，因此使用无色的 `Formatter`；控制台使用带色 Formatter。也可以设置自定义的 `FileFormatter`，让文件使用 JSON 等完全不同的格式。

两个输出级 Formatter 都未设置时，共享 Formatter 每条日志只调用一次，同一份字节写入所有输出（被过滤器或 `FileOnly` 排除的输出除外）。设置任一输出级 Formatter 后，各输出独立格式化，一路 Formatter 返回错误不会阻断另一路；文件切割按照文件自身格式化后的字节数计算。`Color` 是显式开关，不会自动探测终端，重定向控制台输出时应按需关闭。

## 按输出过滤单条日志

`ConsoleFilter` 和 `FileFilter` 逐条决定一条日志是否进入对应输出，类型为 `func(logwriter.Entry) bool`，返回 `true` 表示写入。过滤器返回 `false` 时，该输出跳过这条日志的格式化和写入；两个过滤器都为 `nil` 时接受全部日志。例如过滤掉 health check 刷屏：

```go
logger, err := logwriter.New(logwriter.Config{
    Console:   true,
    File:      true,
    Directory: "logs",
    ConsoleFilter: func(entry logwriter.Entry) bool {
        return entry.Message != "health check"
    },
})
```

`FileOnly()` 返回一个标记字段：把它作为字段传入任意日志方法，这条日志就只写入文件、跳过控制台，适合记录 token、payload 等不适合在终端展示的内容。

```go
_ = logger.Info("service started")
_ = logger.Info("token refreshed", logwriter.FileOnly())
```

标记字段在格式化前会被移除，不会出现在输出里；自定义 Formatter 可以通过 `Entry.FileOnly` 判断标记。只启用控制台时，`FileOnly` 日志会被直接丢弃。过滤器在后台消费者中串行调用，不要求并发安全，其 panic 与 Formatter 的 panic 一样被恢复并转换为后台错误。

## 文件命名规则

假设 `Directory` 为 `logs`，`FilePrefix` 使用默认值 `app`。

### 不切割

```go
Config{File: true, Directory: "logs"}
```

```text
logs/app.log
```

### 仅按天切割

```go
Config{File: true, Directory: "logs", RotateByDay: true}
```

```text
logs/20261104/20261104.log
logs/20261105/20261105.log
```

### 仅按大小切割

```go
Config{File: true, Directory: "logs", MaxSize: 200 * 1024 * 1024}
```

```text
logs/app_1.log
logs/app_2.log
```

### 按天和大小组合切割

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

### 按天分目录（保留原文件名）

```go
Config{File: true, Directory: "logs", DirectoryByDay: true}
```

```text
logs/2026-11-04/app.log
logs/2026-11-05/app.log
```

目录名为 `2006-01-02` 格式的本地日期，文件名保持 `FilePrefix` 原名；与 `MaxSize` 组合时加上相同的序号后缀，例如 `logs/2026-11-04/app_1.log`。`DirectoryByDay` 与 `RotateByDay` 互斥，同时设置会在 `New()` 时返回错误。

切割行为：

- 写入前检查大小；当前文件大小加本条日志超过 `MaxSize` 时，先切换到新文件。
- 跨天后序号重新从 `1` 开始。
- 跨天在实际处理下一条日志时生效，不在零点创建空文件。
- 单条日志超过 `MaxSize` 时不拆分该条日志，允许其单独超过限制。
- 重启后追加到当天最新的未满文件；最新文件已满时创建下一个序号。
- `DirectoryByDay` 的跨天切换、序号和重启追加行为与 `RotateByDay` 一致。
- `MaxSize` 单位为字节，示例中的 `200 * 1024 * 1024` 表示 200 MiB。

## 队列与并发

`QueueSize` 默认为 `1024`。队列是有界的：

- 多个业务 goroutine 可以同时调用日志方法。
- 队列满时，日志调用会阻塞等待，形成背压，不主动丢弃日志。
- 关闭与入队状态使用原子操作协调；文件写入和切割没有互斥锁。
- `Close()` 可并发、重复调用，会等待已经接受的日志处理完成。
- 开始关闭后，新日志调用返回 `logwriter.ErrClosed`。

`OnError` 在后台消费者中同步调用。回调应快速返回，且不要调用该 Logger 的日志方法或 `Close()`，否则队列已满时可能阻塞，直接调用 `Close()` 还会形成自等待。

后台消费者会恢复处理日志过程中的 panic（如 `Formatter` 或 `ConsoleWriter` 抛出的），将其转换为错误并通过 `OnError` 和 `Close()` 返回，不会导致进程崩溃；`OnError` 自身 panic 时同样被遏制，并并入 `Close()` 的返回值。

## 配置项

| 字段 | 默认值 | 说明 |
|---|---|---|
| `Level` | `DebugLevel` | 最低输出级别 |
| `Formatter` | `TextFormatter{}` | 输出级 Formatter 未设置时共用的日志格式化器 |
| `ConsoleFormatter` | `nil` | 控制台专用 Formatter，`nil` 时使用 `Formatter` |
| `FileFormatter` | `nil` | 文件专用 Formatter，`nil` 时使用 `Formatter` |
| `ConsoleFilter` | `nil` | 控制台过滤器，返回 `false` 跳过该条日志的控制台输出；`nil` 接受全部 |
| `FileFilter` | `nil` | 文件过滤器，返回 `false` 跳过该条日志的文件输出；`nil` 接受全部 |
| `Console` | `false` | 是否输出控制台 |
| `ConsoleWriter` | `os.Stdout` | 控制台目标，可替换为其他 `io.Writer` |
| `File` | `false` | 是否输出文件 |
| `Directory` | `logs` | 日志根目录 |
| `FilePrefix` | `app` | 基础文件名（`RotateByDay` 模式下文件名使用日期），不能包含路径 |
| `RotateByDay` | `false` | 是否按本地日期创建目录并以日期命名、切割文件 |
| `DirectoryByDay` | `false` | 是否按本地日期（`2006-01-02`）分目录并保留 `FilePrefix` 原文件名；与 `RotateByDay` 互斥 |
| `MaxSize` | `0` | 单文件最大字节数，`0` 表示不按大小切割 |
| `QueueSize` | `1024` | 队列容量，必须大于等于 `0` |
| `Caller` | `false` | 是否采集调用文件和行号 |
| `OnError` | `nil` | 后台格式化、输出和关闭错误回调 |

至少启用 `Console` 或 `File` 之一。

## 包结构

```text
logwriter.go               对外统一 API
internal/record/           日志模型与级别
internal/formatter/        Formatter 与默认文本格式
internal/writer/           文件输出及切割
internal/core/             队列、单消费者、并发关闭与错误处理
examples/                  可运行示例
.github/workflows/         CI 与 release-please 工作流
```

`internal` 下的包仅供本模块内部使用，外部项目统一导入根包。

## CI

[ci.yml](.github/workflows/ci.yml) 在向 `master` 推送、向 `master` 提交 PR 或手动触发时运行：

1. 使用 `go.mod` 中的 Go 版本。
2. 检查 `go fmt` 是否产生变更。
3. 执行 `go vet ./...`。
4. 执行 `go test -race -coverprofile=coverage.out ./...`。
5. 构建库和全部 examples。

本地运行竞态测试需要启用 CGO，并安装 C 编译器；Ubuntu 的 GitHub Actions 环境已提供这些依赖。

## 版本发布

发布配置：

- [release-please.yml](.github/workflows/release-please.yml)：在 `master` 更新后运行。
- [release-please-config.json](release-please-config.json)：Go 项目、标签和 CHANGELOG 规则。
- [.release-please-manifest.json](.release-please-manifest.json)：当前版本基线为 `1.0.0`。

### 首个版本：手动发布 v1.0.0

`v1.0.0` 不由 release-please 自动创建。当前仓库尚未产生首个提交时，可按以下顺序建立基线：

```bash
git add .
git commit -m "feat: initial logwriter implementation"
git tag -a v1.0.0 -m "v1.0.0"
git push origin v1.0.0
```

先在 GitHub 的 **Releases** 页面基于 `v1.0.0` 创建首个 Release，再推送默认分支：

```bash
git push -u origin master
```

这个顺序可避免 release-please 在首个标签和 Release 建立前，就把首个版本误判为待发布变更。如果仓库已有提交，只需在首个正式版本对应的提交上创建并推送 `v1.0.0`，再创建 GitHub Release。

### 后续版本：release-please 自动管理

提交信息遵循 [Conventional Commits](https://www.conventionalcommits.org/zh-hans/)：

| 提交示例 | 版本变化 |
|---|---|
| `fix: handle file reopen failure` | 修订号，例如 `v1.0.0 -> v1.0.1` |
| `feat: add hourly rotation` | 次版本，例如 `v1.0.1 -> v1.1.0` |
| `feat!: redesign formatter API` | 主版本，例如 `v1.1.0 -> v2.0.0` |

合并到 `master` 后，release-please 会创建或更新一个 Release PR。该 PR 包含版本号、`.release-please-manifest.json` 和 `CHANGELOG.md` 变更；**合并 Release PR 后**，release-please 自动创建对应的 Git 标签和 GitHub Release。

还需要在 GitHub 仓库的 **Settings → Actions → General** 中允许 GitHub Actions 创建 Pull Request。工作流已声明 `contents: write` 和 `pull-requests: write` 权限。

## 本地检查

```bash
go fmt ./...
go vet ./...
go test ./...
go test -race ./...
go build ./...
```
