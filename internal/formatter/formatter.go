package formatter

import (
	"fmt"
	"strings"

	"github.com/great-magician-01/logwriter/internal/record"
)

const DefaultTimeFormat = "2006-01-02 15:04:05.000"

// Formatter converts an Entry into the bytes written to each enabled output.
// Implementations should include a trailing newline when one is desired.
type Formatter interface {
	Format(record.Entry) ([]byte, error)
}

// FormatterFunc adapts a function to the Formatter interface.
type FormatterFunc func(record.Entry) ([]byte, error)

// Format calls f(entry).
func (f FormatterFunc) Format(entry record.Entry) ([]byte, error) {
	return f(entry)
}

// TextFormatter writes entries as text. TimeFormat uses Go's time layout and
// defaults to DefaultTimeFormat.
type TextFormatter struct {
	TimeFormat string
}

// Format formats an entry as timestamp, level, optional caller, message, and
// structured fields, followed by a newline.
func (f TextFormatter) Format(entry record.Entry) ([]byte, error) {
	layout := f.TimeFormat
	if layout == "" {
		layout = DefaultTimeFormat
	}

	var b strings.Builder
	b.WriteString(entry.Time.Format(layout))
	b.WriteString(" [")
	b.WriteString(entry.Level.String())
	b.WriteByte(']')
	if entry.Caller.File != "" {
		fmt.Fprintf(&b, " [%s:%d]", baseName(entry.Caller.File), entry.Caller.Line)
	}
	b.WriteByte(' ')
	b.WriteString(entry.Message)
	for _, field := range entry.Fields {
		b.WriteByte(' ')
		b.WriteString(field.Key)
		b.WriteByte('=')
		fmt.Fprint(&b, field.Value)
	}
	b.WriteByte('\n')
	return []byte(b.String()), nil
}

// baseName returns everything after the last path separator, treating both
// '/' and '\' as separators. runtime.Caller yields native paths ('\' on
// Windows, '/' elsewhere), and filepath.Base only handles the platform's own
// separator, so paths from another platform would otherwise be kept whole.
func baseName(file string) string {
	if i := strings.LastIndexAny(file, `/\`); i >= 0 {
		return file[i+1:]
	}
	return file
}
