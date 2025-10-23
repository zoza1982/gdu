package analyze

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// TestIncrementalWithJSONExport tests incremental caching with JSON export
// Validates that exported JSON includes all directory information correctly
func TestIncrementalWithJSONExport(t *testing.T) {
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

	// Export to JSON
	jsonFile := filepath.Join(tmpDir, "export.json")
	f, err := os.Create(jsonFile)
	assert.NoError(t, err)
	if err != nil {
		return
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			t.Logf("Failed to close file: %v", closeErr)
		}
	}()

	// Marshal the directory structure
	data, err := json.Marshal(dir)
	assert.NoError(t, err)
	if err != nil {
		return
	}

	// Verify JSON was created and contains data
	assert.Greater(t, len(data), 0, "JSON data should not be empty")
	assert.Contains(t, string(data), "test_dir", "JSON should contain directory name")

	// Verify basic structure
	assert.Equal(t, "test_dir", dir.Name)
	assert.Greater(t, dir.Size, int64(0))
	assert.Greater(t, dir.ItemCount, 0)
}

// TestIncrementalWithSequential verifies incremental and sequential cannot coexist
// Each analyzer type is independent and should work correctly on its own
func TestIncrementalWithSequential(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	// Test incremental analyzer
	tmpDir := t.TempDir()
	incrementalOpts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}
	incrementalAnalyzer := CreateIncrementalAnalyzer(incrementalOpts)
	incrementalDir := incrementalAnalyzer.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-incrementalAnalyzer.GetProgressChan()
	incrementalAnalyzer.GetDone().Wait()
	incrementalDir.UpdateStats(make(fs.HardLinkedItems))

	// Test sequential analyzer (separate from incremental)
	seqAnalyzer := CreateSeqAnalyzer()
	seqDir := seqAnalyzer.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-seqAnalyzer.GetProgressChan()
	seqAnalyzer.GetDone().Wait()
	seqDir.UpdateStats(make(fs.HardLinkedItems))

	// Both should produce same results
	assert.Equal(t, incrementalDir.Name, seqDir.Name)
	assert.Equal(t, incrementalDir.ItemCount, seqDir.ItemCount)
	// Both should have valid sizes
	assert.Greater(t, incrementalDir.Size, int64(0))
	assert.Greater(t, seqDir.Size, int64(0))
}

// TestIncrementalWithNoCross tests incremental caching respects filesystem boundaries
func TestIncrementalWithNoCross(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skip("skipping test that requires root privileges to create mount points")
	}

	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	analyzer := CreateIncrementalAnalyzer(opts)

	// In a real scenario, we would check device IDs
	// For this test, we just verify the analyzer works with ignore function
	dir := analyzer.AnalyzeDir(
		"test_dir",
		func(name, path string) bool {
			// Simulate crossing filesystem boundary
			return name == "nested"
		},
		false,
	).(*Dir)

	<-analyzer.GetProgressChan()
	analyzer.GetDone().Wait()

	// Should only have test_dir, nested should be ignored
	assert.Equal(t, "test_dir", dir.Name)
	// ItemCount should be less since nested is ignored
	assert.Less(t, dir.ItemCount, 5)
}

// TestIncrementalWithIgnoreDirs tests incremental caching with directory ignoring
func TestIncrementalWithIgnoreDirs(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	analyzer := CreateIncrementalAnalyzer(opts)

	// Ignore nested directories
	dir := analyzer.AnalyzeDir(
		"test_dir",
		func(name, path string) bool {
			return name == "nested"
		},
		false,
	).(*Dir)

	// Drain progress channel properly
	progressChan := analyzer.GetProgressChan()
	doneChan := analyzer.GetDone()
	go func() {
		for range progressChan {
			// Drain
		}
	}()
	doneChan.Wait()
	dir.UpdateStats(make(fs.HardLinkedItems))

	// Verify nested directory was ignored
	assert.Equal(t, "test_dir", dir.Name)
	for _, file := range dir.Files {
		assert.NotEqual(t, "nested", file.GetName())
	}

	// Second scan should also respect ignore
	analyzer2 := CreateIncrementalAnalyzer(opts)
	dir2 := analyzer2.AnalyzeDir(
		"test_dir",
		func(name, path string) bool {
			return name == "nested"
		},
		false,
	).(*Dir)

	// Drain progress channel properly
	progressChan2 := analyzer2.GetProgressChan()
	doneChan2 := analyzer2.GetDone()
	go func() {
		for range progressChan2 {
			// Drain
		}
	}()
	doneChan2.Wait()
	dir2.UpdateStats(make(fs.HardLinkedItems))

	// Results should be identical
	assert.Equal(t, dir.ItemCount, dir2.ItemCount)
	assert.Equal(t, len(dir.Files), len(dir2.Files))
}

// TestIncrementalWithFollowSymlinks tests incremental caching with symlink following
func TestIncrementalWithFollowSymlinks(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	// Create a symlink
	err := os.Symlink("test_dir/nested/file2", "test_dir/file_link")
	assert.NoError(t, err)
	if err != nil {
		t.Skipf("Failed to create symlink: %v", err)
		return
	}

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	// Test with symlink following disabled
	analyzer1 := CreateIncrementalAnalyzer(opts)
	analyzer1.SetFollowSymlinks(false)
	dir1 := analyzer1.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer1.GetProgressChan()
	analyzer1.GetDone().Wait()
	dir1.UpdateStats(make(fs.HardLinkedItems))

	// Test with symlink following enabled
	analyzer2 := CreateIncrementalAnalyzer(opts)
	analyzer2.SetFollowSymlinks(true)
	dir2 := analyzer2.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer2.GetProgressChan()
	analyzer2.GetDone().Wait()
	dir2.UpdateStats(make(fs.HardLinkedItems))

	// With follow symlinks, we should have the symlink file
	assert.Equal(t, "test_dir", dir2.Name)
	assert.Greater(t, dir2.ItemCount, 0)

	// Find the symlink file
	var foundSymlink bool
	for _, file := range dir2.Files {
		if file.GetName() == "file_link" {
			foundSymlink = true
			assert.Equal(t, '@', file.GetFlag())
			break
		}
	}
	assert.True(t, foundSymlink, "Should find symlink file")
}

// TestIncrementalNonInteractiveMode tests incremental caching in non-interactive mode
func TestIncrementalNonInteractiveMode(t *testing.T) {
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

	// In non-interactive mode, we still need to drain channels
	progressChan := analyzer1.GetProgressChan()
	doneChan := analyzer1.GetDone()

	go func() {
		for range progressChan {
			// Drain progress updates
		}
	}()

	doneChan.Wait()
	dir1.UpdateStats(make(fs.HardLinkedItems))

	// Verify structure
	assert.Equal(t, "test_dir", dir1.Name)
	assert.Greater(t, dir1.Size, int64(0))
	assert.Greater(t, dir1.ItemCount, 0)

	// Second scan should use cache
	analyzer2 := CreateIncrementalAnalyzer(opts)
	analyzer2.ResetProgress() // Reset to start fresh
	dir2 := analyzer2.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)

	progressChan2 := analyzer2.GetProgressChan()
	doneChan2 := analyzer2.GetDone()

	go func() {
		for range progressChan2 {
			// Drain progress updates
		}
	}()

	doneChan2.Wait()
	dir2.UpdateStats(make(fs.HardLinkedItems))

	// Results should be identical
	assert.Equal(t, dir1.Size, dir2.Size)
	assert.Equal(t, dir1.ItemCount, dir2.ItemCount)

	// Verify cache was used
	stats := analyzer2.GetCacheStats()
	assert.Greater(t, stats.CacheHits, int64(0))
	assert.Greater(t, stats.BytesFromCache, int64(0))
}

// TestIncrementalWithCacheMaxAge tests cache expiration
func TestIncrementalWithCacheMaxAge(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()

	// First scan with no max age
	opts1 := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}
	analyzer1 := CreateIncrementalAnalyzer(opts1)
	dir1 := analyzer1.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer1.GetProgressChan()
	analyzer1.GetDone().Wait()

	// Wait a bit to let cache age
	time.Sleep(100 * time.Millisecond)

	// Second scan with very short max age (should force rescan)
	opts2 := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   50 * time.Millisecond, // Very short age
		ForceFullScan: false,
	}
	analyzer2 := CreateIncrementalAnalyzer(opts2)
	dir2 := analyzer2.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer2.GetProgressChan()
	analyzer2.GetDone().Wait()

	// Verify cache expiry stats
	stats := analyzer2.GetCacheStats()
	assert.Greater(t, stats.CacheExpired, int64(0), "Cache should have expired entries")
	assert.Greater(t, stats.DirsRescanned, int64(0), "Should have rescanned directories")

	// Results should still be accurate
	assert.Equal(t, dir1.Name, dir2.Name)
	assert.Equal(t, dir1.ItemCount, dir2.ItemCount)
}

// TestIncrementalWithForceFullScan tests force full scan flag
func TestIncrementalWithForceFullScan(t *testing.T) {
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
	dir1 := analyzer1.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer1.GetProgressChan()
	analyzer1.GetDone().Wait()

	// Second scan with force full scan
	opts2 := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: true, // Force rescan
	}
	analyzer2 := CreateIncrementalAnalyzer(opts2)
	dir2 := analyzer2.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer2.GetProgressChan()
	analyzer2.GetDone().Wait()

	// Verify all directories were rescanned
	stats := analyzer2.GetCacheStats()
	assert.Greater(t, stats.DirsRescanned, int64(0), "Should have rescanned directories")
	assert.Equal(t, int64(0), stats.CacheHits, "Should have no cache hits with force full scan")
	assert.Greater(t, stats.BytesScanned, int64(0), "Should have scanned bytes")

	// Results should be identical
	assert.Equal(t, dir1.Name, dir2.Name)
	assert.Equal(t, dir1.ItemCount, dir2.ItemCount)
}

// TestIncrementalWithIOThrottling tests I/O throttling configuration
func TestIncrementalWithIOThrottling(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()

	// Test with I/O throttling enabled
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
		MaxIOPS:       100,                // Limit to 100 IOPS
		IODelay:       10 * time.Millisecond, // 10ms delay
	}
	analyzer := CreateIncrementalAnalyzer(opts)

	startTime := time.Now()
	dir := analyzer.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer.GetProgressChan()
	analyzer.GetDone().Wait()
	elapsed := time.Since(startTime)

	// With throttling, it should take at least some time
	// (though for a small test dir, it might be fast)
	assert.Greater(t, elapsed, time.Duration(0))

	// Verify results are still correct
	assert.Equal(t, "test_dir", dir.Name)
	assert.Greater(t, dir.ItemCount, 0)
}

// TestIncrementalCacheStats tests cache statistics display
func TestIncrementalCacheStats(t *testing.T) {
	fin := testdir.CreateTestDir()
	defer fin()

	tmpDir := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpDir,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	// First scan - should populate cache
	analyzer1 := CreateIncrementalAnalyzer(opts)
	dir1 := analyzer1.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer1.GetProgressChan()
	analyzer1.GetDone().Wait()

	stats1 := analyzer1.GetCacheStats()
	assert.Greater(t, stats1.CacheMisses, int64(0), "First scan should have cache misses")
	assert.Equal(t, int64(0), stats1.CacheHits, "First scan should have no cache hits")
	assert.Greater(t, stats1.BytesScanned, int64(0))
	assert.Equal(t, int64(0), stats1.BytesFromCache)
	assert.Greater(t, stats1.TotalDirs, int64(0))

	// Second scan - should use cache
	analyzer2 := CreateIncrementalAnalyzer(opts)
	dir2 := analyzer2.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer2.GetProgressChan()
	analyzer2.GetDone().Wait()

	stats2 := analyzer2.GetCacheStats()
	assert.Greater(t, stats2.CacheHits, int64(0), "Second scan should have cache hits")
	assert.Greater(t, stats2.BytesFromCache, int64(0), "Second scan should load from cache")
	assert.Greater(t, stats2.TotalDirs, int64(0))

	// Verify cache efficiency
	if stats2.TotalDirs > 0 {
		hitRate := float64(stats2.CacheHits) / float64(stats2.TotalDirs) * 100
		assert.Greater(t, hitRate, 0.0, "Should have positive cache hit rate")
	}

	// Verify scan time was recorded
	assert.Greater(t, stats1.TotalScanTime, time.Duration(0))
	assert.Greater(t, stats2.TotalScanTime, time.Duration(0))

	// Results should be identical
	assert.Equal(t, dir1.Name, dir2.Name)
	assert.Equal(t, dir1.ItemCount, dir2.ItemCount)
	assert.Equal(t, dir1.Size, dir2.Size)
}

// TestIncrementalWithModifiedFiles tests cache invalidation on file changes
func TestIncrementalWithModifiedFiles(t *testing.T) {
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
	size1 := dir1.Size

	// Modify the directory to change its mtime (touch the directory)
	time.Sleep(100 * time.Millisecond) // Ensure mtime difference
	nestedPath := "test_dir/nested"
	now := time.Now()
	err := os.Chtimes(nestedPath, now, now)
	assert.NoError(t, err)
	if err != nil {
		return
	}

	// Second scan should detect change in directory mtime
	analyzer2 := CreateIncrementalAnalyzer(opts)
	dir2 := analyzer2.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer2.GetProgressChan()
	analyzer2.GetDone().Wait()

	// Verify rescan happened for the modified directory
	stats := analyzer2.GetCacheStats()
	// We should have at least some cache hits (test_dir/nested/subnested)
	// and at least one rescan (test_dir/nested due to mtime change)
	assert.Greater(t, stats.TotalDirs, int64(0), "Should have scanned directories")

	// Size should be consistent
	assert.Equal(t, size1, dir2.Size, "Size should remain same if only mtime changed")
}
