// Package audit implements file and HTTP audit observers.
package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	auditapp "github.com/76Parker/metrico/internal/applications/audit"
)

// File appends complete JSON lines to one audit file.
type File struct {
	mu   sync.Mutex
	file *os.File
}

// NewFile opens the audit file for append, creating it if necessary.
func NewFile(path string) (*File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open audit file %q: %w", path, err)
	}
	return &File{file: file}, nil
}

// Notify appends an event, serializing writes from concurrent requests.
func (f *File) Notify(ctx context.Context, event auditapp.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal file audit event: %w", err)
	}
	data = append(data, '\n')

	f.mu.Lock()
	defer f.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("write audit file: %w", err)
	}
	if f.file == nil {
		return fmt.Errorf("write audit file: %w", os.ErrClosed)
	}
	if _, err := f.file.Write(data); err != nil {
		return fmt.Errorf("write audit file %q: %w", f.file.Name(), err)
	}
	return nil
}

// Close releases the file after active requests have finished.
func (f *File) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.file == nil {
		return nil
	}
	err := f.file.Close()
	f.file = nil
	if err != nil {
		return fmt.Errorf("close audit file: %w", err)
	}
	return nil
}
