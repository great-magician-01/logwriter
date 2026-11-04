package formatter

import (
	"testing"
	"time"

	"github.com/great-magician-01/logwriter/internal/record"
)

func TestTextFormatter(t *testing.T) {
	entry := record.Entry{
		Time:    time.Date(2026, 11, 4, 15, 30, 20, 123456789, time.UTC),
		Level:   record.InfoLevel,
		Message: "user logged in",
		Fields: []record.Field{
			{Key: "user_id", Value: 10086},
			{Key: "admin", Value: true},
		},
		Caller: record.Caller{File: `D:\project\main.go`, Line: 42},
	}

	data, err := (TextFormatter{}).Format(entry)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	want := "2026-11-04 15:30:20.123 [INFO] [main.go:42] user logged in user_id=10086 admin=true\n"
	if string(data) != want {
		t.Fatalf("Format() = %q, want %q", data, want)
	}
}

func TestTextFormatterCustomTimeLayout(t *testing.T) {
	entry := record.Entry{
		Time:    time.Date(2026, 11, 4, 15, 30, 20, 0, time.UTC),
		Level:   record.WarnLevel,
		Message: "warning",
	}
	data, err := (TextFormatter{TimeFormat: "2006/01/02"}).Format(entry)
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	if want := "2026/11/04 [WARN] warning\n"; string(data) != want {
		t.Fatalf("Format() = %q, want %q", data, want)
	}
}

func TestFormatterFunc(t *testing.T) {
	formatter := FormatterFunc(func(entry record.Entry) ([]byte, error) {
		return []byte(entry.Message), nil
	})
	data, err := formatter.Format(record.Entry{Message: "custom"})
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	if string(data) != "custom" {
		t.Fatalf("Format() = %q, want custom", data)
	}
}
