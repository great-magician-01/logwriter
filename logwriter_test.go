package logwriter_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/great-magician-01/logwriter"
)

func TestPublicAPIWithCustomFormatter(t *testing.T) {
	directory := t.TempDir()
	var console bytes.Buffer
	logger, err := logwriter.New(logwriter.Config{
		Formatter: logwriter.FormatterFunc(func(entry logwriter.Entry) ([]byte, error) {
			line := fmt.Sprintf("%s|%s", entry.Level, entry.Message)
			for _, field := range entry.Fields {
				line += fmt.Sprintf("|%s=%v", field.Key, field.Value)
			}
			return []byte(line + "\n"), nil
		}),
		Console:       true,
		ConsoleWriter: &console,
		File:          true,
		Directory:     directory,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := logger.Info("started", logwriter.Field{Key: "port", Value: 8080}); err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	const want = "INFO|started|port=8080\n"
	if console.String() != want {
		t.Fatalf("console output = %q, want %q", console.String(), want)
	}
	data, err := os.ReadFile(filepath.Join(directory, "app.log"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != want {
		t.Fatalf("file output = %q, want %q", data, want)
	}
}

func TestConsoleColorWithPlainFile(t *testing.T) {
	directory := t.TempDir()
	var console bytes.Buffer
	logger, err := logwriter.New(logwriter.Config{
		Console:          true,
		ConsoleWriter:    &console,
		ConsoleFormatter: logwriter.TextFormatter{Color: true},
		File:             true,
		Directory:        directory,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := logger.Info("colored console, plain file"); err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(directory, "app.log"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if strings.Contains(string(data), "\x1b[") {
		t.Fatalf("file output contains ANSI escapes: %q", data)
	}
	if !strings.Contains(string(data), "[INFO] colored console, plain file\n") {
		t.Fatalf("file output = %q, want plain INFO entry", data)
	}
	wantConsole := strings.Replace(string(data), "[INFO]", "[\x1b[32mINFO\x1b[0m]", 1)
	if console.String() != wantConsole {
		t.Fatalf("console output = %q, want %q", console.String(), wantConsole)
	}
}

func Example() {
	logger, _ := logwriter.New(logwriter.Config{
		Console: true,
		Formatter: logwriter.FormatterFunc(func(entry logwriter.Entry) ([]byte, error) {
			return []byte(entry.Level.String() + " " + entry.Message + "\n"), nil
		}),
	})
	_ = logger.Info("service started")
	_ = logger.Close()

	// Output: INFO service started
}
