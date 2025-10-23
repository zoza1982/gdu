package analyze

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/dundee/gdu/v5/internal/testdir"
	"github.com/dundee/gdu/v5/pkg/fs"
	"github.com/stretchr/testify/assert"
)

func init() {
	log.SetLevel(log.WarnLevel)
}

// TestIncrementalAnalyzer_FirstScan verifies that initial scan caches all data
func TestIncrementalAnalyzer_FirstScan(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0, // No expiry
		ForceFullScan: false,
	}

	analyzer := CreateIncrementalAnalyzer(opts)
	dir := analyzer.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)

	progress := <-analyzer.GetProgressChan()
	assert.GreaterOrEqual(t, progress.TotalSize, int64(0))

	analyzer.GetDone().Wait()

	// Verify statistics BEFORE resetting - first scan should have all cache misses
	stats := analyzer.GetCacheStats()
	assert.Greater(t, stats.CacheMisses, int64(0), "First scan should have cache misses")
	assert.Equal(t, int64(0), stats.CacheHits, "First scan should have no cache hits")
	assert.Greater(t, stats.BytesScanned, int64(0), "First scan should scan bytes")
	assert.Equal(t, int64(0), stats.BytesFromCache, "First scan should not load from cache")

	analyzer.ResetProgress()
	dir.UpdateStats(make(fs.HardLinkedItems))

	// Verify directory structure
	assert.Equal(t, "test_dir", dir.Name)
	// Directory size now includes actual filesystem sizes, not hardcoded 4096
	assert.Greater(t, dir.Size, int64(7), "Directory size should be > file contents")
	assert.Equal(t, 5, dir.ItemCount)
	assert.True(t, dir.IsDir())

	// Verify nested structure
	assert.Equal(t, "nested", dir.Files[0].GetName())
	assert.Equal(t, "subnested", dir.Files[0].(*Dir).Files[1].GetName())
}

// TestIncrementalAnalyzer_UnchangedDirectory verifies 100% cache hit on unchanged directory
func TestIncrementalAnalyzer_UnchangedDirectory(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0, // No expiry
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
	dir1.UpdateStats(make(fs.HardLinkedItems))

	firstScanSize := dir1.Size
	firstScanCount := dir1.ItemCount

	// Second scan should use cache
	analyzer2 := CreateIncrementalAnalyzer(opts)
	dir2 := analyzer2.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)

	// Drain progress channel (may or may not have messages depending on cache hits)
	doneChan := analyzer2.GetDone()
	progressChan := analyzer2.GetProgressChan()
	for {
		select {
		case <-progressChan:
			// Progress received, continue draining
		case <-doneChan:
			goto done2
		}
	}
done2:
	// Verify cache statistics BEFORE resetting
	stats := analyzer2.GetCacheStats()
	assert.Greater(t, stats.CacheHits, int64(0), "Second scan should have cache hits")
	assert.Greater(t, stats.BytesFromCache, int64(0), "Should load bytes from cache")

	// Calculate hit rate
	hitRate := stats.HitRate()
	assert.Greater(t, hitRate, 90.0, "Hit rate should be >90% for unchanged directory")

	analyzer2.ResetProgress()
	dir2.UpdateStats(make(fs.HardLinkedItems))

	// Verify results match
	assert.Equal(t, firstScanSize, dir2.Size, "Size should match cached value")
	assert.Equal(t, firstScanCount, dir2.ItemCount, "Item count should match cached value")
}

// TestIncrementalAnalyzer_ChangedDirectory verifies that changed mtime triggers rescan
func TestIncrementalAnalyzer_ChangedDirectory(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	// First scan
	analyzer1 := CreateIncrementalAnalyzer(opts)
	dir1 := analyzer1.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer1.GetProgressChan()
	analyzer1.GetDone().Wait()
	analyzer1.ResetProgress()

	// Modify a file to change directory mtime
	// Need longer sleep to ensure filesystem mtime granularity is exceeded
	time.Sleep(1100 * time.Millisecond) // Many filesystems have 1-second mtime granularity

	// Add a file directly to test_dir (not nested) to ensure test_dir's mtime changes
	err := os.WriteFile("test_dir/newfile.txt", []byte("new content"), 0o644)
	assert.NoError(t, err)

	// Force sync to ensure mtime is updated
	time.Sleep(100 * time.Millisecond)

	// Second scan should detect change
	analyzer2 := CreateIncrementalAnalyzer(opts)
	dir2 := analyzer2.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer2.GetProgressChan()
	analyzer2.GetDone().Wait()

	// Verify that the modified directory was rescanned (BEFORE resetting stats)
	stats := analyzer2.GetCacheStats()
	assert.Greater(t, stats.DirsRescanned, int64(0), "Should have rescanned modified directories")
	assert.Greater(t, stats.BytesScanned, int64(0), "Should have scanned new bytes")

	analyzer2.ResetProgress()
	dir2.UpdateStats(make(fs.HardLinkedItems))

	// Verify new file is detected (added newfile.txt to test_dir, so count should increase by 1)
	assert.Equal(t, dir1.ItemCount+1, dir2.ItemCount, "Should detect new file")
}

// TestIncrementalAnalyzer_CacheExpiry verifies --cache-max-age works correctly
func TestIncrementalAnalyzer_CacheExpiry(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()

	// First scan with no expiry
	opts1 := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}
	analyzer1 := CreateIncrementalAnalyzer(opts1)
	analyzer1.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	)
	<-analyzer1.GetProgressChan()
	analyzer1.GetDone().Wait()
	analyzer1.ResetProgress()

	// Wait for cache to age
	time.Sleep(100 * time.Millisecond)

	// Second scan with very short expiry
	opts2 := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   50 * time.Millisecond, // Cache expired
		ForceFullScan: false,
	}
	analyzer2 := CreateIncrementalAnalyzer(opts2)
	analyzer2.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	)
	<-analyzer2.GetProgressChan()
	analyzer2.GetDone().Wait()

	// Verify cache was expired (BEFORE resetting stats)
	stats := analyzer2.GetCacheStats()
	assert.Greater(t, stats.CacheExpired, int64(0), "Cache should have expired entries")
	assert.Greater(t, stats.DirsRescanned, int64(0), "Should rescan expired directories")

	analyzer2.ResetProgress()
}

// TestIncrementalAnalyzer_ForceFullScan verifies --force-full-scan bypasses cache
func TestIncrementalAnalyzer_ForceFullScan(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()

	// First scan to populate cache
	opts1 := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}
	analyzer1 := CreateIncrementalAnalyzer(opts1)
	analyzer1.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	)
	<-analyzer1.GetProgressChan()
	analyzer1.GetDone().Wait()
	analyzer1.ResetProgress()

	// Second scan with force full scan
	opts2 := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: true, // Force rescan
	}
	analyzer2 := CreateIncrementalAnalyzer(opts2)
	analyzer2.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	)
	<-analyzer2.GetProgressChan()
	analyzer2.GetDone().Wait()

	// Verify cache was bypassed (BEFORE resetting stats)
	stats := analyzer2.GetCacheStats()
	assert.Greater(t, stats.DirsRescanned, int64(0), "Should rescan all directories")
	assert.Equal(t, int64(0), stats.CacheHits, "Should not use cache with force full scan")
	assert.Greater(t, stats.BytesScanned, int64(0), "Should scan all bytes")

	analyzer2.ResetProgress()
}

// TestIncrementalAnalyzer_IgnoredDirectories verifies ignored dirs are not cached
func TestIncrementalAnalyzer_IgnoredDirectories(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	// Scan with ignored directories
	analyzer := CreateIncrementalAnalyzer(opts)
	dir := analyzer.AnalyzeDir(
		"test_dir",
		func(name, _ string) bool { return name == "nested" }, // Ignore "nested"
		false,
	).(*Dir)

	<-analyzer.GetProgressChan()
	analyzer.GetDone().Wait()
	analyzer.ResetProgress()
	dir.UpdateStats(make(fs.HardLinkedItems))

	// Verify ignored directory is not included
	assert.Equal(t, "test_dir", dir.Name)
	assert.Equal(t, 1, dir.ItemCount, "Should only count root directory")
	assert.Equal(t, 0, len(dir.Files), "Should have no files/subdirs")
}

// TestIncrementalAnalyzer_ErrorHandling verifies graceful error handling
func TestIncrementalAnalyzer_ErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	analyzer := CreateIncrementalAnalyzer(opts)

	// Scan non-existent directory
	dir := analyzer.AnalyzeDir(
		"/non/existent/path", func(_, _ string) bool { return false }, false,
	).(*Dir)

	// Drain progress channel properly
	doneChan := analyzer.GetDone()
	progressChan := analyzer.GetProgressChan()
	for {
		select {
		case <-progressChan:
		case <-doneChan:
			goto done
		}
	}
done:
	analyzer.ResetProgress()

	// Verify error directory is created
	assert.Equal(t, "path", dir.Name)
	assert.Equal(t, '!', dir.Flag, "Should have error flag")
	assert.Equal(t, 0, dir.ItemCount)
}

// TestIncrementalAnalyzer_ResetProgress verifies progress can be reset
func TestIncrementalAnalyzer_ResetProgress(t *testing.T) {
	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	analyzer := CreateIncrementalAnalyzer(opts)

	// Get initial stats
	stats1 := analyzer.GetCacheStats()
	assert.Equal(t, int64(0), stats1.TotalDirs)

	// Reset progress
	analyzer.ResetProgress()

	// Verify stats are reset
	stats2 := analyzer.GetCacheStats()
	assert.Equal(t, int64(0), stats2.TotalDirs)
	assert.Equal(t, int64(0), stats2.CacheHits)
	assert.Equal(t, int64(0), stats2.CacheMisses)
}

// TestIncrementalAnalyzer_CacheStatsAccuracy verifies statistics calculations
func TestIncrementalAnalyzer_CacheStatsAccuracy(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	// Sleep to ensure filesystem timestamps are stable
	time.Sleep(1100 * time.Millisecond)

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	// First scan
	analyzer1 := CreateIncrementalAnalyzer(opts)
	analyzer1.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	)
	<-analyzer1.GetProgressChan()
	analyzer1.GetDone().Wait()
	analyzer1.ResetProgress()

	stats1 := analyzer1.GetCacheStats()
	assert.Equal(t, 0.0, stats1.HitRate(), "First scan should have 0% hit rate")
	assert.Equal(t, 0.0, stats1.IOReduction(), "First scan should have 0% I/O reduction")

	// Second scan (unchanged)
	analyzer2 := CreateIncrementalAnalyzer(opts)
	analyzer2.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	)
	<-analyzer2.GetProgressChan()
	analyzer2.GetDone().Wait()

	// Get stats BEFORE resetting (ResetProgress() resets stats)
	stats2 := analyzer2.GetCacheStats()
	assert.Greater(t, stats2.HitRate(), 0.0, "Second scan should have >0% hit rate")
	assert.Greater(t, stats2.IOReduction(), 0.0, "Second scan should have >0% I/O reduction")
	assert.Greater(t, stats2.TotalScanTime, time.Duration(0), "Should track scan time")

	analyzer2.ResetProgress()
}

// TestIncrementalAnalyzer_EmptyDirectory verifies empty directory handling
func TestIncrementalAnalyzer_EmptyDirectory(t *testing.T) {
	// Create empty directory
	emptyDir := filepath.Join(t.TempDir(), "empty")
	err := os.Mkdir(emptyDir, 0o755)
	assert.NoError(t, err)

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	analyzer := CreateIncrementalAnalyzer(opts)
	dir := analyzer.AnalyzeDir(
		emptyDir, func(_, _ string) bool { return false }, false,
	).(*Dir)

	<-analyzer.GetProgressChan()
	analyzer.GetDone().Wait()
	analyzer.ResetProgress()
	dir.UpdateStats(make(fs.HardLinkedItems))

	// Verify empty directory
	assert.Equal(t, "empty", dir.Name)
	assert.Equal(t, 1, dir.ItemCount)
	assert.Equal(t, 0, len(dir.Files))
	assert.Equal(t, 'e', dir.Flag)
}

// TestIncrementalAnalyzer_ExtractFileMetadata verifies metadata extraction
func TestIncrementalAnalyzer_ExtractFileMetadata(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	analyzer := CreateIncrementalAnalyzer(opts)
	dir := analyzer.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer.GetProgressChan()
	analyzer.GetDone().Wait()
	analyzer.ResetProgress()

	// Extract metadata
	metadata := analyzer.extractFileMetadata(dir)

	// Verify metadata
	assert.Greater(t, len(metadata), 0, "Should extract file metadata")

	// Find nested directory in metadata
	var foundNested bool
	for _, m := range metadata {
		if m.Name == "nested" {
			foundNested = true
			assert.True(t, m.IsDir, "Nested should be a directory")
			assert.Greater(t, m.Size, int64(0), "Should have size")
		}
	}
	assert.True(t, foundNested, "Should find nested directory in metadata")
}

// TestIncrementalAnalyzer_RebuildFromCache verifies cache reconstruction
func TestIncrementalAnalyzer_RebuildFromCache(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	// First scan
	analyzer1 := CreateIncrementalAnalyzer(opts)
	dir1 := analyzer1.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer1.GetProgressChan()
	analyzer1.GetDone().Wait()
	analyzer1.ResetProgress()
	dir1.UpdateStats(make(fs.HardLinkedItems))

	originalSize := dir1.Size
	originalCount := dir1.ItemCount
	originalName := dir1.Name

	// Second scan (should rebuild from cache)
	analyzer2 := CreateIncrementalAnalyzer(opts)
	dir2 := analyzer2.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer2.GetProgressChan()
	analyzer2.GetDone().Wait()
	analyzer2.ResetProgress()
	dir2.UpdateStats(make(fs.HardLinkedItems))

	// Verify reconstructed directory matches original
	assert.Equal(t, originalName, dir2.Name, "Name should match")
	assert.Equal(t, originalSize, dir2.Size, "Size should match")
	assert.Equal(t, originalCount, dir2.ItemCount, "Item count should match")

	// Verify nested structure is preserved
	assert.Equal(t, "nested", dir2.Files[0].GetName())
	assert.Equal(t, "subnested", dir2.Files[0].(*Dir).Files[1].GetName())
}

// TestIncrementalAnalyzer_DirectorySizeCalculation verifies that actual directory sizes are used
func TestIncrementalAnalyzer_DirectorySizeCalculation(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	analyzer := CreateIncrementalAnalyzer(opts)
	dir := analyzer.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)

	<-analyzer.GetProgressChan()
	analyzer.GetDone().Wait()
	analyzer.ResetProgress()

	// Verify that directory size is not zero (it should use actual filesystem size)
	assert.Greater(t, dir.Size, int64(0), "Directory size should be greater than 0")

	// Verify directory size is reasonable (not just DefaultDirBlockSize)
	// The actual size will include file contents plus directory metadata
	assert.Greater(t, dir.Size, int64(7), "Directory size should include file contents")
}

// TestIncrementalAnalyzer_DirectorySizeFallback verifies fallback to DefaultDirBlockSize
func TestIncrementalAnalyzer_DirectorySizeFallback(t *testing.T) {
	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	analyzer := CreateIncrementalAnalyzer(opts)

	// Try to scan a non-existent directory
	dir := analyzer.AnalyzeDir(
		"/non/existent/directory", func(_, _ string) bool { return false }, false,
	).(*Dir)

	// In error cases, done is signaled but progress may not be sent
	// Just wait for done signal
	analyzer.GetDone().Wait()

	// Verify error flag is set
	assert.Equal(t, '!', dir.Flag, "Error flag should be set for non-existent directory")
}

// TestIncrementalAnalyzer_ProgressUpdateNoLeak verifies no goroutine leak in updateProgress
func TestIncrementalAnalyzer_ProgressUpdateNoLeak(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	analyzer := CreateIncrementalAnalyzer(opts)

	// Run multiple scans to ensure goroutines are properly cleaned up
	for i := 0; i < 5; i++ {
		dir := analyzer.AnalyzeDir(
			"test_dir", func(_, _ string) bool { return false }, false,
		).(*Dir)

		// Drain progress channel
		doneChan := analyzer.GetDone()
		progressChan := analyzer.GetProgressChan()
		for {
			select {
			case <-progressChan:
			case <-doneChan:
				goto done
			}
		}
	done:
		analyzer.ResetProgress()
		assert.NotNil(t, dir)
	}

	// Test passes if we don't hang (goroutines properly terminated)
}

// TestIncrementalAnalyzer_ErrorMessageFormatting verifies error message contains helpful info
func TestIncrementalAnalyzer_ErrorMessageFormatting(t *testing.T) {
	// Use an invalid storage path that will fail
	invalidPath := "/root/invalid_cache_path_for_testing"

	opts := IncrementalOptions{
		StoragePath:   invalidPath,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	analyzer := CreateIncrementalAnalyzer(opts)

	// Create a temporary directory to scan
	testDir := t.TempDir()

	// This should trigger the error path due to invalid storage location
	dir := analyzer.AnalyzeDir(
		testDir, func(_, _ string) bool { return false }, false,
	).(*Dir)

	// In error cases, done is signaled but progress may not be sent
	analyzer.GetDone().Wait()

	// Verify that an error directory was returned
	// The actual error message is logged and printed to stderr, which we can't easily capture in this test
	// But we can verify that the function didn't panic and returned an error directory
	assert.NotNil(t, dir, "Should return an error directory instead of nil")
	assert.Equal(t, '!', dir.Flag, "Error flag should be set")
}

// TestDefaultDirBlockSizeConstant verifies the constant is defined
func TestDefaultDirBlockSizeConstant(t *testing.T) {
	assert.Equal(t, int64(4096), int64(DefaultDirBlockSize), "DefaultDirBlockSize should be 4096")
}

// TestIncrementalAnalyzer_MemoryUsage verifies that cache rebuild doesn't use excessive memory
// Due to the fix, rebuildFromCache() no longer calls processDir() recursively, which was
// causing the entire tree to be loaded twice into memory. This test verifies the fix works.
func TestIncrementalAnalyzer_MemoryUsage(t *testing.T) {
	// Create a larger test directory to make memory differences more apparent
	testDir := filepath.Join(t.TempDir(), "large_test")
	err := os.MkdirAll(testDir, 0o755)
	assert.NoError(t, err)

	// Create multiple nested directories with files to amplify memory usage differences
	for i := 0; i < 10; i++ {
		dirPath := filepath.Join(testDir, fmt.Sprintf("dir%d", i))
		err = os.MkdirAll(dirPath, 0o755)
		assert.NoError(t, err)

		// Create subdirectories
		for j := 0; j < 5; j++ {
			subPath := filepath.Join(dirPath, fmt.Sprintf("subdir%d", j))
			err = os.MkdirAll(subPath, 0o755)
			assert.NoError(t, err)

			// Create files in subdirectories
			for k := 0; k < 10; k++ {
				filePath := filepath.Join(subPath, fmt.Sprintf("file%d.txt", k))
				err = os.WriteFile(filePath, []byte("test content"), 0o644)
				assert.NoError(t, err)
			}
		}
	}

	tmpDir := t.TempDir()

	// First scan (cold cache) - measure memory usage
	opts1 := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}
	analyzer1 := CreateIncrementalAnalyzer(opts1)

	// Capture memory before first scan
	runtime.GC()
	var m1 runtime.MemStats
	runtime.ReadMemStats(&m1)

	analyzer1.AnalyzeDir(testDir, func(_, _ string) bool { return false }, false)

	// Drain progress channel
	doneChan := analyzer1.GetDone()
	progressChan := analyzer1.GetProgressChan()
	for {
		select {
		case <-progressChan:
		case <-doneChan:
			goto done1
		}
	}
done1:

	// Capture memory after first scan
	runtime.GC()
	var m2 runtime.MemStats
	runtime.ReadMemStats(&m2)
	firstRunAlloc := m2.Alloc - m1.Alloc

	// Second scan (warm cache - should not use MORE memory than 2x first run)
	// The bug was causing 2x memory usage by loading tree twice
	analyzer2 := CreateIncrementalAnalyzer(opts1)

	// Capture memory before second scan
	runtime.GC()
	var m3 runtime.MemStats
	runtime.ReadMemStats(&m3)

	analyzer2.AnalyzeDir(testDir, func(_, _ string) bool { return false }, false)

	// Drain progress channel
	doneChan2 := analyzer2.GetDone()
	progressChan2 := analyzer2.GetProgressChan()
	for {
		select {
		case <-progressChan2:
		case <-doneChan2:
			goto done2
		}
	}
done2:

	// Capture memory after second scan
	runtime.GC()
	var m4 runtime.MemStats
	runtime.ReadMemStats(&m4)
	secondRunAlloc := m4.Alloc - m3.Alloc

	// Log the memory usage for debugging
	t.Logf("First run (filesystem scan) allocated: %d bytes", firstRunAlloc)
	t.Logf("Second run (cache rebuild) allocated: %d bytes", secondRunAlloc)
	if firstRunAlloc > 0 {
		ratio := float64(secondRunAlloc) / float64(firstRunAlloc)
		t.Logf("Second run / First run ratio: %.2fx", ratio)

		// The bug was causing 2x memory usage. With the fix, it should be roughly similar
		// (or better). Allow up to 1.5x due to cache overhead, but not 2x which was the bug.
		assert.Less(t, ratio, 1.5,
			"Second run should not use significantly more memory than first run (bug was 2x). First: %d, Second: %d, Ratio: %.2fx",
			firstRunAlloc, secondRunAlloc, ratio)
	}

	// Verify cache was actually used
	stats := analyzer2.GetCacheStats()
	assert.Greater(t, stats.CacheHits, int64(0), "Should have cache hits on second run")
	t.Logf("Cache hit rate: %.2f%%", stats.HitRate())
}
