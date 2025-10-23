package analyze

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestRebuildFromCache_DetectsNewDirsViaFallback tests the scenario where:
// 1. Cache contains old state with 500 dirs
// 2. Filesystem now has 502 dirs (dir501, dir502 added)
// 3. rebuildFromCache() falls back to processDir() for missing children
// 4. But the PARENT directory's mtime should have changed, triggering rescan
//
// This test investigates WHY the user sees:
// - New directories appear in output (dir501, dir502)
// - But cache stats show 100% hit rate (0 rescanned)
func TestRebuildFromCache_DetectsNewDirsViaFallback(t *testing.T) {
	// Create test directory
	testRoot := filepath.Join(t.TempDir(), "test-root")
	err := os.Mkdir(testRoot, 0755)
	assert.NoError(t, err)

	// Create 5 initial directories (smaller for faster test)
	for i := 1; i <= 5; i++ {
		dirPath := filepath.Join(testRoot, fmt.Sprintf("dir%d", i))
		err = os.Mkdir(dirPath, 0755)
		assert.NoError(t, err)
	}

	// Wait for filesystem timestamp stability
	time.Sleep(1100 * time.Millisecond)

	// First scan - populate cache
	tmpCache := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpCache,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	analyzer1 := CreateIncrementalAnalyzer(opts)

	// Manually get mtime before first scan
	statBefore, err := os.Stat(testRoot)
	assert.NoError(t, err)
	mtimeBefore := statBefore.ModTime()
	t.Logf("BEFORE first scan: testRoot mtime = %v", mtimeBefore)

	dir1 := analyzer1.AnalyzeDir(testRoot, func(_, _ string) bool { return false }, false).(*Dir)

	// Drain progress
	doneChan1 := analyzer1.GetDone()
	progressChan1 := analyzer1.GetProgressChan()
	for {
		select {
		case <-progressChan1:
		case <-doneChan1:
			goto done1
		}
	}
done1:

	stats1 := analyzer1.GetCacheStats()
	t.Logf("First scan stats: Total=%d, Hits=%d, Misses=%d, Rescanned=%d",
		stats1.TotalDirs, stats1.CacheHits, stats1.CacheMisses, stats1.DirsRescanned)

	assert.Equal(t, 5, len(dir1.Files), "Should have 5 subdirectories initially")

	// Wait to ensure mtime will change
	time.Sleep(1100 * time.Millisecond)

	// Add TWO new directories
	err = os.Mkdir(filepath.Join(testRoot, "dir6"), 0755)
	assert.NoError(t, err)
	err = os.Mkdir(filepath.Join(testRoot, "dir7"), 0755)
	assert.NoError(t, err)

	// Force filesystem sync
	time.Sleep(100 * time.Millisecond)

	// Verify mtime changed
	statAfter, err := os.Stat(testRoot)
	assert.NoError(t, err)
	mtimeAfter := statAfter.ModTime()
	t.Logf("AFTER adding dirs: testRoot mtime = %v", mtimeAfter)
	t.Logf("Mtime changed: %v", !mtimeBefore.Equal(mtimeAfter))
	assert.False(t, mtimeBefore.Equal(mtimeAfter), "Mtime should have changed")

	// Second scan - should detect mtime change
	analyzer2 := CreateIncrementalAnalyzer(opts)
	dir2 := analyzer2.AnalyzeDir(testRoot, func(_, _ string) bool { return false }, false).(*Dir)

	// Drain progress
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

	stats2 := analyzer2.GetCacheStats()
	t.Logf("Second scan stats: Total=%d, Hits=%d, Misses=%d, Rescanned=%d, HitRate=%.1f%%",
		stats2.TotalDirs, stats2.CacheHits, stats2.CacheMisses, stats2.DirsRescanned, stats2.HitRate())

	// THE BUG CHECK:
	// Expected: Root directory mtime changed, so it should be rescanned
	// Actual (if bug exists): Root shows as cache hit despite mtime change
	//                         New dirs appear via fallback in rebuildFromCache()

	assert.Equal(t, 7, len(dir2.Files), "Should now have 7 subdirectories")

	// This is the key assertion - root directory should be rescanned
	assert.Greater(t, stats2.DirsRescanned, int64(0),
		"Root directory should be rescanned because mtime changed")
	assert.Less(t, stats2.HitRate(), 100.0,
		"Hit rate should be <100%% because root was modified")
}

// TestRebuildFromCache_FallbackIncrementsStats verifies that when rebuildFromCache
// calls processDir() for missing children, the stats are correctly incremented
func TestRebuildFromCache_FallbackIncrementsStats(t *testing.T) {
	// This test checks if the fallback path in rebuildFromCache() at line 470
	// correctly increments statistics when calling processDir()

	testRoot := filepath.Join(t.TempDir(), "test-root")
	err := os.Mkdir(testRoot, 0755)
	assert.NoError(t, err)

	// Create one subdirectory
	subDir := filepath.Join(testRoot, "subdir")
	err = os.Mkdir(subDir, 0755)
	assert.NoError(t, err)

	time.Sleep(1100 * time.Millisecond)

	// First scan
	tmpCache := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpCache,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	analyzer1 := CreateIncrementalAnalyzer(opts)
	analyzer1.AnalyzeDir(testRoot, func(_, _ string) bool { return false }, false)
	analyzer1.GetDone().Wait()

	// Manually delete the child's cache entry to force fallback
	storage := analyzer1.storage
	err = storage.DeleteDirMetadata(subDir)
	assert.NoError(t, err)
	t.Logf("Deleted cache entry for %s to force fallback", subDir)

	// Second scan - parent is in cache, child is not (will trigger fallback)
	analyzer2 := CreateIncrementalAnalyzer(opts)
	analyzer2.AnalyzeDir(testRoot, func(_, _ string) bool { return false }, false)
	analyzer2.GetDone().Wait()

	stats2 := analyzer2.GetCacheStats()
	t.Logf("Stats after fallback: Total=%d, Hits=%d, Misses=%d, Rescanned=%d",
		stats2.TotalDirs, stats2.CacheHits, stats2.CacheMisses, stats2.DirsRescanned)

	// The fallback to processDir() should increment cache misses
	assert.Greater(t, stats2.CacheMisses, int64(0),
		"Fallback to processDir() should register as cache miss")
}
