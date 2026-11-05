package core

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/great-magician-01/logwriter/internal/formatter"
	"github.com/great-magician-01/logwriter/internal/record"
	logfile "github.com/great-magician-01/logwriter/internal/writer"
)

const DefaultQueueSize = 1024

var ErrClosed = errors.New("logwriter: logger is closed")

// Config controls logging, outputs, queuing, and file rotation.
// ConsoleFormatter and FileFormatter optionally override Formatter for their
// respective outputs. A nil output-specific formatter falls back to Formatter.
type Config struct {
	Level            record.Level
	Formatter        formatter.Formatter
	ConsoleFormatter formatter.Formatter
	FileFormatter    formatter.Formatter
	Console          bool
	ConsoleWriter    io.Writer
	File             bool
	Directory        string
	FilePrefix       string
	RotateByDay      bool
	MaxSize          int64
	QueueSize        int
	Caller           bool
	OnError          func(error)
}

// Logger accepts entries concurrently and writes them from one consumer.
type Logger struct {
	queue chan record.Entry
	done  chan struct{}
	state atomic.Uint64

	minLevel         record.Level
	formatter        formatter.Formatter
	consoleFormatter formatter.Formatter
	fileFormatter    formatter.Formatter
	console          io.Writer
	file             *logfile.File
	caller           bool
	onError          func(error)
	now              func() time.Time

	closeErr error
}

// New validates config and starts the logger's background consumer.
func New(config Config) (*Logger, error) {
	if !config.Level.Valid() {
		return nil, fmt.Errorf("logwriter: invalid minimum level %d", config.Level)
	}
	if !config.Console && !config.File {
		return nil, errors.New("logwriter: at least one output must be enabled")
	}
	if config.QueueSize < 0 {
		return nil, errors.New("logwriter: queue size cannot be negative")
	}
	if config.MaxSize < 0 {
		return nil, errors.New("logwriter: maximum file size cannot be negative")
	}
	if config.Formatter == nil {
		config.Formatter = formatter.TextFormatter{}
	}
	if config.QueueSize == 0 {
		config.QueueSize = DefaultQueueSize
	}

	var console io.Writer
	if config.Console {
		console = config.ConsoleWriter
		if console == nil {
			console = os.Stdout
		}
	}

	var file *logfile.File
	if config.File {
		if config.Directory == "" {
			config.Directory = "logs"
		}
		if config.FilePrefix == "" {
			config.FilePrefix = "app"
		}
		if config.FilePrefix == "." || config.FilePrefix == ".." || strings.ContainsAny(config.FilePrefix, `/\\`) {
			return nil, errors.New("logwriter: file prefix must be a file name, not a path")
		}
		file = logfile.NewFile(config.Directory, config.FilePrefix, config.RotateByDay, config.MaxSize)
	}

	logger := &Logger{
		queue:            make(chan record.Entry, config.QueueSize),
		done:             make(chan struct{}),
		minLevel:         config.Level,
		formatter:        config.Formatter,
		consoleFormatter: config.ConsoleFormatter,
		fileFormatter:    config.FileFormatter,
		console:          console,
		file:             file,
		caller:           config.Caller,
		onError:          config.OnError,
		now:              time.Now,
	}
	go logger.run()
	return logger, nil
}

// Debug queues a debug entry. It returns ErrClosed after Close has started.
func (l *Logger) Debug(message string, fields ...record.Field) error {
	return l.log(record.DebugLevel, message, fields, 2)
}

// Info queues an informational entry. It returns ErrClosed after Close has started.
func (l *Logger) Info(message string, fields ...record.Field) error {
	return l.log(record.InfoLevel, message, fields, 2)
}

// Warn queues a warning entry. It returns ErrClosed after Close has started.
func (l *Logger) Warn(message string, fields ...record.Field) error {
	return l.log(record.WarnLevel, message, fields, 2)
}

// Error queues an error entry. It returns ErrClosed after Close has started.
func (l *Logger) Error(message string, fields ...record.Field) error {
	return l.log(record.ErrorLevel, message, fields, 2)
}

// Log queues an entry at the supplied level.
func (l *Logger) Log(level record.Level, message string, fields ...record.Field) error {
	return l.log(level, message, fields, 2)
}

func (l *Logger) log(level record.Level, message string, fields []record.Field, callerSkip int) error {
	if !level.Valid() {
		return fmt.Errorf("logwriter: invalid entry level %d", level)
	}
	if level < l.minLevel {
		return nil
	}

	entry := record.Entry{
		Time:    l.now(),
		Level:   level,
		Message: message,
		Fields:  append([]record.Field(nil), fields...),
	}
	if l.caller {
		if _, file, line, ok := runtime.Caller(callerSkip); ok {
			entry.Caller = record.Caller{File: file, Line: line}
		}
	}

	if !l.acquire() {
		return ErrClosed
	}
	defer l.release()
	l.queue <- entry
	return nil
}

// Close stops accepting entries, drains the queue, closes the current file,
// and waits for the consumer to exit. It is safe to call more than once.
func (l *Logger) Close() error {
	for {
		state := l.state.Load()
		if state&closingFlag != 0 {
			break
		}
		if l.state.CompareAndSwap(state, state|closingFlag) {
			if state&activeMask == 0 {
				close(l.queue)
			}
			break
		}
	}
	<-l.done
	return l.closeErr
}

const (
	closingFlag = uint64(1) << 63
	activeMask  = closingFlag - 1
)

func (l *Logger) acquire() bool {
	for {
		state := l.state.Load()
		if state&closingFlag != 0 {
			return false
		}
		if state&activeMask == activeMask {
			return false
		}
		if l.state.CompareAndSwap(state, state+1) {
			return true
		}
	}
}

func (l *Logger) release() {
	state := l.state.Add(^uint64(0))
	if state&closingFlag != 0 && state&activeMask == 0 {
		close(l.queue)
	}
}

func (l *Logger) run() {
	defer func() {
		if l.file != nil {
			if err := l.file.Close(); err != nil {
				l.reportError(err)
			}
		}
		close(l.done)
	}()

	for entry := range l.queue {
		l.consume(entry)
	}
}

// consume writes one entry, converting a panic in the Formatter, the console
// writer, or OnError into a reported error so the consumer survives it.
func (l *Logger) consume(entry record.Entry) {
	defer func() {
		if recovered := recover(); recovered != nil {
			l.reportError(fmt.Errorf("logwriter: panic while processing entry %q: %v\n%s",
				entry.Message, recovered, debug.Stack()))
		}
	}()
	if err := l.write(entry); err != nil {
		l.reportError(err)
	}
}

func (l *Logger) write(entry record.Entry) error {
	if l.consoleFormatter == nil && l.fileFormatter == nil {
		data, err := l.formatter.Format(entry)
		if err != nil {
			return fmt.Errorf("format log entry: %w", err)
		}
		if len(data) == 0 {
			return nil
		}
		return l.writeShared(entry, data)
	}

	var errs []error
	if l.console != nil {
		outputFormatter := l.consoleFormatter
		if outputFormatter == nil {
			outputFormatter = l.formatter
		}
		data, err := outputFormatter.Format(entry)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("format console log entry: %w", err))
		case len(data) > 0:
			if err := writeAll(l.console, data); err != nil {
				errs = append(errs, fmt.Errorf("write console: %w", err))
			}
		}
	}
	if l.file != nil {
		outputFormatter := l.fileFormatter
		if outputFormatter == nil {
			outputFormatter = l.formatter
		}
		data, err := outputFormatter.Format(entry)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("format file log entry: %w", err))
		case len(data) > 0:
			if err := l.file.Write(entry.Time, data); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (l *Logger) writeShared(entry record.Entry, data []byte) error {
	var errs []error
	if l.console != nil {
		if err := writeAll(l.console, data); err != nil {
			errs = append(errs, fmt.Errorf("write console: %w", err))
		}
	}
	if l.file != nil {
		if err := l.file.Write(entry.Time, data); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func writeAll(destination io.Writer, data []byte) error {
	n, err := destination.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	return nil
}

// reportError records the first background error and forwards err to OnError.
// A panicking OnError cannot crash the consumer; its panic is joined into the
// error Close returns.
func (l *Logger) reportError(err error) {
	if l.closeErr == nil {
		l.closeErr = err
	}
	if l.onError == nil {
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			l.closeErr = errors.Join(l.closeErr,
				fmt.Errorf("logwriter: OnError panicked: %v", recovered))
		}
	}()
	l.onError(err)
}
