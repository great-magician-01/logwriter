package core

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/great-magician-01/logwriter/internal/formatter"
	"github.com/great-magician-01/logwriter/internal/record"
)

func TestQueuePreservesOrderAndCloseDrains(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(Config{
		Console:       true,
		ConsoleWriter: &output,
		Formatter:     messageFormatter(),
		QueueSize:     8,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	for i := 0; i < 100; i++ {
		if err := logger.Info(fmt.Sprintf("entry-%03d", i)); err != nil {
			t.Fatalf("Info() error = %v", err)
		}
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}
	if err := logger.Info("after close"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Info() after Close error = %v, want ErrClosed", err)
	}

	lines := strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")
	if len(lines) != 100 {
		t.Fatalf("output has %d lines, want 100", len(lines))
	}
	for i, line := range lines {
		if want := fmt.Sprintf("entry-%03d", i); line != want {
			t.Fatalf("line %d = %q, want %q", i, line, want)
		}
	}
}

func TestMinimumLevel(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(Config{
		Level:         record.WarnLevel,
		Console:       true,
		ConsoleWriter: &output,
		Formatter:     messageFormatter(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = logger.Debug("debug")
	_ = logger.Info("info")
	_ = logger.Warn("warn")
	_ = logger.Error("error")
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if want := "warn\nerror\n"; output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestFieldsAreCopiedBeforeQueueing(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(Config{
		Console:       true,
		ConsoleWriter: &output,
		Formatter: formatter.FormatterFunc(func(entry record.Entry) ([]byte, error) {
			return []byte(fmt.Sprint(entry.Fields[0].Value) + "\n"), nil
		}),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	fields := []record.Field{{Key: "state", Value: "before"}}
	if err := logger.Info("message", fields...); err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	fields[0].Value = "after"
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if want := "before\n"; output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestConcurrentLoggingAndClose(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(Config{
		Console:       true,
		ConsoleWriter: &output,
		Formatter:     messageFormatter(),
		QueueSize:     4,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	const (
		producers          = 8
		entriesPerProducer = 50
	)
	var accepted atomic.Int64
	var ready sync.WaitGroup
	var producersDone sync.WaitGroup
	ready.Add(producers)
	producersDone.Add(producers)
	start := make(chan struct{})
	for producer := 0; producer < producers; producer++ {
		go func(producer int) {
			defer producersDone.Done()
			ready.Done()
			<-start
			for i := 0; i < entriesPerProducer; i++ {
				err := logger.Info(fmt.Sprintf("%d-%d", producer, i))
				switch {
				case err == nil:
					accepted.Add(1)
				case errors.Is(err, ErrClosed):
				default:
					t.Errorf("Info() error = %v", err)
				}
			}
		}(producer)
	}
	ready.Wait()
	close(start)
	closeResult := make(chan error, 1)
	go func() { closeResult <- logger.Close() }()
	producersDone.Wait()
	if err := <-closeResult; err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if got, want := strings.Count(output.String(), "\n"), int(accepted.Load()); got != want {
		t.Fatalf("output has %d entries, want %d accepted entries", got, want)
	}
}

func TestCloseWaitsForBlockedProducer(t *testing.T) {
	output := &blockingWriter{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	logger, err := New(Config{
		Console:       true,
		ConsoleWriter: output,
		Formatter:     messageFormatter(),
		QueueSize:     1,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := logger.Info("one"); err != nil {
		t.Fatalf("Info(one) error = %v", err)
	}
	waitForSignal(t, output.started, "first console write")
	if err := logger.Info("two"); err != nil {
		t.Fatalf("Info(two) error = %v", err)
	}

	thirdResult := make(chan error, 1)
	go func() { thirdResult <- logger.Info("three") }()
	waitForState(t, logger, activeMask, "blocked producer")
	closeResult := make(chan error, 1)
	go func() { closeResult <- logger.Close() }()
	waitForState(t, logger, closingFlag, "close to start")

	select {
	case err := <-closeResult:
		t.Fatalf("Close() returned before the queue was released: %v", err)
	default:
	}
	close(output.release)
	if err := <-thirdResult; err != nil {
		t.Fatalf("blocked Info() error = %v", err)
	}
	if err := <-closeResult; err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if want := "one\ntwo\nthree\n"; output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestFormatterErrorIsReportedAndConsumerContinues(t *testing.T) {
	formatErr := errors.New("format failed")
	var output bytes.Buffer
	var reported []error
	logger, err := New(Config{
		Console:       true,
		ConsoleWriter: &output,
		Formatter: formatter.FormatterFunc(func(entry record.Entry) ([]byte, error) {
			if entry.Message == "bad" {
				return nil, formatErr
			}
			return []byte(entry.Message + "\n"), nil
		}),
		OnError: func(err error) { reported = append(reported, err) },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = logger.Info("bad")
	_ = logger.Info("good")
	closeErr := logger.Close()
	if !errors.Is(closeErr, formatErr) {
		t.Fatalf("Close() error = %v, want format error", closeErr)
	}
	if len(reported) != 1 || !errors.Is(reported[0], formatErr) {
		t.Fatalf("OnError received %v, want one format error", reported)
	}
	if want := "good\n"; output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestFormatterPanicIsReportedAndConsumerContinues(t *testing.T) {
	var output bytes.Buffer
	var reported []error
	logger, err := New(Config{
		Console:       true,
		ConsoleWriter: &output,
		Formatter: formatter.FormatterFunc(func(entry record.Entry) ([]byte, error) {
			if entry.Message == "boom" {
				panic("formatter exploded")
			}
			return []byte(entry.Message + "\n"), nil
		}),
		OnError: func(err error) { reported = append(reported, err) },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = logger.Info("boom")
	_ = logger.Info("good")
	closeErr := logger.Close()
	if closeErr == nil || !strings.Contains(closeErr.Error(), "formatter exploded") {
		t.Fatalf("Close() error = %v, want it to contain the panic value", closeErr)
	}
	if len(reported) != 1 || !strings.Contains(reported[0].Error(), "formatter exploded") {
		t.Fatalf("OnError received %v, want one panic error", reported)
	}
	if !strings.Contains(reported[0].Error(), "logger_test.go") {
		t.Fatalf("panic error = %v, want a stack trace pointing at the panic site", reported[0])
	}
	if want := "good\n"; output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestPanickingOnErrorDoesNotCrashConsumer(t *testing.T) {
	formatErr := errors.New("format failed")
	var output bytes.Buffer
	logger, err := New(Config{
		Console:       true,
		ConsoleWriter: &output,
		Formatter: formatter.FormatterFunc(func(entry record.Entry) ([]byte, error) {
			if entry.Message == "bad" {
				return nil, formatErr
			}
			return []byte(entry.Message + "\n"), nil
		}),
		OnError: func(err error) { panic("OnError exploded") },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = logger.Info("bad")
	_ = logger.Info("good")
	closeErr := logger.Close()
	if !errors.Is(closeErr, formatErr) {
		t.Fatalf("Close() error = %v, want format error", closeErr)
	}
	if !strings.Contains(closeErr.Error(), "OnError exploded") {
		t.Fatalf("Close() error = %v, want it to contain the OnError panic", closeErr)
	}
	if want := "good\n"; output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

// TestFormatterNilPanicIsReported pins the Go 1.21+ semantics where a literal
// panic(nil) reaches recover as *runtime.PanicNilError; lowering the go
// directive in go.mod would make this test fail.
func TestFormatterNilPanicIsReported(t *testing.T) {
	var output bytes.Buffer
	var reported []error
	logger, err := New(Config{
		Console:       true,
		ConsoleWriter: &output,
		Formatter: formatter.FormatterFunc(func(entry record.Entry) ([]byte, error) {
			if entry.Message == "nil panic" {
				panic(nil)
			}
			return []byte(entry.Message + "\n"), nil
		}),
		OnError: func(err error) { reported = append(reported, err) },
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = logger.Info("nil panic")
	_ = logger.Info("good")
	closeErr := logger.Close()
	if len(reported) != 1 || !strings.Contains(reported[0].Error(), "panic called with nil argument") {
		t.Fatalf("OnError received %v, want the runtime nil-panic error", reported)
	}
	if closeErr == nil || !strings.Contains(closeErr.Error(), "panic called with nil argument") {
		t.Fatalf("Close() error = %v, want it to contain the nil-panic error", closeErr)
	}
	if want := "good\n"; output.String() != want {
		t.Fatalf("output = %q, want %q", output.String(), want)
	}
}

func TestConsoleFailureDoesNotSkipFile(t *testing.T) {
	writeErr := errors.New("console failed")
	directory := t.TempDir()
	logger, err := New(Config{
		Console:       true,
		ConsoleWriter: errorWriter{err: writeErr},
		File:          true,
		Directory:     directory,
		Formatter:     messageFormatter(),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = logger.Info("persisted")
	if err := logger.Close(); !errors.Is(err, writeErr) {
		t.Fatalf("Close() error = %v, want console error", err)
	}
	data, err := os.ReadFile(filepath.Join(directory, "app.log"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if want := "persisted\n"; string(data) != want {
		t.Fatalf("file output = %q, want %q", data, want)
	}
}

func TestSharedFormatterRunsOnceForBothOutputs(t *testing.T) {
	directory := t.TempDir()
	var console bytes.Buffer
	formatCalls := 0
	logger, err := New(Config{
		Console:       true,
		ConsoleWriter: &console,
		File:          true,
		Directory:     directory,
		Formatter: formatter.FormatterFunc(func(entry record.Entry) ([]byte, error) {
			formatCalls++
			return []byte(entry.Message + "\n"), nil
		}),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = logger.Info("shared")
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if formatCalls != 1 {
		t.Fatalf("Formatter called %d times, want 1", formatCalls)
	}
	if want := "shared\n"; console.String() != want {
		t.Fatalf("console output = %q, want %q", console.String(), want)
	}
	data, err := os.ReadFile(filepath.Join(directory, "app.log"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if want := "shared\n"; string(data) != want {
		t.Fatalf("file output = %q, want %q", data, want)
	}
}

func TestOutputSpecificFormatters(t *testing.T) {
	directory := t.TempDir()
	var console bytes.Buffer
	sharedCalls := 0
	logger, err := New(Config{
		Console:       true,
		ConsoleWriter: &console,
		File:          true,
		Directory:     directory,
		Formatter: formatter.FormatterFunc(func(entry record.Entry) ([]byte, error) {
			sharedCalls++
			return []byte("shared " + entry.Message + "\n"), nil
		}),
		ConsoleFormatter: formatter.FormatterFunc(func(entry record.Entry) ([]byte, error) {
			return []byte("console " + entry.Message + "\n"), nil
		}),
		FileFormatter: formatter.FormatterFunc(func(entry record.Entry) ([]byte, error) {
			return []byte("file " + entry.Message + "\n"), nil
		}),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = logger.Info("entry")
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if sharedCalls != 0 {
		t.Fatalf("shared Formatter called %d times, want 0", sharedCalls)
	}
	if want := "console entry\n"; console.String() != want {
		t.Fatalf("console output = %q, want %q", console.String(), want)
	}
	data, err := os.ReadFile(filepath.Join(directory, "app.log"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if want := "file entry\n"; string(data) != want {
		t.Fatalf("file output = %q, want %q", data, want)
	}
}

func TestConsoleFormatterErrorDoesNotSkipFile(t *testing.T) {
	formatErr := errors.New("console format failed")
	directory := t.TempDir()
	var console bytes.Buffer
	logger, err := New(Config{
		Console:       true,
		ConsoleWriter: &console,
		File:          true,
		Directory:     directory,
		Formatter:     messageFormatter(),
		ConsoleFormatter: formatter.FormatterFunc(func(record.Entry) ([]byte, error) {
			return nil, formatErr
		}),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = logger.Info("persisted")
	if err := logger.Close(); !errors.Is(err, formatErr) {
		t.Fatalf("Close() error = %v, want console format error", err)
	}
	if console.Len() != 0 {
		t.Fatalf("console output = %q, want empty", console.String())
	}
	data, err := os.ReadFile(filepath.Join(directory, "app.log"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if want := "persisted\n"; string(data) != want {
		t.Fatalf("file output = %q, want %q", data, want)
	}
}

func TestCallerIsCapturedAtLoggingCall(t *testing.T) {
	var caller record.Caller
	logger, err := New(Config{
		Console:       true,
		ConsoleWriter: &bytes.Buffer{},
		Caller:        true,
		Formatter: formatter.FormatterFunc(func(entry record.Entry) ([]byte, error) {
			caller = entry.Caller
			return []byte(entry.Message + "\n"), nil
		}),
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_ = logger.Info("caller")
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !strings.HasSuffix(caller.File, "logger_test.go") || caller.Line == 0 {
		t.Fatalf("caller = %+v, want logger_test.go with a line", caller)
	}
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{name: "no output", config: Config{}},
		{name: "invalid level", config: Config{Console: true, Level: record.Level(99)}},
		{name: "negative queue", config: Config{Console: true, QueueSize: -1}},
		{name: "negative size", config: Config{File: true, MaxSize: -1}},
		{name: "path as prefix", config: Config{File: true, FilePrefix: `dir/app`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logger, err := New(test.config)
			if err == nil {
				if logger != nil {
					_ = logger.Close()
				}
				t.Fatal("New() succeeded, want validation error")
			}
		})
	}
}

type blockingWriter struct {
	bytes.Buffer
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	w.once.Do(func() {
		close(w.started)
		<-w.release
	})
	return w.Buffer.Write(p)
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func messageFormatter() formatter.Formatter {
	return formatter.FormatterFunc(func(entry record.Entry) ([]byte, error) {
		return []byte(entry.Message + "\n"), nil
	})
}

func waitForSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", description)
	}
}

func waitForState(t *testing.T, logger *Logger, mask uint64, description string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for logger.state.Load()&mask == 0 {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", description)
		}
		runtime.Gosched()
	}
}
