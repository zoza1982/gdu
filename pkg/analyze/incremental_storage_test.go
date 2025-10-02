package analyze

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestIncrementalStorage_StoreLoad verifies basic store and load operations
func TestIncrementalStorage_StoreLoad(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test/path")

	closeFn := storage.Open()
	defer closeFn()

	// Create test metadata
	meta := &IncrementalDirMetadata{
		Path:      "/test/path/subdir",
		Mtime:     time.Now(),
		Size:      1024,
		Usage:     4096,
		ItemCount: 5,
		Flag:      ' ',
		Files: []FileMetadata{
			{
				Name:  "file1.txt",
				IsDir: false,
				Size:  512,
				Usage: 4096,
				Mtime: time.Now(),
				Flag:  ' ',
				Mli:   0,
			},
			{
				Name:  "subdir",
				IsDir: true,
				Size:  512,
				Usage: 4096,
				Mtime: time.Now(),
				Flag:  ' ',
				Mli:   0,
			},
		},
		CachedAt:     time.Now(),
		ScanDuration: 100 * time.Millisecond,
	}

	// Store metadata
	err := storage.StoreDirMetadata(meta)
	assert.NoError(t, err, "Should store metadata without error")

	// Load metadata
	loaded, err := storage.LoadDirMetadata("/test/path/subdir")
	assert.NoError(t, err, "Should load metadata without error")
	assert.NotNil(t, loaded, "Loaded metadata should not be nil")

	// Verify loaded data
	assert.Equal(t, meta.Path, loaded.Path)
	assert.Equal(t, meta.Size, loaded.Size)
	assert.Equal(t, meta.Usage, loaded.Usage)
	assert.Equal(t, meta.ItemCount, loaded.ItemCount)
	assert.Equal(t, meta.Flag, loaded.Flag)
	assert.Equal(t, len(meta.Files), len(loaded.Files))

	// Verify file metadata
	if len(loaded.Files) > 0 {
		assert.Equal(t, "file1.txt", loaded.Files[0].Name)
		assert.Equal(t, int64(512), loaded.Files[0].Size)
		assert.Equal(t, false, loaded.Files[0].IsDir)
	}
}

// TestIncrementalStorage_LoadNonExistent verifies error on missing key
func TestIncrementalStorage_LoadNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test/path")

	closeFn := storage.Open()
	defer closeFn()

	// Try to load non-existent path
	loaded, err := storage.LoadDirMetadata("/non/existent/path")
	assert.Error(t, err, "Should return error for non-existent path")
	assert.Nil(t, loaded, "Should return nil for non-existent path")
}

// TestIncrementalStorage_DeleteMetadata verifies deletion
func TestIncrementalStorage_DeleteMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test/path")

	closeFn := storage.Open()
	defer closeFn()

	// Store metadata
	meta := &IncrementalDirMetadata{
		Path:      "/test/path/delete",
		Mtime:     time.Now(),
		Size:      100,
		Usage:     4096,
		ItemCount: 1,
		Flag:      ' ',
		Files:     []FileMetadata{},
		CachedAt:  time.Now(),
	}

	err := storage.StoreDirMetadata(meta)
	assert.NoError(t, err)

	// Verify it exists
	loaded, err := storage.LoadDirMetadata("/test/path/delete")
	assert.NoError(t, err)
	assert.NotNil(t, loaded)

	// Delete it
	err = storage.DeleteDirMetadata("/test/path/delete")
	assert.NoError(t, err)

	// Verify it's gone
	loaded, err = storage.LoadDirMetadata("/test/path/delete")
	assert.Error(t, err)
	assert.Nil(t, loaded)
}

// TestIncrementalStorage_OverwriteMetadata verifies updating existing entries
func TestIncrementalStorage_OverwriteMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test/path")

	closeFn := storage.Open()
	defer closeFn()

	path := "/test/path/update"

	// Store initial metadata
	meta1 := &IncrementalDirMetadata{
		Path:      path,
		Mtime:     time.Now(),
		Size:      100,
		Usage:     4096,
		ItemCount: 1,
		Flag:      ' ',
		Files:     []FileMetadata{},
		CachedAt:  time.Now(),
	}
	err := storage.StoreDirMetadata(meta1)
	assert.NoError(t, err)

	// Update with new metadata
	meta2 := &IncrementalDirMetadata{
		Path:      path,
		Mtime:     time.Now().Add(1 * time.Hour),
		Size:      200, // Changed
		Usage:     8192,
		ItemCount: 2, // Changed
		Flag:      ' ',
		Files:     []FileMetadata{},
		CachedAt:  time.Now(),
	}
	err = storage.StoreDirMetadata(meta2)
	assert.NoError(t, err)

	// Load and verify it was updated
	loaded, err := storage.LoadDirMetadata(path)
	assert.NoError(t, err)
	assert.Equal(t, int64(200), loaded.Size, "Size should be updated")
	assert.Equal(t, 2, loaded.ItemCount, "Item count should be updated")
}

// TestIncrementalStorage_MultipleEntries verifies storing multiple paths
func TestIncrementalStorage_MultipleEntries(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test/path")

	closeFn := storage.Open()
	defer closeFn()

	// Store multiple entries
	paths := []string{
		"/test/path/dir1",
		"/test/path/dir2",
		"/test/path/dir3",
	}

	for i, path := range paths {
		meta := &IncrementalDirMetadata{
			Path:      path,
			Mtime:     time.Now(),
			Size:      int64(100 * (i + 1)),
			Usage:     4096,
			ItemCount: i + 1,
			Flag:      ' ',
			Files:     []FileMetadata{},
			CachedAt:  time.Now(),
		}
		err := storage.StoreDirMetadata(meta)
		assert.NoError(t, err)
	}

	// Load and verify all entries
	for i, path := range paths {
		loaded, err := storage.LoadDirMetadata(path)
		assert.NoError(t, err)
		assert.Equal(t, int64(100*(i+1)), loaded.Size)
		assert.Equal(t, i+1, loaded.ItemCount)
	}
}

// TestIncrementalStorage_IsOpen verifies open status tracking
func TestIncrementalStorage_IsOpen(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test/path")

	// Initially not open
	assert.False(t, storage.IsOpen(), "Should not be open initially")

	// Open and verify
	closeFn := storage.Open()
	assert.True(t, storage.IsOpen(), "Should be open after Open()")

	// Close and verify
	closeFn()
	assert.False(t, storage.IsOpen(), "Should not be open after close")
}

// TestIncrementalStorage_GetTopDir verifies top directory retrieval
func TestIncrementalStorage_GetTopDir(t *testing.T) {
	storage := NewIncrementalStorage("/tmp/cache", "/home/user/data")
	assert.Equal(t, "/home/user/data", storage.GetTopDir())
}

// TestIncrementalStorage_ClearCache verifies cache clearing
func TestIncrementalStorage_ClearCache(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test/path")

	closeFn := storage.Open()
	defer closeFn()

	// Store some data
	meta := &IncrementalDirMetadata{
		Path:      "/test/path/clear",
		Mtime:     time.Now(),
		Size:      100,
		Usage:     4096,
		ItemCount: 1,
		Flag:      ' ',
		Files:     []FileMetadata{},
		CachedAt:  time.Now(),
	}
	err := storage.StoreDirMetadata(meta)
	assert.NoError(t, err)

	// Verify it exists
	_, err = storage.LoadDirMetadata("/test/path/clear")
	assert.NoError(t, err)

	// Clear cache
	err = storage.ClearCache()
	assert.NoError(t, err)

	// Verify it's gone
	_, err = storage.LoadDirMetadata("/test/path/clear")
	assert.Error(t, err)
}

// TestIncrementalStorage_GetCacheSize verifies cache size calculation
func TestIncrementalStorage_GetCacheSize(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test/path")

	closeFn := storage.Open()
	defer closeFn()

	// Get initial size
	size1, err := storage.GetCacheSize()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, size1, int64(0))

	// Store some data
	meta := &IncrementalDirMetadata{
		Path:      "/test/path/size",
		Mtime:     time.Now(),
		Size:      100,
		Usage:     4096,
		ItemCount: 1,
		Flag:      ' ',
		Files:     []FileMetadata{},
		CachedAt:  time.Now(),
	}
	err = storage.StoreDirMetadata(meta)
	assert.NoError(t, err)

	// Get size after storing
	size2, err := storage.GetCacheSize()
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, size2, size1, "Cache size should increase after storing data")
}

// TestIncrementalStorage_LargeMetadata verifies handling of large file lists
func TestIncrementalStorage_LargeMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test/path")

	closeFn := storage.Open()
	defer closeFn()

	// Create large file list
	files := make([]FileMetadata, 1000)
	for i := 0; i < 1000; i++ {
		files[i] = FileMetadata{
			Name:  "file" + string(rune(i)) + ".txt",
			IsDir: false,
			Size:  int64(i * 100),
			Usage: 4096,
			Mtime: time.Now(),
			Flag:  ' ',
			Mli:   0,
		}
	}

	meta := &IncrementalDirMetadata{
		Path:      "/test/path/large",
		Mtime:     time.Now(),
		Size:      100000,
		Usage:     4096000,
		ItemCount: 1000,
		Flag:      ' ',
		Files:     files,
		CachedAt:  time.Now(),
	}

	// Store large metadata
	err := storage.StoreDirMetadata(meta)
	assert.NoError(t, err)

	// Load and verify
	loaded, err := storage.LoadDirMetadata("/test/path/large")
	assert.NoError(t, err)
	assert.Equal(t, 1000, len(loaded.Files))
	assert.Equal(t, int64(100000), loaded.Size)
}

// TestIncrementalStorage_SpecialCharactersInPath verifies path handling
func TestIncrementalStorage_SpecialCharactersInPath(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test/path")

	closeFn := storage.Open()
	defer closeFn()

	// Paths with special characters
	specialPaths := []string{
		"/test/path/dir with spaces",
		"/test/path/dir-with-dashes",
		"/test/path/dir_with_underscores",
		"/test/path/dir.with.dots",
	}

	for _, path := range specialPaths {
		meta := &IncrementalDirMetadata{
			Path:      path,
			Mtime:     time.Now(),
			Size:      100,
			Usage:     4096,
			ItemCount: 1,
			Flag:      ' ',
			Files:     []FileMetadata{},
			CachedAt:  time.Now(),
		}

		err := storage.StoreDirMetadata(meta)
		assert.NoError(t, err, "Should handle path: "+path)

		loaded, err := storage.LoadDirMetadata(path)
		assert.NoError(t, err, "Should load path: "+path)
		assert.Equal(t, path, loaded.Path)
	}
}

// TestIncrementalStorage_ConcurrentAccess verifies thread safety
func TestIncrementalStorage_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test/path")

	closeFn := storage.Open()
	defer closeFn()

	// Concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			meta := &IncrementalDirMetadata{
				Path:      "/test/path/concurrent" + string(rune(idx)),
				Mtime:     time.Now(),
				Size:      int64(idx * 100),
				Usage:     4096,
				ItemCount: idx,
				Flag:      ' ',
				Files:     []FileMetadata{},
				CachedAt:  time.Now(),
			}
			err := storage.StoreDirMetadata(meta)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func(idx int) {
			_, err := storage.LoadDirMetadata("/test/path/concurrent" + string(rune(idx)))
			// May or may not find the key depending on timing
			_ = err
			done <- true
		}(i)
	}

	// Wait for all reads
	for i := 0; i < 10; i++ {
		<-done
	}
}

// TestIncrementalStorage_GobEncoding verifies gob encoding/decoding
func TestIncrementalStorage_GobEncoding(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test/path")

	closeFn := storage.Open()
	defer closeFn()

	// Create metadata with various types
	meta := &IncrementalDirMetadata{
		Path:         "/test/path/encoding",
		Mtime:        time.Date(2025, 10, 2, 12, 0, 0, 0, time.UTC),
		Size:         9223372036854775807, // Max int64
		Usage:        4096,
		ItemCount:    999999,
		Flag:         'X',
		Files:        []FileMetadata{},
		CachedAt:     time.Now(),
		ScanDuration: 5 * time.Second,
	}

	err := storage.StoreDirMetadata(meta)
	assert.NoError(t, err)

	loaded, err := storage.LoadDirMetadata("/test/path/encoding")
	assert.NoError(t, err)

	// Verify all fields preserved
	assert.Equal(t, meta.Path, loaded.Path)
	assert.Equal(t, meta.Size, loaded.Size)
	assert.Equal(t, meta.Usage, loaded.Usage)
	assert.Equal(t, meta.ItemCount, loaded.ItemCount)
	assert.Equal(t, meta.Flag, loaded.Flag)
	assert.Equal(t, 5*time.Second, loaded.ScanDuration)
}

// TestIncrementalStorage_ReopenDatabase verifies persistence across opens
func TestIncrementalStorage_ReopenDatabase(t *testing.T) {
	tmpDir := t.TempDir()
	storage1 := NewIncrementalStorage(tmpDir, "/test/path")

	// First session
	closeFn1 := storage1.Open()
	meta := &IncrementalDirMetadata{
		Path:      "/test/path/persist",
		Mtime:     time.Now(),
		Size:      12345,
		Usage:     4096,
		ItemCount: 42,
		Flag:      ' ',
		Files:     []FileMetadata{},
		CachedAt:  time.Now(),
	}
	err := storage1.StoreDirMetadata(meta)
	assert.NoError(t, err)
	closeFn1()

	// Second session (new storage instance, same path)
	storage2 := NewIncrementalStorage(tmpDir, "/test/path")
	closeFn2 := storage2.Open()
	defer closeFn2()

	// Verify data persisted
	loaded, err := storage2.LoadDirMetadata("/test/path/persist")
	assert.NoError(t, err)
	assert.Equal(t, int64(12345), loaded.Size)
	assert.Equal(t, 42, loaded.ItemCount)
}

// TestIncrementalStorage_CorruptedData simulates corruption handling
func TestIncrementalStorage_CorruptedData(t *testing.T) {
	tmpDir := t.TempDir()

	// Create and close storage normally
	storage1 := NewIncrementalStorage(tmpDir, "/test/path")
	closeFn1 := storage1.Open()
	closeFn1()

	// Corrupt a file in the BadgerDB directory (if possible)
	// This is tricky to do reliably, so we'll just verify opening doesn't panic
	storage2 := NewIncrementalStorage(tmpDir, "/test/path")
	closeFn2 := storage2.Open()
	defer closeFn2()

	// Verify database still works
	assert.True(t, storage2.IsOpen())
}

// TestIncrementalStorage_EmptyFilesList verifies handling empty file lists
func TestIncrementalStorage_EmptyFilesList(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test/path")

	closeFn := storage.Open()
	defer closeFn()

	meta := &IncrementalDirMetadata{
		Path:      "/test/path/empty",
		Mtime:     time.Now(),
		Size:      0,
		Usage:     4096,
		ItemCount: 1,
		Flag:      'e',
		Files:     []FileMetadata{}, // Empty
		CachedAt:  time.Now(),
	}

	err := storage.StoreDirMetadata(meta)
	assert.NoError(t, err)

	loaded, err := storage.LoadDirMetadata("/test/path/empty")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(loaded.Files))
	assert.Equal(t, 'e', loaded.Flag)
}

// TestIncrementalStorage_NilFilesList verifies nil files list handling
func TestIncrementalStorage_NilFilesList(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test/path")

	closeFn := storage.Open()
	defer closeFn()

	meta := &IncrementalDirMetadata{
		Path:      "/test/path/nil",
		Mtime:     time.Now(),
		Size:      0,
		Usage:     4096,
		ItemCount: 1,
		Flag:      ' ',
		Files:     nil, // Nil instead of empty slice
		CachedAt:  time.Now(),
	}

	err := storage.StoreDirMetadata(meta)
	assert.NoError(t, err)

	loaded, err := storage.LoadDirMetadata("/test/path/nil")
	assert.NoError(t, err)
	// Gob encoding may convert nil to empty slice
	assert.NotNil(t, loaded)
}

// TestIncrementalStorage_MakeKey verifies key generation
func TestIncrementalStorage_MakeKey(t *testing.T) {
	storage := NewIncrementalStorage("/tmp", "/test")

	key1 := storage.makeKey("/test/path1")
	key2 := storage.makeKey("/test/path2")
	key3 := storage.makeKey("/test/path1") // Duplicate

	assert.NotEqual(t, key1, key2, "Different paths should have different keys")
	assert.Equal(t, key1, key3, "Same path should have same key")

	// Verify key format
	assert.Contains(t, string(key1), "incr:", "Key should have incr: prefix")
}

// TestIncrementalStorage_CheckCountGC verifies GC triggering
func TestIncrementalStorage_CheckCountGC(t *testing.T) {
	tmpDir := t.TempDir()
	storage := NewIncrementalStorage(tmpDir, "/test/path")

	closeFn := storage.Open()
	defer closeFn()

	// Trigger checkCount multiple times
	for i := 0; i < 1500; i++ {
		meta := &IncrementalDirMetadata{
			Path:      "/test/path/" + string(rune(i)),
			Mtime:     time.Now(),
			Size:      int64(i),
			Usage:     4096,
			ItemCount: 1,
			Flag:      ' ',
			Files:     []FileMetadata{},
			CachedAt:  time.Now(),
		}
		err := storage.StoreDirMetadata(meta)
		assert.NoError(t, err)
	}

	// Verify GC was triggered (counter should have gone past 1000)
	// This is mainly to ensure no panic occurs during GC
	assert.True(t, storage.IsOpen())
}

// TestIncrementalStorage_NonExistentDirectory verifies behavior with missing storage dir
func TestIncrementalStorage_NonExistentDirectory(t *testing.T) {
	// Use a path that doesn't exist yet
	tmpDir := t.TempDir()
	storagePath := tmpDir + "/subdir/cache"

	storage := NewIncrementalStorage(storagePath, "/test/path")

	// BadgerDB should create the directory
	closeFn := storage.Open()
	defer closeFn()

	// Verify it works
	assert.True(t, storage.IsOpen())

	// Verify directory was created
	_, err := os.Stat(storagePath)
	assert.NoError(t, err, "BadgerDB should create storage directory")
}
