package common

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFixture provides common test utilities and test data generators.
type TestFixture struct {
	tmpDir string
	t      *testing.T
}

// NewTestFixture creates a new test fixture with a temporary directory.
func NewTestFixture(t *testing.T) *TestFixture {
	tmpDir := t.TempDir()
	return &TestFixture{
		tmpDir: tmpDir,
		t:      t,
	}
}

// TempDir returns the temporary directory path.
func (tf *TestFixture) TempDir() string {
	return tf.tmpDir
}

// CreateFile creates a file in the temporary directory with the given content.
func (tf *TestFixture) CreateFile(filename string, content []byte) string {
	filePath := filepath.Join(tf.tmpDir, filename)

	// Create nested directories if needed
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		tf.t.Fatalf("Failed to create directory %s: %v", dir, err)
	}

	if err := os.WriteFile(filePath, content, 0644); err != nil {
		tf.t.Fatalf("Failed to create file %s: %v", filePath, err)
	}

	return filePath
}

// CreateDirectory creates a directory in the temporary directory.
func (tf *TestFixture) CreateDirectory(dirname string) string {
	dirPath := filepath.Join(tf.tmpDir, dirname)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		tf.t.Fatalf("Failed to create directory %s: %v", dirPath, err)
	}
	return dirPath
}

// ReadFile reads a file from the temporary directory.
func (tf *TestFixture) ReadFile(filename string) []byte {
	filePath := filepath.Join(tf.tmpDir, filename)
	content, err := os.ReadFile(filePath)
	if err != nil {
		tf.t.Fatalf("Failed to read file %s: %v", filePath, err)
	}
	return content
}

// FileExists checks if a file exists in the temporary directory.
func (tf *TestFixture) FileExists(filename string) bool {
	filePath := filepath.Join(tf.tmpDir, filename)
	_, err := os.Stat(filePath)
	return err == nil
}
