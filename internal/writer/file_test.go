package writer

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRotationByDayAndSize(t *testing.T) {
	directory := t.TempDir()
	writer := NewFile(directory, "app", true, 10)
	dayOne := time.Date(2026, 11, 4, 23, 59, 59, 0, time.Local)
	dayTwo := dayOne.Add(24 * time.Hour)

	for _, write := range []struct {
		time time.Time
		data string
	}{
		{dayOne, "aaaa\n"},
		{dayOne, "bbbb\n"},
		{dayOne, "cccc\n"},
		{dayTwo, "dddd\n"},
	} {
		if err := writer.Write(write.time, []byte(write.data)); err != nil {
			t.Fatalf("Write(%q) error = %v", write.data, err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	assertFileContent(t, filepath.Join(directory, "20261104", "20261104_1.log"), "aaaa\nbbbb\n")
	assertFileContent(t, filepath.Join(directory, "20261104", "20261104_2.log"), "cccc\n")
	assertFileContent(t, filepath.Join(directory, "20261105", "20261105_1.log"), "dddd\n")
}

func TestRotationBySizeOnly(t *testing.T) {
	directory := t.TempDir()
	writer := NewFile(directory, "server", false, 5)
	if err := writer.Write(time.Now(), []byte("12345")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Write(time.Now(), []byte("6")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	assertFileContent(t, filepath.Join(directory, "server_1.log"), "12345")
	assertFileContent(t, filepath.Join(directory, "server_2.log"), "6")
}

func TestSingleFileWithoutRotation(t *testing.T) {
	directory := t.TempDir()
	writer := NewFile(directory, "app", false, 0)
	if err := writer.Write(time.Now(), []byte("one")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Write(time.Now().Add(48*time.Hour), []byte("two")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	assertFileContent(t, filepath.Join(directory, "app.log"), "onetwo")
}

func TestRestartAppendsAndAdvancesLatestIndex(t *testing.T) {
	directory := t.TempDir()
	at := time.Date(2026, 11, 4, 12, 0, 0, 0, time.Local)

	for _, data := range []string{"aaaa\n", "bbbb\n", "cccc\n", "dddd\n"} {
		writer := NewFile(directory, "app", true, 10)
		if err := writer.Write(at, []byte(data)); err != nil {
			t.Fatalf("Write(%q) error = %v", data, err)
		}
		if err := writer.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}

	assertFileContent(t, filepath.Join(directory, "20261104", "20261104_1.log"), "aaaa\nbbbb\n")
	assertFileContent(t, filepath.Join(directory, "20261104", "20261104_2.log"), "cccc\ndddd\n")
}

func TestOversizedEntryIsNotSplit(t *testing.T) {
	directory := t.TempDir()
	writer := NewFile(directory, "app", false, 5)
	if err := writer.Write(time.Now(), []byte("123456789")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Write(time.Now(), []byte("x")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	assertFileContent(t, filepath.Join(directory, "app_1.log"), "123456789")
	assertFileContent(t, filepath.Join(directory, "app_2.log"), "x")
}

func TestRestartUsesHighestNumericIndex(t *testing.T) {
	directory := t.TempDir()
	for name, content := range map[string]string{
		"app_2.log":       "two",
		"app_10.log":      "ten",
		"app_bad.log":     "ignored",
		"other_20.log":    "ignored",
		"app_100.log.txt": "ignored",
	} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile(%q) error = %v", name, err)
		}
	}

	writer := NewFile(directory, "app", false, 100)
	if err := writer.Write(time.Now(), []byte("+")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	assertFileContent(t, filepath.Join(directory, "app_10.log"), "ten+")
	if _, err := os.Stat(filepath.Join(directory, "app_11.log")); !os.IsNotExist(err) {
		t.Fatalf("app_11.log should not exist, Stat() error = %v", err)
	}
}

func TestCloseWithoutWriteCreatesNoFiles(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "logs")
	writer := NewFile(directory, "app", true, 10)
	if err := writer.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("log directory should not exist, Stat() error = %v", err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("ReadFile(%q) = %q, want %q", path, data, want)
	}
}
