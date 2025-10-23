package analyze

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestRealWorldScenario_CacheUpdateRace tests if there's a race where:
// - First scan caches old state
// - User modifies directory WHILE first scan is still running
// - Cache gets updated with NEW mtime but OLD contents
func TestRealWorldScenario_CacheUpdateRace(t *testing.T) {
	testRoot := filepath.Join(t.TempDir(), "test")
	err := os.Mkdir(testRoot, 0755)
	assert.NoError(t, err)

	// Create 5 initial dirs
	for i := 1; i <= 5; i++ {
		err = os.Mkdir(filepath.Join(testRoot, fmt.Sprintf("dir%d", i)), 0755)
		assert.NoError(t, err)
	}

	time.Sleep(1100 * time.Millisecond)

	tmpCache := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpCache,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	// First scan
	t.Log("=== FIRST SCAN ===")
	analyzer1 := CreateIncrementalAnalyzer(opts)
	dir1 := analyzer1.AnalyzeDir(testRoot, func(_, _ string) bool { return false }, false).(*Dir)
	analyzer1.GetDone().Wait()

	assert.Equal(t, 5, len(dir1.Files))

	// Completely close the first analyzer
	analyzer1 = nil

	// User modifies directory
	time.Sleep(1100 * time.Millisecond)
	t.Log("=== MODIFYING DIRECTORY ===")
	err = os.Mkdir(filepath.Join(testRoot, "dir6"), 0755)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Second scan - should detect change
	t.Log("=== SECOND SCAN ===")
	analyzer2 := CreateIncrementalAnalyzer(opts)
	dir2 := analyzer2.AnalyzeDir(testRoot, func(_, _ string) bool { return false }, false).(*Dir)
	analyzer2.GetDone().Wait()

	stats2 := analyzer2.GetCacheStats()
	t.Logf("Stats: Total=%d, Hits=%d, Misses=%d, Rescanned=%d, HitRate=%.1f%%",
		stats2.TotalDirs, stats2.CacheHits, stats2.CacheMisses, stats2.DirsRescanned, stats2.HitRate())

	assert.Equal(t, 6, len(dir2.Files), "Should find new directory")
	assert.Greater(t, stats2.DirsRescanned, int64(0), "Should rescan root due to mtime change")
}

// TestRealWorldScenario_EmptySubdirectories tests specifically with empty subdirectories
// like the user's scenario (dir501, dir502 are empty)
func TestRealWorldScenario_EmptySubdirectories(t *testing.T) {
	testRoot := filepath.Join(t.TempDir(), "test")
	err := os.Mkdir(testRoot, 0755)
	assert.NoError(t, err)

	// Create 20 empty subdirectories (like user's dir1..dir500)
	for i := 1; i <= 20; i++ {
		err = os.Mkdir(filepath.Join(testRoot, fmt.Sprintf("dir%d", i)), 0755)
		assert.NoError(t, err)
	}

	time.Sleep(1100 * time.Millisecond)

	tmpCache := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpCache,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	// First scan
	t.Log("=== FIRST SCAN ===")
	statBefore, _ := os.Stat(testRoot)
	t.Logf("Root mtime before first scan: %v", statBefore.ModTime())

	analyzer1 := CreateIncrementalAnalyzer(opts)
	dir1 := analyzer1.AnalyzeDir(testRoot, func(_, _ string) bool { return false }, false).(*Dir)
	analyzer1.GetDone().Wait()

	stats1 := analyzer1.GetCacheStats()
	t.Logf("First scan: Total=%d, Subdirs found=%d", stats1.TotalDirs, len(dir1.Files))
	assert.Equal(t, 20, len(dir1.Files))

	// User exits gdu, time passes
	time.Sleep(1100 * time.Millisecond)

	// User adds TWO new empty directories
	t.Log("=== USER ADDS DIRECTORIES ===")
	statBeforeMod, _ := os.Stat(testRoot)
	t.Logf("Root mtime before modification: %v", statBeforeMod.ModTime())

	time.Sleep(1100 * time.Millisecond)

	err = os.Mkdir(filepath.Join(testRoot, "dir21"), 0755)
	assert.NoError(t, err)
	err = os.Mkdir(filepath.Join(testRoot, "dir22"), 0755)
	assert.NoError(t, err)

	statAfterMod, _ := os.Stat(testRoot)
	t.Logf("Root mtime after modification: %v", statAfterMod.ModTime())
	t.Logf("Mtime changed: %v", !statBeforeMod.ModTime().Equal(statAfterMod.ModTime()))

	// Second scan
	t.Log("=== SECOND SCAN ===")
	analyzer2 := CreateIncrementalAnalyzer(opts)
	dir2 := analyzer2.AnalyzeDir(testRoot, func(_, _ string) bool { return false }, false).(*Dir)
	analyzer2.GetDone().Wait()

	stats2 := analyzer2.GetCacheStats()
	t.Logf("Second scan: Total=%d, Hits=%d, Misses=%d, Rescanned=%d, HitRate=%.1f%%",
		stats2.TotalDirs, stats2.CacheHits, stats2.CacheMisses, stats2.DirsRescanned, stats2.HitRate())
	t.Logf("Second scan: Subdirs found=%d", len(dir2.Files))

	// Verify behavior
	assert.Equal(t, 22, len(dir2.Files), "Should find all 22 directories")

	if stats2.DirsRescanned == 0 {
		t.Error("BUG REPRODUCED: Root directory not rescanned despite mtime change!")
		t.Errorf("  Mtime before: %v", statBeforeMod.ModTime())
		t.Errorf("  Mtime after:  %v", statAfterMod.ModTime())
		t.Errorf("  Stats: %d total, %d rescanned", stats2.TotalDirs, stats2.DirsRescanned)
	}

	assert.Greater(t, stats2.DirsRescanned, int64(0),
		"Root directory must be rescanned when mtime changes")
}
