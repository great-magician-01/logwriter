package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/great-magician-01/logwriter"
)

type jsonFormatter struct{}

func (jsonFormatter) Format(entry logwriter.Entry) ([]byte, error) {
	fields := make(map[string]any, len(entry.Fields))
	for _, field := range entry.Fields {
		fields[field.Key] = field.Value
	}

	record := struct {
		Time    string         `json:"time"`
		Level   string         `json:"level"`
		Message string         `json:"message"`
		Fields  map[string]any `json:"fields,omitempty"`
		Caller  string         `json:"caller,omitempty"`
	}{
		Time:    entry.Time.Format(time.RFC3339Nano),
		Level:   entry.Level.String(),
		Message: entry.Message,
		Fields:  fields,
	}
	if entry.Caller.File != "" {
		record.Caller = fmt.Sprintf("%s:%d", entry.Caller.File, entry.Caller.Line)
	}

	data, err := json.Marshal(record)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func main() {
	logger, err := logwriter.New(logwriter.Config{
		Formatter: jsonFormatter{},
		Console:   true,
		Caller:    true,
	})
	if err != nil {
		log.Fatalf("create logger: %v", err)
	}

	if err := logger.Info(
		"request completed",
		logwriter.Field{Key: "method", Value: "GET"},
		logwriter.Field{Key: "path", Value: "/users/10086"},
		logwriter.Field{Key: "status", Value: 200},
	); err != nil {
		_ = logger.Close()
		log.Fatalf("queue log entry: %v", err)
	}

	if err := logger.Close(); err != nil {
		log.Fatalf("close logger: %v", err)
	}
}
