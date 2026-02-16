package lifecycle

import (
	"fmt"
	"os"
	"sync"
)

const (
	// defaultMaxLogBytes is the default maximum size of a log file before rotation (10 MB).
	defaultMaxLogBytes int64 = 10 * 1024 * 1024
)

// cappedFileWriter wraps an os.File and rotates it when the written bytes
// exceed maxBytes. It keeps one rotated copy (e.g. agent.log.1).
// All methods are safe for concurrent use.
type cappedFileWriter struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	written  int64
	maxBytes int64
}

// newCappedFileWriter opens (or creates) the file at path in append mode and
// returns a writer that will rotate the file once maxBytes is exceeded.
// If maxBytes <= 0, defaultMaxLogBytes is used.
func newCappedFileWriter(path string, maxBytes int64) (*cappedFileWriter, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxLogBytes
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}

	// Determine current file size so we account for existing content.
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	return &cappedFileWriter{
		file:     f,
		path:     path,
		written:  info.Size(),
		maxBytes: maxBytes,
	}, nil
}

// Write implements io.Writer. It writes p to the underlying file and rotates
// if the cumulative size exceeds the cap.
func (w *cappedFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	n, err := w.file.Write(p)
	w.written += int64(n)

	if err == nil && w.written >= w.maxBytes {
		w.rotate()
	}

	return n, err
}

// Close closes the underlying file.
func (w *cappedFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

// rotate moves the current log to path.1 (removing any older rotated copy)
// and opens a fresh file. Errors during rotation are best-effort; the writer
// continues with the existing file if rotation fails.
func (w *cappedFileWriter) rotate() {
	w.file.Close()

	rotated := fmt.Sprintf("%s.1", w.path)
	_ = os.Remove(rotated)
	_ = os.Rename(w.path, rotated)

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		f, _ = os.OpenFile(rotated, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	}
	w.file = f
	w.written = 0
}
