package analyze

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestIncrementalAnalyzer_DirectoryMtimeDetection reproduces the exact bug scenario:
// 1. First scan creates cache
// 2. User adds new subdirectories
// 3. Second scan should detect mtime change and rescan
func TestIncrementalAnalyzer_DirectoryMtimeDetection(t *testing.T) {
	// Create test directory with 500 empty subdirectories
	testRoot := filepath.Join(t.TempDir(), "gdu-test-cache")
	err := os.Mkdir(testRoot, 0755)
	assert.NoError(t, err)

	// Create 500 initial directories
	for i := 1; i <= 500; i++ {
		dirPath := filepath.Join(testRoot, fmt.Sprintf("dir%d", i))
		err = os.Mkdir(dirPath, 0755)
		assert.NoError(t, err)
	}

	// Wait to ensure filesystem timestamp is stable
	time.Sleep(1100 * time.Millisecond)

	// First scan - populate cache
	tmpCache := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpCache,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	analyzer1 := CreateIncrementalAnalyzer(opts)
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
	t.Logf("First scan: Total dirs: %d, Misses: %d, Hits: %d, Rescanned: %d",
		stats1.TotalDirs, stats1.CacheMisses, stats1.CacheHits, stats1.DirsRescanned)

	// Verify initial state
	assert.Equal(t, 500, len(dir1.Files), "Should have 500 subdirectories")

	// Get the mtime of testRoot BEFORE modification
	statBefore, err := os.Stat(testRoot)
	assert.NoError(t, err)
	mtimeBefore := statBefore.ModTime()
	t.Logf("testRoot mtime before modification: %v", mtimeBefore)

	// Wait to ensure filesystem mtime granularity is exceeded
	time.Sleep(1100 * time.Millisecond)

	// Add two new directories (simulating user action: mkdir dir501 dir502)
	err = os.Mkdir(filepath.Join(testRoot, "dir501"), 0755)
	assert.NoError(t, err)
	err = os.Mkdir(filepath.Join(testRoot, "dir502"), 0755)
	assert.NoError(t, err)

	// Force filesystem sync
	time.Sleep(100 * time.Millisecond)

	// Verify mtime changed
	statAfter, err := os.Stat(testRoot)
	assert.NoError(t, err)
	mtimeAfter := statAfter.ModTime()
	t.Logf("testRoot mtime after modification: %v", mtimeAfter)
	assert.NotEqual(t, mtimeBefore, mtimeAfter, "Mtime should have changed after adding directories")

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
	t.Logf("Second scan: Total dirs: %d, Misses: %d, Hits: %d, Rescanned: %d, Hit Rate: %.1f%%",
		stats2.TotalDirs, stats2.CacheMisses, stats2.CacheHits, stats2.DirsRescanned, stats2.HitRate())

	// THE BUG: Cache stats show 100% hit rate even though mtime changed
	// Expected: DirsRescanned should be at least 1 (for the root directory)
	// Actual: DirsRescanned is 0, indicating cache was used despite mtime change

	// Verify new directories are detected
	assert.Equal(t, 502, len(dir2.Files), "Should now have 502 subdirectories (500 + 2 new)")

	// Verify cache correctly detected the change
	assert.Greater(t, stats2.DirsRescanned, int64(0),
		"Should have rescanned at least the root directory (mtime changed)")
	assert.Less(t, stats2.HitRate(), 100.0,
		"Hit rate should be <100%% because root directory was modified")
}

// TestMtimeComparisonPrecision tests if there's a time precision issue
func TestMtimeComparisonPrecision(t *testing.T) {
	testDir := filepath.Join(t.TempDir(), "test-precision")
	err := os.Mkdir(testDir, 0755)
	assert.NoError(t, err)

	// Get initial mtime
	stat1, err := os.Stat(testDir)
	assert.NoError(t, err)
	mtime1 := stat1.ModTime()
	t.Logf("Initial mtime: %v (Unix: %d, UnixNano: %d)", mtime1, mtime1.Unix(), mtime1.UnixNano())

	// Wait for filesystem granularity
	time.Sleep(1100 * time.Millisecond)

	// Modify directory
	subDir := filepath.Join(testDir, "subdir")
	err = os.Mkdir(subDir, 0755)
	assert.NoError(t, err)

	// Get new mtime
	stat2, err := os.Stat(testDir)
	assert.NoError(t, err)
	mtime2 := stat2.ModTime()
	t.Logf("After modification: %v (Unix: %d, UnixNano: %d)", mtime2, mtime2.Unix(), mtime2.UnixNano())

	// Test various comparison methods
	t.Logf("mtime1 == mtime2: %v", mtime1 == mtime2)
	t.Logf("mtime1.Equal(mtime2): %v", mtime1.Equal(mtime2))
	t.Logf("mtime1.Unix() == mtime2.Unix(): %v", mtime1.Unix() == mtime2.Unix())
	t.Logf("mtime1.UnixNano() == mtime2.UnixNano(): %v", mtime1.UnixNano() == mtime2.UnixNano())

	// The mtimes should be different
	assert.False(t, mtime1.Equal(mtime2), "Mtimes should be different after modification")
}
