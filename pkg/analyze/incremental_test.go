package analyze

import (
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
	analyzer.ResetProgress()
	dir.UpdateStats(make(fs.HardLinkedItems))

	// Verify directory structure
	assert.Equal(t, "test_dir", dir.Name)
	assert.Equal(t, int64(7+4096*3), dir.Size)
	assert.Equal(t, 5, dir.ItemCount)
	assert.True(t, dir.IsDir())

	// Verify nested structure
	assert.Equal(t, "nested", dir.Files[0].GetName())
	assert.Equal(t, "subnested", dir.Files[0].(*Dir).Files[1].GetName())

	// Verify statistics - first scan should have all cache misses
	stats := analyzer.GetCacheStats()
	assert.Greater(t, stats.CacheMisses, int64(0), "First scan should have cache misses")
	assert.Equal(t, int64(0), stats.CacheHits, "First scan should have no cache hits")
	assert.Greater(t, stats.BytesScanned, int64(0), "First scan should scan bytes")
	assert.Equal(t, int64(0), stats.BytesFromCache, "First scan should not load from cache")
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
	<-analyzer2.GetProgressChan()
	analyzer2.GetDone().Wait()
	analyzer2.ResetProgress()
	dir2.UpdateStats(make(fs.HardLinkedItems))

	// Verify results match
	assert.Equal(t, firstScanSize, dir2.Size, "Size should match cached value")
	assert.Equal(t, firstScanCount, dir2.ItemCount, "Item count should match cached value")

	// Verify cache statistics
	stats := analyzer2.GetCacheStats()
	assert.Greater(t, stats.CacheHits, int64(0), "Second scan should have cache hits")
	assert.Greater(t, stats.BytesFromCache, int64(0), "Should load bytes from cache")

	// Calculate hit rate
	hitRate := stats.HitRate()
	assert.Greater(t, hitRate, 90.0, "Hit rate should be >90% for unchanged directory")
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
	time.Sleep(10 * time.Millisecond) // Ensure time difference
	err := os.WriteFile("test_dir/nested/newfile.txt", []byte("new content"), 0o644)
	assert.NoError(t, err)

	// Second scan should detect change
	analyzer2 := CreateIncrementalAnalyzer(opts)
	dir2 := analyzer2.AnalyzeDir(
		"test_dir", func(_, _ string) bool { return false }, false,
	).(*Dir)
	<-analyzer2.GetProgressChan()
	analyzer2.GetDone().Wait()
	analyzer2.ResetProgress()
	dir2.UpdateStats(make(fs.HardLinkedItems))

	// Verify that the modified directory was rescanned
	stats := analyzer2.GetCacheStats()
	assert.Greater(t, stats.DirsRescanned, int64(0), "Should have rescanned modified directories")
	assert.Greater(t, stats.BytesScanned, int64(0), "Should have scanned new bytes")

	// Verify new file is detected
	assert.Greater(t, dir2.ItemCount, dir1.ItemCount, "Should detect new file")
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
	analyzer2.ResetProgress()

	// Verify cache was expired
	stats := analyzer2.GetCacheStats()
	assert.Greater(t, stats.CacheExpired, int64(0), "Cache should have expired entries")
	assert.Greater(t, stats.DirsRescanned, int64(0), "Should rescan expired directories")
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
	analyzer2.ResetProgress()

	// Verify cache was bypassed
	stats := analyzer2.GetCacheStats()
	assert.Greater(t, stats.DirsRescanned, int64(0), "Should rescan all directories")
	assert.Equal(t, int64(0), stats.CacheHits, "Should not use cache with force full scan")
	assert.Greater(t, stats.BytesScanned, int64(0), "Should scan all bytes")
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

	<-analyzer.GetProgressChan()
	analyzer.GetDone().Wait()
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
	analyzer2.ResetProgress()

	stats2 := analyzer2.GetCacheStats()
	assert.Greater(t, stats2.HitRate(), 0.0, "Second scan should have >0% hit rate")
	assert.Greater(t, stats2.IOReduction(), 0.0, "Second scan should have >0% I/O reduction")
	assert.Greater(t, stats2.TotalScanTime, time.Duration(0), "Should track scan time")
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
