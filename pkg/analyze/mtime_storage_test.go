package analyze

import (
	"bytes"
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestMtimeGobEncodingPrecision verifies that time precision is preserved through gob encoding
func TestMtimeGobEncodingPrecision(t *testing.T) {
	// Create a test directory
	testDir := filepath.Join(t.TempDir(), "test")
	err := os.Mkdir(testDir, 0755)
	assert.NoError(t, err)

	// Get its mtime
	stat, err := os.Stat(testDir)
	assert.NoError(t, err)
	originalMtime := stat.ModTime()

	t.Logf("Original mtime: %v", originalMtime)
	t.Logf("  Unix: %d", originalMtime.Unix())
	t.Logf("  UnixNano: %d", originalMtime.UnixNano())

	// Create metadata and encode it
	meta := &IncrementalDirMetadata{
		Path:  testDir,
		Mtime: originalMtime,
	}

	// Encode to gob
	b := &bytes.Buffer{}
	enc := gob.NewEncoder(b)
	err = enc.Encode(meta)
	assert.NoError(t, err)

	// Decode from gob
	var decodedMeta IncrementalDirMetadata
	dec := gob.NewDecoder(bytes.NewBuffer(b.Bytes()))
	err = dec.Decode(&decodedMeta)
	assert.NoError(t, err)

	t.Logf("Decoded mtime: %v", decodedMeta.Mtime)
	t.Logf("  Unix: %d", decodedMeta.Mtime.Unix())
	t.Logf("  UnixNano: %d", decodedMeta.Mtime.UnixNano())

	// Check if they're equal
	t.Logf("originalMtime.Equal(decodedMeta.Mtime): %v", originalMtime.Equal(decodedMeta.Mtime))
	t.Logf("originalMtime == decodedMeta.Mtime: %v", originalMtime == decodedMeta.Mtime)

	// The issue: time.Time includes timezone and monotonic clock reading
	// These might not be preserved through gob encoding
	assert.True(t, originalMtime.Equal(decodedMeta.Mtime), "Times should be equal after gob encoding")
}

// TestMtimeCachingRoundTrip verifies full cache storage round trip preserves mtime
func TestMtimeCachingRoundTrip(t *testing.T) {
	// Create test directory
	testDir := filepath.Join(t.TempDir(), "test")
	err := os.Mkdir(testDir, 0755)
	assert.NoError(t, err)

	// Get its mtime
	stat, err := os.Stat(testDir)
	assert.NoError(t, err)
	originalMtime := stat.ModTime()

	t.Logf("Original filesystem mtime: %v (Unix: %d, Nano: %d)",
		originalMtime, originalMtime.Unix(), originalMtime.UnixNano())

	// Create storage
	cacheDir := t.TempDir()
	storage := NewIncrementalStorage(cacheDir, testDir)
	closeFn, err := storage.Open()
	assert.NoError(t, err)
	defer closeFn()

	// Store metadata
	meta := &IncrementalDirMetadata{
		Path:      testDir,
		Mtime:     originalMtime,
		Size:      4096,
		Usage:     4096,
		ItemCount: 1,
		Files:     []FileMetadata{},
		CachedAt:  time.Now(),
	}

	err = storage.StoreDirMetadata(meta)
	assert.NoError(t, err)

	// Load it back
	loaded, err := storage.LoadDirMetadata(testDir)
	assert.NoError(t, err)

	t.Logf("Loaded cached mtime: %v (Unix: %d, Nano: %d)",
		loaded.Mtime, loaded.Mtime.Unix(), loaded.Mtime.UnixNano())

	// Compare
	t.Logf("originalMtime.Equal(loaded.Mtime): %v", originalMtime.Equal(loaded.Mtime))
	t.Logf("Difference in nanoseconds: %d", loaded.Mtime.UnixNano()-originalMtime.UnixNano())

	assert.True(t, originalMtime.Equal(loaded.Mtime),
		"Mtime should be equal after cache storage round trip")

	// Now wait and modify the directory
	time.Sleep(1100 * time.Millisecond)
	subDir := filepath.Join(testDir, "subdir")
	err = os.Mkdir(subDir, 0755)
	assert.NoError(t, err)

	// Get new mtime
	stat2, err := os.Stat(testDir)
	assert.NoError(t, err)
	newMtime := stat2.ModTime()

	t.Logf("New filesystem mtime after modification: %v (Unix: %d, Nano: %d)",
		newMtime, newMtime.Unix(), newMtime.UnixNano())
	t.Logf("cached.Mtime.Equal(newMtime): %v", loaded.Mtime.Equal(newMtime))
	t.Logf("!cached.Mtime.Equal(newMtime): %v", !loaded.Mtime.Equal(newMtime))

	// This should be false (mtimes are different)
	assert.False(t, loaded.Mtime.Equal(newMtime),
		"Cached mtime should NOT equal new mtime after modification")
}

// TestMtimeMonotonicClockStripping verifies if monotonic clock affects comparisons
func TestMtimeMonotonicClockStripping(t *testing.T) {
	// Create a time with monotonic clock reading
	now := time.Now()
	t.Logf("time.Now() with monotonic: %v", now)

	// Round trip through gob encoding (which strips monotonic clock)
	b := &bytes.Buffer{}
	enc := gob.NewEncoder(b)
	err := enc.Encode(now)
	assert.NoError(t, err)

	var decoded time.Time
	dec := gob.NewDecoder(bytes.NewBuffer(b.Bytes()))
	err = dec.Decode(&decoded)
	assert.NoError(t, err)

	t.Logf("After gob decode: %v", decoded)
	t.Logf("now.Equal(decoded): %v", now.Equal(decoded))

	// They should still be equal even if monotonic clock is stripped
	assert.True(t, now.Equal(decoded), "Times should be equal despite monotonic clock stripping")
}
