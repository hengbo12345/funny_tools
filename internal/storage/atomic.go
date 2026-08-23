package storage

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// DefaultDirPerm is 0700 for sensitive directories
	DefaultDirPerm os.FileMode = 0700
	// DefaultFilePerm is 0600 for sensitive files
	DefaultFilePerm os.FileMode = 0600
)

// EnsureDir ensures that the directory exists with the specified permissions.
func EnsureDir(dir string, perm os.FileMode) error {
	if perm == 0 {
		perm = DefaultDirPerm
	}
	return os.MkdirAll(dir, perm)
}

// AtomicWriteFile writes data to a temporary file in the same directory,
// fsyncs, closes, and atomically renames it to the target file.
func AtomicWriteFile(filename string, data []byte, perm os.FileMode) error {
	if perm == 0 {
		perm = DefaultFilePerm
	}

	dir := filepath.Dir(filename)
	if err := EnsureDir(dir, DefaultDirPerm); err != nil {
		return fmt.Errorf("failed to ensure dir %s: %w", dir, err)
	}

	// Create a temp file in the same directory to ensure it's on the same filesystem
	tmpFile, err := os.CreateTemp(dir, filepath.Base(filename)+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file in %s: %w", dir, err)
	}
	tmpName := tmpFile.Name()

	// Ensure cleanup on failure
	var success bool
	defer func() {
		if !success {
			_ = tmpFile.Close()
			_ = os.Remove(tmpName)
		}
	}()

	// Set file permissions explicitly before writing sensitive content
	if err := os.Chmod(tmpName, perm); err != nil {
		return fmt.Errorf("failed to chmod temp file %s: %w", tmpName, err)
	}

	// Write data
	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("failed to write to temp file %s: %w", tmpName, err)
	}

	// Fsync to flush data to disk
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temp file %s: %w", tmpName, err)
	}

	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file %s: %w", tmpName, err)
	}

	// Atomic rename
	if err := os.Rename(tmpName, filename); err != nil {
		return fmt.Errorf("failed to rename %s to %s: %w", tmpName, filename, err)
	}

	success = true
	return nil
}

// ReadFile reads the file contents.
func ReadFile(filename string) ([]byte, error) {
	return os.ReadFile(filename)
}
