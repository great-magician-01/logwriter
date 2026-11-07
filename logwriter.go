// Package logwriter provides queued console and rotating-file logging.
package logwriter

import (
	"github.com/great-magician-01/logwriter/internal/core"
	"github.com/great-magician-01/logwriter/internal/formatter"
	"github.com/great-magician-01/logwriter/internal/record"
)

type (
	// Logger accepts entries concurrently and writes them through a queue.
	Logger = core.Logger
	// Config controls logging, outputs, queuing, and file rotation.
	Config = core.Config
	// Entry is the log record supplied to a Formatter.
	Entry = record.Entry
	// Level is the severity of a log entry.
	Level = record.Level
	// Field is a structured value attached to an entry.
	Field = record.Field
	// Caller identifies the source location of an entry.
	Caller = record.Caller
	// Formatter converts an Entry into output bytes.
	Formatter = formatter.Formatter
	// FormatterFunc adapts a function to Formatter.
	FormatterFunc = formatter.FormatterFunc
	// TextFormatter is the built-in text formatter with optional level colors.
	TextFormatter = formatter.TextFormatter
)

const (
	DebugLevel = record.DebugLevel
	InfoLevel  = record.InfoLevel
	WarnLevel  = record.WarnLevel
	ErrorLevel = record.ErrorLevel

	DefaultTimeFormat = formatter.DefaultTimeFormat
	DefaultQueueSize  = core.DefaultQueueSize
)

var ErrClosed = core.ErrClosed

// FileOnly returns a Field that marks an entry as file-only: enabled console
// outputs skip the entry and file outputs write it. The marker is removed
// from the entry's fields before formatting; custom formatters can inspect
// Entry.FileOnly instead.
//
//	logger.Info("token refreshed", logwriter.FileOnly())
func FileOnly() Field {
	return record.FileOnlyField
}

// New creates a Logger and starts its single background consumer.
func New(config Config) (*Logger, error) {
	return core.New(config)
}
