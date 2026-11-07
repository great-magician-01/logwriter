package logwriter_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestFileOnlyWritesFileNotConsole(t *testing.T) {
	directory := t.TempDir()
	var console bytes.Buffer
	logger, err := logwriter.New(logwriter.Config{
		Console:       true,
		ConsoleWriter: &console,
		File:          true,
		Directory:     directory,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := logger.Info("on console"); err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if err := logger.Info("in file only", logwriter.FileOnly()); err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if console.Len() == 0 || strings.Count(console.String(), "\n") != 1 || !strings.Contains(console.String(), "on console") {
		t.Fatalf("console output = %q, want only the \"on console\" entry", console.String())
	}
	if strings.Contains(console.String(), "in file only") {
		t.Fatalf("console output = %q, want no file-only entry", console.String())
	}
	data, err := os.ReadFile(filepath.Join(directory, "app.log"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "in file only\n") {
		t.Fatalf("file output = %q, want the file-only entry", data)
	}
	if strings.Contains(string(data), "logwriter.file_only") {
		t.Fatalf("file output = %q, want the marker field stripped", data)
	}
}

func TestDayDirectoryKeepsPlainFileName(t *testing.T) {
	directory := t.TempDir()
	day := time.Now().Format("2006-01-02")
	logger, err := logwriter.New(logwriter.Config{
		File:           true,
		Directory:      directory,
		DirectoryByDay: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := logger.Info("plain file name in day directory"); err != nil {
		t.Fatalf("Info() error = %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(directory, day, "app.log"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !strings.Contains(string(data), "plain file name in day directory") {
		t.Fatalf("file output = %q, want the entry", data)
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
