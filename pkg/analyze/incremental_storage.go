package analyze

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/pkg/errors"
)

func init() {
	gob.RegisterName("analyze.IncrementalDirMetadata", &IncrementalDirMetadata{})
	gob.RegisterName("analyze.FileMetadata", &FileMetadata{})
}

// IncrementalDirMetadata contains cached directory metadata
type IncrementalDirMetadata struct {
	Path         string         // Full path to directory
	Mtime        time.Time      // Directory modification time
	Size         int64          // Total apparent size
	Usage        int64          // Total disk usage
	ItemCount    int            // Number of items in tree
	Flag         rune           // Directory flag
	Files        []FileMetadata // Direct children metadata
	CachedAt     time.Time      // When this was cached
	ScanDuration time.Duration  // How long the scan took
}

// FileMetadata contains metadata for a single file or directory
type FileMetadata struct {
	Name  string    // File name
	IsDir bool      // Whether this is a directory
	Size  int64     // Apparent size
	Usage int64     // Disk usage
	Mtime time.Time // Modification time
	Flag  rune      // File flag
	Mli   uint64    // Multi-linked inode (for hardlinks)
}

// IncrementalStorage manages BadgerDB storage for incremental caching
type IncrementalStorage struct {
	db          *badger.DB
	storagePath string
	topDir      string
	m           sync.RWMutex
	counter     int
	counterM    sync.Mutex
}

// NewIncrementalStorage creates a new incremental storage instance
func NewIncrementalStorage(storagePath, topDir string) *IncrementalStorage {
	return &IncrementalStorage{
		storagePath: storagePath,
		topDir:      topDir,
	}
}

// GetTopDir returns the top directory
func (s *IncrementalStorage) GetTopDir() string {
	return s.topDir
}

// IsOpen returns true if BadgerDB is open
func (s *IncrementalStorage) IsOpen() bool {
	s.m.RLock()
	defer s.m.RUnlock()
	return s.db != nil
}

// Open opens the BadgerDB database with detailed error handling
func (s *IncrementalStorage) Open() (func(), error) {
	options := badger.DefaultOptions(s.storagePath)
	options.Logger = nil

	db, err := badger.Open(options)
	if err != nil {
		// Provide specific error messages for common issues
		errMsg := err.Error()

		// Permission denied
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied opening cache at %s: %w", s.storagePath, err)
		}

		// Disk space issues
		if strings.Contains(errMsg, "no space left") || strings.Contains(errMsg, "disk full") {
			return nil, fmt.Errorf("insufficient disk space for cache at %s: %w", s.storagePath, err)
		}

		// Database corruption or version mismatch
		if strings.Contains(errMsg, "corrupted") ||
			strings.Contains(errMsg, "invalid") ||
			strings.Contains(errMsg, "checksum") ||
			strings.Contains(errMsg, "manifest") {
			return nil, fmt.Errorf("cache database corrupted at %s (try deleting it with: rm -rf %s): %w",
				s.storagePath, s.storagePath, err)
		}

		// Concurrent access (another process using the database)
		if strings.Contains(errMsg, "Another process is using this Badger database") ||
			strings.Contains(errMsg, "Cannot acquire directory lock") ||
			strings.Contains(errMsg, "resource temporarily unavailable") {
			return nil, fmt.Errorf("cache database at %s is locked by another gdu process: %w",
				s.storagePath, err)
		}

		// Directory doesn't exist
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("cache directory does not exist at %s (create it with: mkdir -p %s): %w",
				s.storagePath, s.storagePath, err)
		}

		// Generic error with helpful context
		return nil, fmt.Errorf("failed to open cache database at %s: %w", s.storagePath, err)
	}

	s.db = db

	return func() {
		s.db.Close()
		s.db = nil
	}, nil
}

// StoreDirMetadata stores directory metadata in cache
func (s *IncrementalStorage) StoreDirMetadata(meta *IncrementalDirMetadata) error {
	s.checkCount()
	s.m.RLock()
	defer s.m.RUnlock()

	return s.db.Update(func(txn *badger.Txn) error {
		b := &bytes.Buffer{}
		enc := gob.NewEncoder(b)
		err := enc.Encode(meta)
		if err != nil {
			return errors.Wrap(err, "encoding directory metadata")
		}

		key := s.makeKey(meta.Path)
		return txn.Set(key, b.Bytes())
	})
}

// LoadDirMetadata loads directory metadata from cache with error handling
func (s *IncrementalStorage) LoadDirMetadata(path string) (*IncrementalDirMetadata, error) {
	s.checkCount()
	s.m.RLock()
	defer s.m.RUnlock()

	var meta IncrementalDirMetadata

	err := s.db.View(func(txn *badger.Txn) error {
		key := s.makeKey(path)
		item, err := txn.Get(key)
		if err != nil {
			return errors.Wrap(err, "reading cached metadata for path: "+path)
		}

		return item.Value(func(val []byte) error {
			b := bytes.NewBuffer(val)
			dec := gob.NewDecoder(b)
			decodeErr := dec.Decode(&meta)
			if decodeErr != nil {
				// Corrupted cache entry - wrap with context
				return fmt.Errorf("corrupted cache entry for %s (will rescan): %w", path, decodeErr)
			}
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	// Validate the loaded metadata
	if meta.Path == "" {
		return nil, fmt.Errorf("invalid cache entry for %s: empty path", path)
	}

	return &meta, nil
}

// DeleteDirMetadata removes directory metadata from cache
func (s *IncrementalStorage) DeleteDirMetadata(path string) error {
	s.m.RLock()
	defer s.m.RUnlock()

	return s.db.Update(func(txn *badger.Txn) error {
		key := s.makeKey(path)
		return txn.Delete(key)
	})
}

// makeKey creates a BadgerDB key for a given path
func (s *IncrementalStorage) makeKey(path string) []byte {
	return []byte(fmt.Sprintf("incr:%s", path))
}

// checkCount manages garbage collection based on operation count
func (s *IncrementalStorage) checkCount() {
	s.counterM.Lock()
	defer s.counterM.Unlock()

	s.counter++
	if s.counter%1000 == 0 {
		// Trigger value log GC periodically
		go func() {
			s.m.RLock()
			defer s.m.RUnlock()
			if s.db != nil {
				s.db.RunValueLogGC(0.5) //nolint:errcheck // GC errors in background task are not critical
			}
		}()
	}
}

// ClearCache removes all cached entries
func (s *IncrementalStorage) ClearCache() error {
	s.m.RLock()
	defer s.m.RUnlock()

	return s.db.DropAll()
}

// GetCacheSize returns the approximate size of the cache in bytes
func (s *IncrementalStorage) GetCacheSize() (int64, error) {
	s.m.RLock()
	defer s.m.RUnlock()

	lsm, vlog := s.db.Size()
	return lsm + vlog, nil
}
