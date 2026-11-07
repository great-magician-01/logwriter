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
	// FileOnly reports whether the entry should be written to file outputs
	// only. Console outputs skip it. It is set by the FileOnly field marker,
	// which is removed from Fields before formatting.
	FileOnly bool
}

// fileOnlyMark is the sentinel Value of FileOnlyField. The unexported type
// prevents ordinary fields with a matching key from being mistaken for the
// marker.
type fileOnlyMark struct{}

const fileOnlyKey = "logwriter.file_only"

// FileOnlyField marks an entry as file-only when passed among the fields of a
// logging call. The logger strips it from the entry and skips console outputs
// for that entry.
var FileOnlyField = Field{Key: fileOnlyKey, Value: fileOnlyMark{}}

// StripFileOnly removes every FileOnlyField from fields and reports whether
// at least one was present. The returned slice shares fields when nothing is
// stripped.
func StripFileOnly(fields []Field) ([]Field, bool) {
	stripped := 0
	for _, field := range fields {
		if isFileOnly(field) {
			stripped++
		}
	}
	if stripped == 0 {
		return fields, false
	}
	rest := make([]Field, 0, len(fields)-stripped)
	for _, field := range fields {
		if !isFileOnly(field) {
			rest = append(rest, field)
		}
	}
	return rest, true
}

func isFileOnly(field Field) bool {
	if field.Key != fileOnlyKey {
		return false
	}
	_, ok := field.Value.(fileOnlyMark)
	return ok
}
