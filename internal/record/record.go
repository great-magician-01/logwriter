package record

import (
	"fmt"
	"time"
)

// Level is the severity of a log entry.
type Level uint8

const (
	DebugLevel Level = iota
	InfoLevel
	WarnLevel
	ErrorLevel
)

// String returns the upper-case name of the level.
func (l Level) String() string {
	switch l {
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	default:
		return fmt.Sprintf("LEVEL(%d)", uint8(l))
	}
}

// Valid reports whether l is a defined log level.
func (l Level) Valid() bool {
	return l <= ErrorLevel
}

// Field is a structured value attached to a log entry.
type Field struct {
	Key   string
	Value any
}

// Caller identifies the source location that produced a log entry.
type Caller struct {
	File string
	Line int
}

// Entry is the log record passed to a formatter.
type Entry struct {
	Time    time.Time
	Level   Level
	Message string
	Fields  []Field
	Caller  Caller
}
