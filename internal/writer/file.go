package writer

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	dayNameLayout = "20060102"
	dayDirLayout  = "2006-01-02"
)

// Mode selects where the local date appears in the file layout.
type Mode uint8

const (
	// ModeFlat writes every file directly into the configured directory.
	ModeFlat Mode = iota
	// ModeDayName groups files in a local-date directory and names them
	// after the same compact date: directory/20060102/20060102.log.
	ModeDayName
	// ModeDayDir groups files in a local-date directory and keeps the plain
	// file name: directory/2006-01-02/prefix.log.
	ModeDayDir
)

// File writes log bytes to rotating files. It is not safe for concurrent use;
// the core package restricts it to the single queue consumer.
type File struct {
	directory string
	prefix    string
	mode      Mode
	maxSize   int64

	file  *os.File
	day   string
	index int
	size  int64
}

// NewFile creates a lazily opened rotating file writer.
func NewFile(directory, prefix string, mode Mode, maxSize int64) *File {
	return &File{
		directory: directory,
		prefix:    prefix,
		mode:      mode,
		maxSize:   maxSize,
	}
}

// Write writes one complete formatted entry, rotating first when necessary.
func (w *File) Write(t time.Time, p []byte) error {
	if len(p) == 0 {
		return nil
	}

	day := w.dayString(t)
	switch {
	case w.file == nil:
		if err := w.openLatest(day); err != nil {
			return err
		}
	case w.mode != ModeFlat && day != w.day:
		if err := w.reopenLatest(day); err != nil {
			return err
		}
	case w.maxSize > 0 && w.size > 0 && w.size+int64(len(p)) > w.maxSize:
		if err := w.openNext(); err != nil {
			return err
		}
	}

	n, err := w.file.Write(p)
	w.size += int64(n)
	if err != nil {
		return fmt.Errorf("write log file %q: %w", w.file.Name(), err)
	}
	if n != len(p) {
		return fmt.Errorf("write log file %q: %w", w.file.Name(), io.ErrShortWrite)
	}
	return nil
}

// Close closes the current file. It is safe to call before the first write.
func (w *File) Close() error {
	if w.file == nil {
		return nil
	}
	file := w.file
	w.file = nil
	if err := file.Close(); err != nil {
		return fmt.Errorf("close log file %q: %w", file.Name(), err)
	}
	return nil
}

func (w *File) reopenLatest(day string) error {
	if err := w.Close(); err != nil {
		return err
	}
	return w.openLatest(day)
}

func (w *File) openLatest(day string) error {
	directory := w.directoryFor(day)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create log directory %q: %w", directory, err)
	}

	index := 0
	if w.maxSize > 0 {
		latest, size, err := w.latestFile(directory, day)
		if err != nil {
			return err
		}
		index = latest
		if index == 0 {
			index = 1
		} else if size >= w.maxSize {
			if index == int(^uint(0)>>1) {
				return fmt.Errorf("log file index overflow for %q", w.fileName(day, index))
			}
			index++
		}
	}

	if err := w.open(directory, w.fileName(day, index)); err != nil {
		return err
	}
	w.day = day
	w.index = index
	return nil
}

func (w *File) openNext() error {
	if err := w.Close(); err != nil {
		return err
	}
	if w.index == int(^uint(0)>>1) {
		return fmt.Errorf("log file index overflow for %q", w.prefix)
	}

	directory := w.directoryFor(w.day)
	index := w.index + 1
	if err := w.open(directory, w.fileName(w.day, index)); err != nil {
		return err
	}
	w.index = index
	return nil
}

func (w *File) open(directory, name string) error {
	path := filepath.Join(directory, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open log file %q: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("stat log file %q: %w", path, err)
	}
	w.file = file
	w.size = info.Size()
	return nil
}

func (w *File) latestFile(directory, day string) (index int, size int64, err error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, 0, fmt.Errorf("read log directory %q: %w", directory, err)
	}

	prefix := w.baseName(day) + "_"
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".log") {
			continue
		}
		number := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ".log")
		candidate, parseErr := strconv.ParseUint(number, 10, 63)
		if parseErr != nil || candidate == 0 || candidate > uint64(^uint(0)>>1) {
			continue
		}
		if int(candidate) <= index {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return 0, 0, fmt.Errorf("stat log file %q: %w", filepath.Join(directory, name), infoErr)
		}
		index = int(candidate)
		size = info.Size()
	}
	return index, size, nil
}

func (w *File) fileName(day string, index int) string {
	name := w.baseName(day)
	if w.maxSize > 0 {
		name += "_" + strconv.Itoa(index)
	}
	return name + ".log"
}

// dayString formats the local date used by the layout, or "" for ModeFlat.
func (w *File) dayString(t time.Time) string {
	switch w.mode {
	case ModeDayName:
		return t.Format(dayNameLayout)
	case ModeDayDir:
		return t.Format(dayDirLayout)
	default:
		return ""
	}
}

func (w *File) directoryFor(day string) string {
	if w.mode != ModeFlat {
		return filepath.Join(w.directory, day)
	}
	return w.directory
}

func (w *File) baseName(day string) string {
	if w.mode == ModeDayName {
		return day
	}
	return w.prefix
}
