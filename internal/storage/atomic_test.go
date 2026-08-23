package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteFile(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "atomic_storage_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	targetFile := filepath.Join(tempDir, "sub", "test.yaml")
	data := []byte("test content 12345")

	err = AtomicWriteFile(targetFile, data, 0600)
	if err != nil {
		t.Fatalf("AtomicWriteFile failed: %v", err)
	}

	// Verify content
	readData, err := os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(readData) != string(data) {
		t.Fatalf("got %s, want %s", string(readData), string(data))
	}

	// Verify permissions
	info, err := os.Stat(targetFile)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("expected perm 0600, got %o", perm)
	}

	// Overwrite atomically
	newData := []byte("updated content")
	err = AtomicWriteFile(targetFile, newData, 0600)
	if err != nil {
		t.Fatalf("AtomicWriteFile overwrite failed: %v", err)
	}

	readData, err = os.ReadFile(targetFile)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(readData) != string(newData) {
		t.Fatalf("got %s, want %s", string(readData), string(newData))
	}
}
