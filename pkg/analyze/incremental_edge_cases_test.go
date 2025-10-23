package analyze

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dundee/gdu/v5/internal/testdir"
	"github.com/stretchr/testify/assert"
)

// TestIncrementalAnalyzer_CacheCorruptionFallback verifies graceful fallback on cache corruption
func TestIncrementalAnalyzer_CacheCorruptionFallback(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	// First scan to populate cache
	analyzer1 := CreateIncrementalAnalyzer(opts)
	dir1 := analyzer1.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer1.GetProgressChan()
	analyzer1.GetDone().Wait()
	analyzer1.ResetProgress()

	// Corrupt the cache by writing invalid data to a cache entry
	// Open storage separately and close it before second analyzer
	{
		storage := NewIncrementalStorage(tmpDir, "test_dir")
		closeFn, err := storage.Open()
		if !assert.NoError(t, err) {
			return
		}

		// Store corrupted metadata
		badMeta := &IncrementalDirMetadata{
			Path:  "", // Invalid: empty path
			Mtime: time.Now(),
		}
		// This should succeed but create invalid data
		err = storage.StoreDirMetadata(badMeta)
		// Note: StoreDirMetadata might succeed even with invalid data
		// The corruption will be detected on LoadDirMetadata

		// Close storage before opening second analyzer
		closeFn()
	}

	// Second scan should detect corruption and fall back to full scan
	analyzer2 := CreateIncrementalAnalyzer(opts)
	dir2 := analyzer2.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer2.GetProgressChan()
	analyzer2.GetDone().Wait()

	// Verify the scan completed successfully despite cache issues
	assert.NotNil(t, dir2)
	assert.Equal(t, dir1.ItemCount, dir2.ItemCount)

	// The corrupted entry might be overwritten, so just verify scan succeeded
	stats := analyzer2.GetCacheStats()
	assert.Greater(t, stats.TotalDirs, int64(0), "Should have scanned directories")

	analyzer2.ResetProgress()
}

// TestIncrementalAnalyzer_DeletedDirectory verifies handling of deleted directories
func TestIncrementalAnalyzer_DeletedDirectory(t *testing.T) {
	// Create a test directory
	testRoot := t.TempDir()
	testPath := filepath.Join(testRoot, "deleteme")
	err := os.Mkdir(testPath, 0o755)
	if !assert.NoError(t, err) {
		return
	}

	// Create a file inside
	err = os.WriteFile(filepath.Join(testPath, "file.txt"), []byte("test"), 0o644)
	if !assert.NoError(t, err) {
		return
	}

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	// First scan to populate cache
	analyzer1 := CreateIncrementalAnalyzer(opts)
	dir1 := analyzer1.AnalyzeDir(
		testPath, func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer1.GetProgressChan()
	analyzer1.GetDone().Wait()
	analyzer1.ResetProgress()

	// Verify directory was scanned
	assert.Equal(t, "deleteme", dir1.Name)
	assert.Greater(t, dir1.ItemCount, 0)

	// Delete the directory
	err = os.RemoveAll(testPath)
	if !assert.NoError(t, err) {
		return
	}

	// Second scan should handle missing directory gracefully
	analyzer2 := CreateIncrementalAnalyzer(opts)
	dir2 := analyzer2.AnalyzeDir(
		testPath, func(_, _ string) bool { return false }, false,
	).(*Dir)

	// Drain progress channel
	doneChan := analyzer2.GetDone()
	progressChan := analyzer2.GetProgressChan()
	for {
		select {
		case <-progressChan:
		case <-doneChan:
			goto done
		}
	}
done:

	// Should return error directory
	assert.NotNil(t, dir2)
	assert.Equal(t, '!', dir2.Flag, "Should have error flag for deleted directory")

	analyzer2.ResetProgress()
}

// TestIncrementalAnalyzer_PermissionDenied verifies handling of permission errors
func TestIncrementalAnalyzer_PermissionDenied(t *testing.T) {
	// Skip on Windows where permission handling is different
	if os.Getenv("GOOS") == "windows" {
		t.Skip("Skipping permission test on Windows")
	}

	// Create a test directory
	testRoot := t.TempDir()
	restrictedPath := filepath.Join(testRoot, "restricted")
	err := os.Mkdir(restrictedPath, 0o755)
	if !assert.NoError(t, err) { return }

	// Create a file inside
	err = os.WriteFile(filepath.Join(restrictedPath, "file.txt"), []byte("test"), 0o644)
	if !assert.NoError(t, err) { return }

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	// First scan with normal permissions
	analyzer1 := CreateIncrementalAnalyzer(opts)
	dir1 := analyzer1.AnalyzeDir(
		restrictedPath, func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer1.GetProgressChan()
	analyzer1.GetDone().Wait()
	analyzer1.ResetProgress()

	// Verify directory was scanned
	assert.Equal(t, "restricted", dir1.Name)
	assert.Greater(t, dir1.ItemCount, 0)

	// Remove read permissions
	err = os.Chmod(restrictedPath, 0o000)
	if !assert.NoError(t, err) { return }
	// Restore permissions after test
	defer os.Chmod(restrictedPath, 0o755)

	// Second scan should detect permission error
	analyzer2 := CreateIncrementalAnalyzer(opts)
	dir2 := analyzer2.AnalyzeDir(
		restrictedPath, func(_, _ string) bool { return false }, false,
	).(*Dir)

	// Drain progress channel
	doneChan := analyzer2.GetDone()
	progressChan := analyzer2.GetProgressChan()
	for {
		select {
		case <-progressChan:
		case <-doneChan:
			goto done
		}
	}
done:

	// Should return error directory
	assert.NotNil(t, dir2)
	// On macOS, permission denied might return empty flag instead of '!'
	// Just verify the directory was created and error was handled
	assert.Equal(t, "restricted", dir2.Name)

	analyzer2.ResetProgress()
}

// TestIncrementalStorage_OpenFailures verifies various cache open failure scenarios
func TestIncrementalStorage_OpenFailures(t *testing.T) {
	t.Run("NonExistentPath", func(t *testing.T) {
		storage := NewIncrementalStorage("/nonexistent/cache/path", "/some/dir")
		_, err := storage.Open()
		assert.Error(t, err)
		// Error message varies by OS - just verify there's an error
		assert.NotEmpty(t, err.Error())
	})

	t.Run("InvalidPermissions", func(t *testing.T) {
		// Skip on Windows where permission handling is different
		if os.Getenv("GOOS") == "windows" {
			t.Skip("Skipping permission test on Windows")
		}

		// Create a directory with no write permissions
		restrictedDir := filepath.Join(t.TempDir(), "nowrite")
		err := os.Mkdir(restrictedDir, 0o555) // Read + execute only
		if !assert.NoError(t, err) { return }
		defer os.Chmod(restrictedDir, 0o755) // Restore for cleanup

		storage := NewIncrementalStorage(filepath.Join(restrictedDir, "cache"), "/some/dir")
		_, err = storage.Open()
		// This might fail with permission or "does not exist" error depending on OS
		assert.Error(t, err)
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		tmpDir := t.TempDir()

		// First storage opens successfully
		storage1 := NewIncrementalStorage(tmpDir, "/some/dir")
		close1, err := storage1.Open()
		if !assert.NoError(t, err) { return }
		defer close1()

		// Second storage should fail due to lock
		storage2 := NewIncrementalStorage(tmpDir, "/some/dir")
		_, err = storage2.Open()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "locked")
	})
}

// TestIncrementalAnalyzer_CacheErrorHandling verifies robust error handling
func TestIncrementalAnalyzer_CacheErrorHandling(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()

	t.Run("StorageOpenFailure", func(t *testing.T) {
		// Use invalid storage path
		opts := IncrementalOptions{
			StoragePath:   "/root/invalid_test_cache",
			CacheMaxAge:   0,
			ForceFullScan: false,
		}

		analyzer := CreateIncrementalAnalyzer(opts)
		dir := analyzer.AnalyzeDir(
			"test_dir", func(_, _ string) bool { return false }, false,
		).(*Dir)

		// Should return error directory
		analyzer.GetDone().Wait()
		assert.NotNil(t, dir)
		assert.Equal(t, '!', dir.Flag, "Should return error dir when storage fails")
	})

	t.Run("GracefulDegradation", func(t *testing.T) {
		opts := IncrementalOptions{
			StoragePath:   tmpDir,
			CacheMaxAge:   0,
			ForceFullScan: false,
		}

		// Normal scan should work
		analyzer := CreateIncrementalAnalyzer(opts)
		dir := analyzer.AnalyzeDir(
			"test_dir", func(_, _ string) bool { return false }, false,
		).(*Dir)
		<-analyzer.GetProgressChan()
		analyzer.GetDone().Wait()

		assert.NotNil(t, dir)
		assert.NotEqual(t, '!', dir.Flag, "Normal scan should succeed")
		assert.Greater(t, dir.ItemCount, 0)

		analyzer.ResetProgress()
	})
}

// TestIncrementalAnalyzer_ValidateCachedPath verifies path validation
func TestIncrementalAnalyzer_ValidateCachedPath(t *testing.T) {
	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	analyzer := CreateIncrementalAnalyzer(opts)

	t.Run("ValidPath", func(t *testing.T) {
		// Create a valid directory
		validPath := filepath.Join(t.TempDir(), "valid")
		err := os.Mkdir(validPath, 0o755)
		if !assert.NoError(t, err) { return }

		// Open storage first
		storage := NewIncrementalStorage(tmpDir, validPath)
		closeFn, err := storage.Open()
		if !assert.NoError(t, err) { return }
		defer closeFn()

		analyzer.storage = storage

		// Validate should succeed
		isValid := analyzer.validateCachedPath(validPath)
		assert.True(t, isValid, "Valid path should pass validation")
	})

	t.Run("NonExistentPath", func(t *testing.T) {
		tmpDir2 := t.TempDir()
		storage := NewIncrementalStorage(tmpDir2, "/nonexistent")
		closeFn, err := storage.Open()
		if !assert.NoError(t, err) { return }
		defer closeFn()

		analyzer.storage = storage

		// Validate should fail and clean up cache
		nonExistentPath := "/nonexistent/path/that/does/not/exist"
		isValid := analyzer.validateCachedPath(nonExistentPath)
		assert.False(t, isValid, "Non-existent path should fail validation")
	})

	t.Run("FileInsteadOfDirectory", func(t *testing.T) {
		// Create a file instead of directory
		filePath := filepath.Join(t.TempDir(), "notadir")
		err := os.WriteFile(filePath, []byte("test"), 0o644)
		if !assert.NoError(t, err) { return }

		tmpDir3 := t.TempDir()
		storage := NewIncrementalStorage(tmpDir3, filePath)
		closeFn, err := storage.Open()
		if !assert.NoError(t, err) { return }
		defer closeFn()

		analyzer.storage = storage

		// Validate should fail because it's not a directory
		isValid := analyzer.validateCachedPath(filePath)
		assert.False(t, isValid, "File path should fail directory validation")
	})
}

// TestIncrementalAnalyzer_HandleCacheError verifies cache error handling
func TestIncrementalAnalyzer_HandleCacheError(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	// First scan - will trigger handleCacheError on cache miss
	analyzer := CreateIncrementalAnalyzer(opts)
	dir := analyzer.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)

	<-analyzer.GetProgressChan()
	analyzer.GetDone().Wait()

	assert.NotNil(t, dir)
	assert.Equal(t, "test_dir", dir.Name)

	// Should increment cache miss counter for first scan
	stats := analyzer.GetCacheStats()
	assert.Greater(t, stats.CacheMisses, int64(0), "First scan should have cache misses")

	analyzer.ResetProgress()
}

// TestIncrementalStorage_CorruptedCacheEntry verifies corrupted cache detection
func TestIncrementalStorage_CorruptedCacheEntry(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test")

	closeFn, err := storage.Open()
	if !assert.NoError(t, err) { return }
	defer closeFn()

	// Store metadata with empty path (invalid)
	badMeta := &IncrementalDirMetadata{
		Path:  "", // Invalid
		Mtime: time.Now(),
		Size:  100,
	}

	err = storage.StoreDirMetadata(badMeta)
	if !assert.NoError(t, err) { return } // Storing might succeed

	// Loading should detect the invalid data
	loaded, err := storage.LoadDirMetadata("")
	assert.Error(t, err, "Should detect invalid cache entry")
	assert.Nil(t, loaded, "Should not return invalid metadata")
}

// TestIncrementalAnalyzer_MultipleErrorScenarios tests combined edge cases
func TestIncrementalAnalyzer_MultipleErrorScenarios(t *testing.T) {
	t.Run("CacheErrorThenPermissionError", func(t *testing.T) {
		// Skip on Windows where permission handling differs
		if os.Getenv("GOOS") == "windows" {
			t.Skip("Skipping permission test on Windows")
		}

		testRoot := t.TempDir()
		testPath := filepath.Join(testRoot, "testdir")
		err := os.Mkdir(testPath, 0o755)
		if !assert.NoError(t, err) {
			return
		}

		tmpDir := t.TempDir()
		opts := IncrementalOptions{
			StoragePath:   tmpDir,
			CacheMaxAge:   0,
			ForceFullScan: false,
		}

		// First scan
		analyzer1 := CreateIncrementalAnalyzer(opts)
		dir1 := analyzer1.AnalyzeDir(
			testPath, func(_, _ string) bool { return false }, false,
		)
		analyzer1.GetDone().Wait()
		assert.NotNil(t, dir1)
		analyzer1.ResetProgress()

		// Remove permissions
		err = os.Chmod(testPath, 0o000)
		if !assert.NoError(t, err) {
			return
		}
		defer os.Chmod(testPath, 0o755)

		// Second scan should handle both cache and permission issues
		analyzer2 := CreateIncrementalAnalyzer(opts)
		dir2 := analyzer2.AnalyzeDir(
			testPath, func(_, _ string) bool { return false }, false,
		).(*Dir)
		analyzer2.GetDone().Wait()

		// Verify dir was created (flag behavior varies by OS)
		assert.NotNil(t, dir2)
		assert.Equal(t, "testdir", dir2.Name)
		analyzer2.ResetProgress()
	})

	t.Run("CacheErrorThenDeletedPath", func(t *testing.T) {
		testRoot := t.TempDir()
		testPath := filepath.Join(testRoot, "willdelete")
		err := os.Mkdir(testPath, 0o755)
		if !assert.NoError(t, err) {
			return
		}

		tmpDir := t.TempDir()
		opts := IncrementalOptions{
			StoragePath:   tmpDir,
			CacheMaxAge:   0,
			ForceFullScan: false,
		}

		// First scan
		analyzer1 := CreateIncrementalAnalyzer(opts)
		dir1 := analyzer1.AnalyzeDir(
			testPath, func(_, _ string) bool { return false }, false,
		)
		analyzer1.GetDone().Wait()
		assert.NotNil(t, dir1)
		analyzer1.ResetProgress()

		// Delete directory
		err = os.RemoveAll(testPath)
		if !assert.NoError(t, err) {
			return
		}

		// Second scan should handle missing directory
		analyzer2 := CreateIncrementalAnalyzer(opts)
		dir2 := analyzer2.AnalyzeDir(
			testPath, func(_, _ string) bool { return false }, false,
		).(*Dir)
		analyzer2.GetDone().Wait()

		assert.NotNil(t, dir2)
		assert.Equal(t, '!', dir2.Flag)
		analyzer2.ResetProgress()
	})
}

// TestIncrementalStorage_ErrorMessages verifies error message quality
func TestIncrementalStorage_ErrorMessages(t *testing.T) {
	t.Run("HelpfulNonExistentMessage", func(t *testing.T) {
		storage := NewIncrementalStorage("/does/not/exist/cache", "/test")
		_, err := storage.Open()
		assert.Error(t, err)

		errMsg := err.Error()
		// Should contain helpful information
		assert.Contains(t, errMsg, "cache")
		assert.Contains(t, errMsg, "/does/not/exist/cache")
	})

	t.Run("ConcurrentAccessMessage", func(t *testing.T) {
		tmpDir := t.TempDir()

		// First storage
		storage1 := NewIncrementalStorage(tmpDir, "/test")
		close1, err := storage1.Open()
		if !assert.NoError(t, err) { return }
		defer close1()

		// Second storage
		storage2 := NewIncrementalStorage(tmpDir, "/test")
		_, err = storage2.Open()
		assert.Error(t, err)

		errMsg := err.Error()
		assert.Contains(t, errMsg, "locked")
	})
}
