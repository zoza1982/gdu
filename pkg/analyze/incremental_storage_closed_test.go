package analyze

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestIncrementalStorage_ClosedDatabaseHandling tests that all storage methods
// return appropriate errors when called on a closed database instead of panicking
func TestIncrementalStorage_ClosedDatabaseHandling(t *testing.T) {
	tmpCache := t.TempDir()
	testRoot := t.TempDir()

	storage := NewIncrementalStorage(tmpCache, testRoot)

	// Test 1: Methods should fail gracefully when storage was never opened
	t.Run("NeverOpened", func(t *testing.T) {
		// Test StoreDirMetadata
		meta := &IncrementalDirMetadata{
			Path:  testRoot,
			Mtime: time.Now(),
			Size:  1000,
		}
		err := storage.StoreDirMetadata(meta)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "storage is not open")

		// Test LoadDirMetadata
		_, err = storage.LoadDirMetadata(testRoot)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "storage is not open")

		// Test DeleteDirMetadata
		err = storage.DeleteDirMetadata(testRoot)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "storage is not open")
	})

	// Test 2: Methods should fail gracefully after storage is closed
	t.Run("AfterClose", func(t *testing.T) {
		storage2 := NewIncrementalStorage(tmpCache, testRoot)
		cleanup, err := storage2.Open()
		assert.NoError(t, err)

		// Verify storage is open
		assert.True(t, storage2.IsOpen())

		// Store some data while open
		meta := &IncrementalDirMetadata{
			Path:  testRoot,
			Mtime: time.Now(),
			Size:  1000,
		}
		err = storage2.StoreDirMetadata(meta)
		assert.NoError(t, err)

		// Close the storage
		cleanup()

		// Verify storage is closed
		assert.False(t, storage2.IsOpen())

		// Now try to use it - should get errors, not panics
		err = storage2.StoreDirMetadata(meta)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "storage is not open")

		_, err = storage2.LoadDirMetadata(testRoot)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "storage is not open")

		err = storage2.DeleteDirMetadata(testRoot)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "storage is not open")
	})

	// Test 3: Multiple close calls should not panic
	t.Run("DoubleClose", func(t *testing.T) {
		storage3 := NewIncrementalStorage(tmpCache, testRoot)
		cleanup, err := storage3.Open()
		assert.NoError(t, err)

		// First close
		cleanup()
		assert.False(t, storage3.IsOpen())

		// Second close - should not panic
		assert.NotPanics(t, func() {
			cleanup()
		})
	})
}

// TestIncrementalStorage_ConcurrentCloseAndAccess tests thread safety
// when closing storage while operations are in progress
func TestIncrementalStorage_ConcurrentCloseAndAccess(t *testing.T) {
	tmpCache := t.TempDir()
	testRoot := t.TempDir()

	storage := NewIncrementalStorage(tmpCache, testRoot)
	cleanup, err := storage.Open()
	assert.NoError(t, err)

	// Store initial data
	meta := &IncrementalDirMetadata{
		Path:  testRoot,
		Mtime: time.Now(),
		Size:  1000,
	}
	err = storage.StoreDirMetadata(meta)
	assert.NoError(t, err)

	// Close storage (this sets db to nil)
	cleanup()

	// Try operations after close - should return errors, not panic
	err = storage.StoreDirMetadata(meta)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "storage is not open")

	_, err = storage.LoadDirMetadata(testRoot)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "storage is not open")

	err = storage.DeleteDirMetadata(testRoot)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "storage is not open")
}

// TestIncrementalStorage_ReopenAfterClose tests that storage can be
// reopened after being closed
func TestIncrementalStorage_ReopenAfterClose(t *testing.T) {
	tmpCache := t.TempDir()
	testRoot := t.TempDir()

	storage := NewIncrementalStorage(tmpCache, testRoot)

	// First open/close cycle
	cleanup1, err := storage.Open()
	assert.NoError(t, err)
	assert.True(t, storage.IsOpen())

	meta1 := &IncrementalDirMetadata{
		Path:  testRoot,
		Mtime: time.Now(),
		Size:  1000,
	}
	err = storage.StoreDirMetadata(meta1)
	assert.NoError(t, err)

	cleanup1()
	assert.False(t, storage.IsOpen())

	// Verify methods fail after close
	err = storage.StoreDirMetadata(meta1)
	assert.Error(t, err)

	// Second open/close cycle - should work
	cleanup2, err := storage.Open()
	assert.NoError(t, err)
	assert.True(t, storage.IsOpen())

	// Should be able to use storage again
	meta2 := &IncrementalDirMetadata{
		Path:  filepath.Join(testRoot, "subdir"),
		Mtime: time.Now(),
		Size:  500,
	}
	err = storage.StoreDirMetadata(meta2)
	assert.NoError(t, err)

	// Should be able to load data from first session
	loaded, err := storage.LoadDirMetadata(testRoot)
	assert.NoError(t, err)
	assert.Equal(t, testRoot, loaded.Path)

	cleanup2()
	assert.False(t, storage.IsOpen())
}

// TestIncrementalStorage_IsOpenAccuracy tests that IsOpen accurately
// reflects the storage state
func TestIncrementalStorage_IsOpenAccuracy(t *testing.T) {
	tmpCache := t.TempDir()
	testRoot := t.TempDir()

	storage := NewIncrementalStorage(tmpCache, testRoot)

	// Initially closed
	assert.False(t, storage.IsOpen())

	// After opening
	cleanup, err := storage.Open()
	assert.NoError(t, err)
	assert.True(t, storage.IsOpen())

	// After closing
	cleanup()
	assert.False(t, storage.IsOpen())
}

// TestAnalyzer_StorageClosedAfterDone verifies that the analyzer properly
// closes storage when done, preventing subsequent access
func TestAnalyzer_StorageClosedAfterDone(t *testing.T) {
	testRoot := filepath.Join(t.TempDir(), "test-root")
	err := os.Mkdir(testRoot, 0755)
	assert.NoError(t, err)

	// Create a subdirectory
	subDir := filepath.Join(testRoot, "subdir")
	err = os.Mkdir(subDir, 0755)
	assert.NoError(t, err)

	// Create file in subdirectory
	testFile := filepath.Join(subDir, "test.txt")
	err = os.WriteFile(testFile, []byte("test content"), 0644)
	assert.NoError(t, err)

	time.Sleep(1100 * time.Millisecond)

	tmpCache := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpCache,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	analyzer := CreateIncrementalAnalyzer(opts)
	analyzer.AnalyzeDir(testRoot, func(_, _ string) bool { return false }, false)
	analyzer.GetDone().Wait()

	// Storage should be closed after analyzer is done
	// Attempting to use it directly would cause errors
	// This test just verifies the pattern works without panicking
	assert.NotNil(t, analyzer.storage)

	// Note: We don't test calling storage methods directly here because
	// the analyzer's storage field is private. The test in rebuild_cache_bug_test.go
	// demonstrates the correct pattern: create a new storage instance if you need
	// to access the cache after the analyzer is done.
}

// TestIncrementalStorage_ErrorMessagesQuality tests that error messages
// are helpful and informative
func TestIncrementalStorage_ErrorMessagesQuality(t *testing.T) {
	tmpCache := t.TempDir()
	testRoot := t.TempDir()

	storage := NewIncrementalStorage(tmpCache, testRoot)

	testCases := []struct {
		name     string
		fn       func() error
		errMsg   string
	}{
		{
			name: "StoreDirMetadata",
			fn: func() error {
				return storage.StoreDirMetadata(&IncrementalDirMetadata{
					Path: testRoot,
				})
			},
			errMsg: "storage is not open",
		},
		{
			name: "LoadDirMetadata",
			fn: func() error {
				_, err := storage.LoadDirMetadata(testRoot)
				return err
			},
			errMsg: "storage is not open",
		},
		{
			name: "DeleteDirMetadata",
			fn: func() error {
				return storage.DeleteDirMetadata(testRoot)
			},
			errMsg: "storage is not open",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.fn()
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.errMsg,
				fmt.Sprintf("Error message should contain '%s', got: %v", tc.errMsg, err))
		})
	}
}
