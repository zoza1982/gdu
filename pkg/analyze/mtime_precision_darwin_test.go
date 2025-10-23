// +build darwin

package analyze

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestDarwinMtimePrecision tests mtime precision on macOS (HFS+/APFS)
// APFS has nanosecond precision, HFS+ has 1-second precision
func TestDarwinMtimePrecision(t *testing.T) {
	testDir := filepath.Join(t.TempDir(), "test")
	err := os.Mkdir(testDir, 0755)
	assert.NoError(t, err)

	// Get initial mtime
	stat1, err := os.Stat(testDir)
	assert.NoError(t, err)
	mtime1 := stat1.ModTime()
	t.Logf("Initial mtime: %v", mtime1)
	t.Logf("  Unix seconds: %d", mtime1.Unix())
	t.Logf("  Nanoseconds:  %d", mtime1.Nanosecond())

	// Wait and modify
	time.Sleep(1100 * time.Millisecond)
	subDir := filepath.Join(testDir, "subdir")
	err = os.Mkdir(subDir, 0755)
	assert.NoError(t, err)

	// Get new mtime
	stat2, err := os.Stat(testDir)
	assert.NoError(t, err)
	mtime2 := stat2.ModTime()
	t.Logf("After modification: %v", mtime2)
	t.Logf("  Unix seconds: %d", mtime2.Unix())
	t.Logf("  Nanoseconds:  %d", mtime2.Nanosecond())

	// Verify they're different
	assert.NotEqual(t, mtime1.Unix(), mtime2.Unix(), "Unix seconds should differ")
}

// TestMacOSAPFS_MtimeAfterMkdir specifically tests if creating directories updates parent mtime
func TestMacOSAPFS_MtimeAfterMkdir(t *testing.T) {
	// Create test directory in /tmp
	testRoot := filepath.Join(t.TempDir(), "apfs-test")
	err := os.Mkdir(testRoot, 0755)
	assert.NoError(t, err)

	// Create 3 initial directories
	for i := 1; i <= 3; i++ {
		err = os.Mkdir(filepath.Join(testRoot, fmt.Sprintf("dir%d", i)), 0755)
		assert.NoError(t, err)
	}

	// Wait for filesystem timestamp to stabilize
	time.Sleep(1100 * time.Millisecond)

	// Get mtime BEFORE adding new directories
	statBefore, err := os.Stat(testRoot)
	assert.NoError(t, err)
	mtimeBefore := statBefore.ModTime()
	t.Logf("testRoot mtime BEFORE mkdir: %v (Unix: %d, Nano: %d)",
		mtimeBefore, mtimeBefore.Unix(), mtimeBefore.UnixNano())

	// Wait to ensure time has advanced
	time.Sleep(1100 * time.Millisecond)

	// Add new directory
	err = os.Mkdir(filepath.Join(testRoot, "dir4"), 0755)
	assert.NoError(t, err)

	// IMMEDIATELY check mtime AFTER
	statAfter, err := os.Stat(testRoot)
	assert.NoError(t, err)
	mtimeAfter := statAfter.ModTime()
	t.Logf("testRoot mtime AFTER mkdir:  %v (Unix: %d, Nano: %d)",
		mtimeAfter, mtimeAfter.Unix(), mtimeAfter.UnixNano())

	t.Logf("Difference in seconds: %d", mtimeAfter.Unix()-mtimeBefore.Unix())
	t.Logf("mtimeBefore.Equal(mtimeAfter): %v", mtimeBefore.Equal(mtimeAfter))
	t.Logf("!mtimeBefore.Equal(mtimeAfter): %v", !mtimeBefore.Equal(mtimeAfter))

	// On macOS, mkdir should update the parent directory's mtime
	assert.False(t, mtimeBefore.Equal(mtimeAfter),
		"Parent directory mtime should change after mkdir on macOS")
}

// TestExactUserScenario reproduces the EXACT user scenario on macOS
func TestExactUserScenario(t *testing.T) {
	// User scenario:
	// 1. Run gdu --incremental /tmp/gdu-test-cache (500 dirs)
	// 2. Exit
	// 3. mkdir /tmp/gdu-test-cache/dir501 /tmp/gdu-test-cache/dir502
	// 4. Run gdu --incremental --show-cache-stats /tmp/gdu-test-cache
	// Expected: mtime changed from 18:54 to 19:23:25, should rescan
	// Actual: Hit Rate 100%, 0 rescanned, BUT new dirs appear

	testRoot := filepath.Join(t.TempDir(), "gdu-test-cache")
	err := os.Mkdir(testRoot, 0755)
	assert.NoError(t, err)

	// Create 10 initial directories (smaller for faster test)
	for i := 1; i <= 10; i++ {
		err = os.Mkdir(filepath.Join(testRoot, fmt.Sprintf("dir%d", i)), 0755)
		assert.NoError(t, err)
	}

	// Wait for filesystem to stabilize
	time.Sleep(1100 * time.Millisecond)

	// FIRST SCAN
	tmpCache := t.TempDir()
	opts := IncrementalOptions{
		StoragePath:   tmpCache,
		CacheMaxAge:   0,
		ForceFullScan: false,
	}

	t.Log("=== FIRST SCAN ===")
	analyzer1 := CreateIncrementalAnalyzer(opts)

	// Get mtime just before first scan
	statBeforeScan1, _ := os.Stat(testRoot)
	t.Logf("testRoot mtime before first scan: %v", statBeforeScan1.ModTime())

	dir1 := analyzer1.AnalyzeDir(testRoot, func(_, _ string) bool { return false }, false).(*Dir)
	analyzer1.GetDone().Wait()

	stats1 := analyzer1.GetCacheStats()
	t.Logf("First scan: Total=%d, Hits=%d, Misses=%d, Rescanned=%d, HitRate=%.1f%%",
		stats1.TotalDirs, stats1.CacheHits, stats1.CacheMisses, stats1.DirsRescanned, stats1.HitRate())
	t.Logf("First scan: Found %d subdirectories", len(dir1.Files))

	// Get mtime after first scan (should be same)
	statAfterScan1, _ := os.Stat(testRoot)
	t.Logf("testRoot mtime after first scan: %v", statAfterScan1.ModTime())

	// "User exits gdu" - simulate by waiting
	time.Sleep(1100 * time.Millisecond)

	// Get mtime before modification
	statBeforeMod, _ := os.Stat(testRoot)
	mtimeBeforeMod := statBeforeMod.ModTime()
	t.Logf("\n=== BEFORE MODIFICATION ===")
	t.Logf("testRoot mtime: %v (Unix: %d, Nano: %d)",
		mtimeBeforeMod, mtimeBeforeMod.Unix(), mtimeBeforeMod.UnixNano())

	// Wait before modification (user scenario: time passes)
	time.Sleep(1100 * time.Millisecond)

	// "User adds two directories" - mkdir dir501 dir502
	t.Log("\n=== USER ADDS NEW DIRECTORIES ===")
	err = os.Mkdir(filepath.Join(testRoot, "dir11"), 0755)
	assert.NoError(t, err)
	err = os.Mkdir(filepath.Join(testRoot, "dir12"), 0755)
	assert.NoError(t, err)
	t.Log("Created dir11 and dir12")

	// Get mtime immediately after modification
	statAfterMod, _ := os.Stat(testRoot)
	mtimeAfterMod := statAfterMod.ModTime()
	t.Logf("testRoot mtime: %v (Unix: %d, Nano: %d)",
		mtimeAfterMod, mtimeAfterMod.Unix(), mtimeAfterMod.UnixNano())
	t.Logf("Mtime changed: %v", !mtimeBeforeMod.Equal(mtimeAfterMod))
	t.Logf("Unix seconds diff: %d", mtimeAfterMod.Unix()-mtimeBeforeMod.Unix())

	// Verify mtime actually changed
	assert.False(t, mtimeBeforeMod.Equal(mtimeAfterMod),
		"Mtime must change after adding directories")

	// SECOND SCAN
	t.Log("\n=== SECOND SCAN (after modification) ===")
	analyzer2 := CreateIncrementalAnalyzer(opts)

	// Get current filesystem mtime just before second scan
	statBeforeScan2, _ := os.Stat(testRoot)
	currentMtime := statBeforeScan2.ModTime()
	t.Logf("Current filesystem mtime: %v (Unix: %d, Nano: %d)",
		currentMtime, currentMtime.Unix(), currentMtime.UnixNano())

	dir2 := analyzer2.AnalyzeDir(testRoot, func(_, _ string) bool { return false }, false).(*Dir)
	analyzer2.GetDone().Wait()

	stats2 := analyzer2.GetCacheStats()
	t.Logf("Second scan: Total=%d, Hits=%d, Misses=%d, Rescanned=%d, HitRate=%.1f%%",
		stats2.TotalDirs, stats2.CacheHits, stats2.CacheMisses, stats2.DirsRescanned, stats2.HitRate())
	t.Logf("Second scan: Found %d subdirectories", len(dir2.Files))

	// THE BUG CHECK
	t.Log("\n=== BUG VERIFICATION ===")
	assert.Equal(t, 12, len(dir2.Files), "Should find 12 subdirectories (10 + 2 new)")

	if stats2.DirsRescanned == 0 && stats2.HitRate() == 100.0 {
		t.Error("BUG REPRODUCED!")
		t.Errorf("  Expected: Root directory rescanned (mtime changed)")
		t.Errorf("  Actual:   100%% cache hit, 0 directories rescanned")
		t.Errorf("  Yet new directories appeared in output!")
	} else {
		t.Logf("Bug NOT reproduced - mtime detection working correctly")
		t.Logf("  Rescanned: %d", stats2.DirsRescanned)
		t.Logf("  Hit Rate: %.1f%%", stats2.HitRate())
	}

	// Assert correct behavior
	assert.Greater(t, stats2.DirsRescanned, int64(0),
		"Root directory should be rescanned when mtime changes")
}
