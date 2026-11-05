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
			log.Printf("logwriter background error: %v", err)
		},
	})
	if err != nil {
		log.Fatalf("create logger: %v", err)
	}

	if err := logger.Info(
		"user logged in",
		logwriter.Field{Key: "user_id", Value: 10086},
		logwriter.Field{Key: "admin", Value: true},
	); err != nil {
		_ = logger.Close()
		log.Fatalf("queue log entry: %v", err)
	}

	if err := logger.Close(); err != nil {
		log.Fatalf("close logger: %v", err)
	}
}
