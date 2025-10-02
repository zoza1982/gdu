# Incremental Caching Implementation Plan for gdu

## Executive Summary

This document provides a comprehensive implementation plan for adding incremental caching to gdu (Go Disk Usage) to solve performance issues when scanning NFS/DDN storage systems. The feature will reduce I/O operations by 90%+ for daily scans by only re-scanning directories that have changed based on modification time (mtime) comparison.

**Target Use Case**: Daily disk usage analysis on large NFS/DDN storage without overloading the storage system.

**Estimated Timeline**: 5 weeks for complete implementation including tests and documentation.

---

## 1. Feature Overview

### What We're Building

Incremental caching for gdu that stores directory metadata (mtime, size, item count) in persistent storage and only re-scans directories that have changed since the last analysis. This dramatically reduces I/O operations on slow network filesystems (NFS, DDN) by reusing cached data for unchanged directories.

### Why We Need It

- **Current Problem**: Every gdu scan traverses all files/directories, generating massive I/O on network storage
- **Impact**: Daily scans on large NFS volumes can overload storage systems and take hours
- **Solution**: Only scan changed directories based on mtime comparison, reusing cached data for unchanged paths

### Key Benefits

- 80-95% reduction in I/O for typical daily scans (only changed data is re-scanned)
- Configurable cache expiry to ensure freshness
- Optional I/O throttling for storage-friendly operations
- Maintains full compatibility with existing gdu features

---

## 2. Technical Design

### Architecture Overview

We'll create a new analyzer type `IncrementalAnalyzer` that extends the existing BadgerDB storage backend with mtime-based validation and incremental updates.

```
┌─────────────────────────────────────────────────────┐
│  User runs: gdu --incremental /path                 │
└─────────────────┬───────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────┐
│  IncrementalAnalyzer                                │
│  - Checks BadgerDB for cached directory data        │
│  - Compares current mtime with cached mtime         │
│  - Decision:                                        │
│    • mtime unchanged → use cache                    │
│    • mtime changed → re-scan directory              │
│    • not in cache → full scan                       │
└─────────────────┬───────────────────────────────────┘
                  │
                  ▼
┌─────────────────────────────────────────────────────┐
│  Cache Storage (BadgerDB Extended)                  │
│  Key: /full/path/to/directory                       │
│  Value: {                                           │
│    mtime: timestamp                                 │
│    size: int64                                      │
│    usage: int64                                     │
│    itemCount: int                                   │
│    files: []FileMetadata                            │
│    cachedAt: timestamp                              │
│  }                                                  │
└─────────────────────────────────────────────────────┘
```

### Key Design Decisions

1. **Extend BadgerDB Storage**: Leverage existing `pkg/analyze/storage.go` infrastructure
2. **Mtime-based Validation**: Use directory mtime as the cache invalidation trigger
3. **Per-Directory Granularity**: Cache at directory level, not file level
4. **Hybrid Approach**: Combine cached and fresh data in single result tree
5. **Backward Compatible**: New analyzer type, doesn't break existing features

---

## 3. Database Schema

### Extended StoredDir Structure

```go
// pkg/analyze/incremental.go

type IncrementalDirMetadata struct {
    Path         string              // Full path to directory
    Mtime        time.Time           // Directory modification time
    Size         int64               // Total apparent size
    Usage        int64               // Total disk usage
    ItemCount    int                 // Number of items in tree
    Flag         rune                // Directory flag
    Files        []FileMetadata      // Direct children metadata
    CachedAt     time.Time           // When this was cached
    ScanDuration time.Duration       // How long the scan took
}

type FileMetadata struct {
    Name      string
    IsDir     bool
    Size      int64
    Usage     int64
    Mtime     time.Time
    Flag      rune
    Mli       uint64  // Multi-linked inode (for hardlinks)
}
```

### BadgerDB Key Structure

```
Format: "incr:<path>"
Examples:
  - "incr:/home/user/data"
  - "incr:/mnt/nfs/projects/ml-models"

Metadata Key: "incr:meta:<path>"
  - Stores IncrementalDirMetadata as gob-encoded bytes
```

### Cache Metadata Tracking

```go
type CacheMetadata struct {
    LastFullScan   time.Time
    TotalDirsCached int
    CacheVersion   string  // For schema migrations
}

// Stored at key: "incr:cache:metadata"
```

---

## 4. API/CLI Design

### New Command-Line Flags

```go
// Add to cmd/gdu/app/app.go Flags struct

type Flags struct {
    // ... existing fields ...

    // Incremental caching
    UseIncremental     bool          `yaml:"use-incremental"`
    IncrementalPath    string        `yaml:"incremental-path"`
    CacheMaxAge        time.Duration `yaml:"cache-max-age"`
    ForceFullScan      bool          `yaml:"force-full-scan"`
    MaxIOPS            int           `yaml:"max-iops"`
    IODelay            time.Duration `yaml:"io-delay"`
    ShowCacheStats     bool          `yaml:"show-cache-stats"`
}
```

### CLI Flag Definitions

```bash
# New flags
--incremental                    # Enable incremental caching mode
--incremental-path string        # Path to incremental cache DB (default: ~/.cache/gdu/incremental)
--cache-max-age duration         # Max age before cache entry is invalid (default: 0 = no expiry)
--force-full-scan               # Ignore cache and perform full scan (updates cache)
--max-iops int                  # Limit I/O operations per second (0 = unlimited)
--io-delay duration             # Delay between directory scans (e.g., 10ms)
--show-cache-stats              # Show cache hit/miss statistics after scan

# Examples
gdu --incremental /mnt/nfs/data
gdu --incremental --cache-max-age 24h /mnt/nfs/data
gdu --incremental --max-iops 1000 /mnt/nfs/data
gdu --incremental --force-full-scan /mnt/nfs/data
gdu --incremental --show-cache-stats /mnt/nfs/data
```

### Configuration File Support

```yaml
# ~/.gdu.yaml

use-incremental: true
incremental-path: /home/user/.cache/gdu/incremental
cache-max-age: 24h
max-iops: 1000
io-delay: 10ms
show-cache-stats: false
```

---

## 5. Implementation Phases

### Phase 1: Core Incremental Analyzer (Week 1)

**Goal**: Basic incremental caching without I/O throttling

**Files to Create**:
- `pkg/analyze/incremental.go` - Main IncrementalAnalyzer implementation
- `pkg/analyze/incremental_storage.go` - Extended BadgerDB operations
- `pkg/analyze/incremental_test.go` - Unit tests

**Files to Modify**:
- `cmd/gdu/app/app.go` - Add new flags, instantiate analyzer
- `internal/common/analyze.go` - No changes needed (interface compatible)

**Key Functions**:

```go
// pkg/analyze/incremental.go

type IncrementalAnalyzer struct {
    storage          *IncrementalStorage
    storagePath      string
    cacheMaxAge      time.Duration
    forceFullScan    bool
    stats            *CacheStats
    // ... channels and sync primitives similar to StoredAnalyzer
}

func CreateIncrementalAnalyzer(opts IncrementalOptions) *IncrementalAnalyzer

func (a *IncrementalAnalyzer) AnalyzeDir(
    path string,
    ignore common.ShouldDirBeIgnored,
    constGC bool,
) fs.Item

func (a *IncrementalAnalyzer) processDir(path string) *Dir {
    // 1. Get current directory stat
    // 2. Check cache for this path
    // 3. Compare mtime
    // 4. If unchanged and not expired -> load from cache
    // 5. If changed -> re-scan and update cache
}

func (a *IncrementalAnalyzer) shouldUseCachedDir(
    path string,
    currentMtime time.Time,
) (bool, *IncrementalDirMetadata)

func (a *IncrementalAnalyzer) rebuildDirFromCache(
    meta *IncrementalDirMetadata,
) *Dir
```

**Acceptance Criteria**:
- ✅ Can perform initial scan and cache results
- ✅ Subsequent scan with no changes uses 100% cached data
- ✅ Changed directory triggers re-scan of that subtree only
- ✅ Cache expiry based on --cache-max-age works
- ✅ --force-full-scan ignores cache and updates it

---

### Phase 2: I/O Throttling (Week 2)

**Goal**: Add configurable I/O rate limiting to be gentle on storage

**Files to Create**:
- `pkg/analyze/throttle.go` - I/O throttling implementation

**Files to Modify**:
- `pkg/analyze/incremental.go` - Integrate throttling

**Key Structures**:

```go
// pkg/analyze/throttle.go

type IOThrottle struct {
    maxIOPS       int
    ioDelay       time.Duration
    opCounter     *RateLimiter
    lastOpTime    time.Time
    mu            sync.Mutex
}

func NewIOThrottle(maxIOPS int, ioDelay time.Duration) *IOThrottle

func (t *IOThrottle) Acquire() {
    // Block until we can perform the next I/O operation
    // Uses token bucket or leaky bucket algorithm
}

func (t *IOThrottle) Reset()

// RateLimiter using golang.org/x/time/rate
type RateLimiter struct {
    limiter *rate.Limiter
}
```

**Integration Point**:

```go
// In IncrementalAnalyzer.processDir()
if a.throttle != nil {
    a.throttle.Acquire()
}
files, err := os.ReadDir(path)
```

**Acceptance Criteria**:
- ✅ --max-iops limits directory reads per second
- ✅ --io-delay adds fixed delay between scans
- ✅ Throttling works with both parallel and sequential modes
- ✅ Performance degrades gracefully with throttling

---

### Phase 3: Cache Statistics & Management (Week 3)

**Goal**: Provide visibility into cache performance and management tools

**Files to Create**:
- `pkg/analyze/incremental_stats.go` - Statistics tracking

**Files to Modify**:
- `pkg/analyze/incremental.go` - Integrate stats collection
- `tui/tui.go` - Display cache stats in UI
- `stdout/stdout.go` - Display cache stats in non-interactive mode

**Statistics Structure**:

```go
type CacheStats struct {
    TotalDirs        int64
    CacheHits        int64
    CacheMisses      int64
    CacheExpired     int64
    DirsRescanned    int64
    BytesFromCache   int64
    BytesScanned     int64
    ScanTime         time.Duration
    CacheLoadTime    time.Duration

    mu sync.RWMutex
}

func (s *CacheStats) HitRate() float64 {
    total := s.CacheHits + s.CacheMisses
    if total == 0 {
        return 0
    }
    return float64(s.CacheHits) / float64(total)
}

func (s *CacheStats) IOReduction() float64 {
    total := s.BytesFromCache + s.BytesScanned
    if total == 0 {
        return 0
    }
    return float64(s.BytesFromCache) / float64(total)
}
```

**Output Format** (non-interactive mode):

```
Cache Statistics:
  Hit Rate:         87.3% (1,234 hits, 179 misses)
  I/O Reduction:    91.2% (4.2 TB cached, 387 GB scanned)
  Directories:      1,413 total, 45 rescanned, 12 expired
  Performance:      Scan: 45s, Cache load: 2.3s
```

**TUI Integration**:
- Show cache stats in footer or info modal
- Add keybinding (e.g., 'c') to show detailed cache stats
- Indicate cached vs scanned directories with flag

**Acceptance Criteria**:
- ✅ --show-cache-stats displays detailed statistics
- ✅ Statistics accurate for all scan scenarios
- ✅ TUI shows cache indicators for directories
- ✅ Can export statistics to JSON with --output-file

---

### Phase 4: Edge Cases & Robustness (Week 4)

**Goal**: Handle all edge cases and ensure production readiness

**Edge Cases to Handle**:

1. **Clock Skew**:
   - Problem: NFS server clock differs from client
   - Solution: Store both mtime and cache timestamp, use delta comparison

   ```go
   func isClockSkewed(dirMtime, cachedMtime time.Time, cachedAt time.Time) bool {
       // If cached mtime is in the future relative to cache time
       return cachedMtime.After(cachedAt.Add(1 * time.Hour))
   }
   ```

2. **Deleted Files/Directories**:
   - Problem: Cached directory no longer exists
   - Solution: Validate path existence before using cache

   ```go
   func (a *IncrementalAnalyzer) validateCachedEntry(path string) error {
       _, err := os.Stat(path)
       return err
   }
   ```

3. **Moved/Renamed Directories**:
   - Problem: Directory moved, cache uses old path
   - Solution: Cache is per-path; moved dir appears as new scan (acceptable)

4. **Permissions Changes**:
   - Problem: Directory permissions changed, can't read anymore
   - Solution: Detect read errors, mark as error in results

5. **Concurrent Modifications**:
   - Problem: Files change during scan
   - Solution: Best-effort consistency (same as current gdu behavior)

6. **Cache Corruption**:
   - Problem: BadgerDB corruption or version mismatch
   - Solution: Graceful fallback to full scan

   ```go
   func (a *IncrementalAnalyzer) handleCacheError(err error, path string) {
       log.Warn("Cache error for %s: %v, falling back to full scan", path, err)
       a.stats.CacheMisses++
       return a.fullScanDir(path)
   }
   ```

7. **Large Directory Handling**:
   - Problem: Directory with millions of files
   - Solution: Stream file metadata instead of loading all at once

8. **Hardlink Changes**:
   - Problem: Hardlinks created/removed
   - Solution: Re-scan if directory item count changes

**Files to Modify**:
- `pkg/analyze/incremental.go` - Add edge case handling
- `pkg/analyze/incremental_storage.go` - Add validation and error recovery

**Acceptance Criteria**:
- ✅ All edge cases have unit tests
- ✅ Graceful degradation on cache errors
- ✅ No crashes or data loss scenarios
- ✅ Clear error messages for all failure modes

---

### Phase 5: Integration & Documentation (Week 5)

**Goal**: Complete integration with existing features and comprehensive documentation

**Compatibility Matrix**:

| Feature | Compatible | Notes |
|---------|-----------|-------|
| `--output-file` (JSON export) | ✅ Yes | Export includes cache metadata |
| `--input-file` (JSON import) | ✅ Yes | Can import to populate cache |
| `--sequential` | ✅ Yes | Works with sequential analyzer |
| `--use-storage` | ⚠️ Exclusive | Can't use both; incremental supersedes |
| `--no-cross` | ✅ Yes | Cache respects filesystem boundaries |
| `--ignore-dirs` | ✅ Yes | Ignored dirs not cached |
| `--follow-symlinks` | ✅ Yes | Symlink targets included in cache |
| Interactive TUI | ✅ Yes | Full compatibility |
| Non-interactive mode | ✅ Yes | Full compatibility |

**Documentation Files to Create/Update**:

1. **README.md** - Add incremental caching section
2. **configuration.md** - Document all new flags
3. **docs/incremental-caching.md** - Detailed user guide (new file)

**Content for docs/incremental-caching.md**:

```markdown
# Incremental Caching Guide

## Overview
Incremental caching reduces I/O by only scanning changed directories...

## Quick Start
...

## How It Works
...

## Best Practices
- Set cache-max-age for data freshness requirements
- Use max-iops on shared NFS to avoid overloading storage
- Run force-full-scan weekly to ensure accuracy
...

## Troubleshooting
...

## Performance Tuning
...
```

**Acceptance Criteria**:
- ✅ All existing tests pass
- ✅ New integration tests cover all feature combinations
- ✅ Documentation complete and reviewed
- ✅ Example configurations provided

---

## 6. Algorithm Design

### Cache Validation Algorithm

```go
func (a *IncrementalAnalyzer) processDir(path string) *Dir {
    // Step 1: Get current filesystem state
    stat, err := os.Stat(path)
    if err != nil {
        return a.createErrorDir(path, err)
    }
    currentMtime := stat.ModTime()

    // Step 2: Check if force full scan
    if a.forceFullScan {
        return a.scanAndCache(path, currentMtime)
    }

    // Step 3: Try to load from cache
    cached, err := a.storage.LoadDirMetadata(path)
    if err != nil {
        // Cache miss - new directory
        a.stats.IncrementCacheMisses()
        return a.scanAndCache(path, currentMtime)
    }

    // Step 4: Validate cache age
    if a.cacheMaxAge > 0 {
        age := time.Since(cached.CachedAt)
        if age > a.cacheMaxAge {
            a.stats.IncrementCacheExpired()
            return a.scanAndCache(path, currentMtime)
        }
    }

    // Step 5: Compare mtime
    if !cached.Mtime.Equal(currentMtime) {
        // Directory modified - rescan
        a.stats.IncrementDirsRescanned()
        return a.scanAndCache(path, currentMtime)
    }

    // Step 6: Cache hit - rebuild from cache
    a.stats.IncrementCacheHits()
    a.stats.AddBytesFromCache(cached.Size)
    return a.rebuildFromCache(cached)
}
```

### Incremental Update Algorithm

```go
func (a *IncrementalAnalyzer) rebuildFromCache(
    cached *IncrementalDirMetadata,
) *Dir {
    dir := &Dir{
        File: &File{
            Name:  filepath.Base(cached.Path),
            Size:  cached.Size,
            Usage: cached.Usage,
            Mtime: cached.Mtime,
            Flag:  cached.Flag,
        },
        BasePath:  filepath.Dir(cached.Path),
        ItemCount: cached.ItemCount,
        Files:     make(fs.Files, 0, len(cached.Files)),
    }

    // Recursively process child directories
    for _, fileMeta := range cached.Files {
        if fileMeta.IsDir {
            childPath := filepath.Join(cached.Path, fileMeta.Name)

            // Recursive cache check for subdirectories
            childDir := a.processDir(childPath)
            childDir.Parent = dir
            dir.AddFile(childDir)
        } else {
            // Files are loaded directly from metadata
            file := &File{
                Name:   fileMeta.Name,
                Size:   fileMeta.Size,
                Usage:  fileMeta.Usage,
                Mtime:  fileMeta.Mtime,
                Flag:   fileMeta.Flag,
                Mli:    fileMeta.Mli,
                Parent: dir,
            }
            dir.AddFile(file)
        }
    }

    return dir
}
```

### Scan and Cache Algorithm

```go
func (a *IncrementalAnalyzer) scanAndCache(
    path string,
    currentMtime time.Time,
) *Dir {
    startTime := time.Now()

    // Perform actual filesystem scan
    dir := a.performFullScan(path)

    // Build metadata for caching
    meta := &IncrementalDirMetadata{
        Path:         path,
        Mtime:        currentMtime,
        Size:         dir.Size,
        Usage:        dir.Usage,
        ItemCount:    dir.ItemCount,
        Flag:         dir.Flag,
        Files:        a.extractFileMetadata(dir),
        CachedAt:     time.Now(),
        ScanDuration: time.Since(startTime),
    }

    // Store in cache
    err := a.storage.StoreDirMetadata(meta)
    if err != nil {
        log.Warn("Failed to cache %s: %v", path, err)
    }

    a.stats.AddBytesScanned(dir.Size)
    return dir
}
```

---

## 7. Testing Strategy

### Unit Tests

**File**: `pkg/analyze/incremental_test.go`

```go
func TestIncrementalAnalyzer_FirstScan(t *testing.T)
func TestIncrementalAnalyzer_UnchangedDirectory(t *testing.T)
func TestIncrementalAnalyzer_ChangedDirectory(t *testing.T)
func TestIncrementalAnalyzer_CacheExpiry(t *testing.T)
func TestIncrementalAnalyzer_ForceFullScan(t *testing.T)
func TestIncrementalAnalyzer_DeletedDirectory(t *testing.T)
func TestIncrementalAnalyzer_NewFiles(t *testing.T)
func TestIncrementalAnalyzer_IOThrottling(t *testing.T)
```

### Integration Tests

**File**: `cmd/gdu/app/app_incremental_test.go`

```go
func TestApp_IncrementalMode(t *testing.T)
func TestApp_IncrementalWithExistingFeatures(t *testing.T)
func TestApp_IncrementalJSONExport(t *testing.T)
```

### Benchmark Tests

**File**: `pkg/analyze/incremental_bench_test.go`

```go
func BenchmarkIncrementalAnalyzer_ColdCache(b *testing.B)
func BenchmarkIncrementalAnalyzer_WarmCache(b *testing.B)
func BenchmarkIncrementalAnalyzer_PartialChanges(b *testing.B)
```

### Test Data Setup

```go
// Helper to create test directory structure
func createTestDir(t *testing.T, files int, dirs int) string {
    tmpDir := t.TempDir()
    // Create nested structure with known mtime
    ...
    return tmpDir
}

// Helper to modify directory and update mtime
func touchDir(t *testing.T, path string) {
    // Add/remove file to change mtime
    ...
}
```

### Test Coverage Goals

- Unit test coverage: >85%
- Integration test coverage: All flag combinations
- Benchmark tests: Compare against non-incremental mode

---

## 8. Performance Considerations

### Expected I/O Reduction

**Scenario 1: Daily Scan, 1% Changes**
- First scan: 100% I/O (10,000 dirs, 1M files)
- Second scan: ~1% I/O (100 changed dirs rescanned)
- **Reduction: 99%**

**Scenario 2: Hourly Scan, 5% Changes**
- First scan: 100% I/O
- Subsequent scans: ~5% I/O
- **Reduction: 95%**

**Scenario 3: Weekly Scan with Expiry**
- Cache max age: 7 days
- Daily changes: 1%
- After 7 days: Full rescan
- **Average Reduction: ~85%**

### Memory Usage

**Current gdu (in-memory)**:
- 1M files × ~200 bytes = ~200 MB

**Incremental mode**:
- Cache stored in BadgerDB (on disk)
- In-memory: Only active subtree
- **Memory savings: 50-90%** depending on cache hit rate

### Disk Space

**Cache size estimation**:
- Per directory: ~200 bytes metadata
- Per file: ~100 bytes metadata
- 1M files in 10K dirs: ~10K × 200B + 1M × 100B = ~102 MB

**Recommendation**: Reserve 500 MB for cache per 5M files

### Scan Time Comparison

| Scenario | Without Cache | With Cache (99% hits) | Speedup |
|----------|--------------|----------------------|---------|
| 10K dirs, 100K files | 30s | 2s | 15x |
| 100K dirs, 1M files | 5m | 20s | 15x |
| 1M dirs, 10M files | 45m | 3m | 15x |

*Assumes NFS with 20ms latency per directory read*

---

## 9. Backward Compatibility

### Non-Breaking Changes

1. **New Analyzer Type**: Opt-in via `--incremental` flag
2. **Existing Flags Work**: All current flags remain functional
3. **Config File**: New fields optional, defaults maintain current behavior
4. **JSON Export**: Extended but backward compatible

### Migration Path

**From `--use-storage` to `--incremental`**:

```bash
# Old approach (slow, memory-efficient)
GOGC=10 gdu -g --use-storage /path

# New approach (fast, cached)
gdu --incremental /path
```

**Deprecation Strategy**:
- `--use-storage` remains supported
- Documentation encourages `--incremental` for better performance
- No removal planned for `--use-storage`

### Compatibility Testing

```go
func TestBackwardCompatibility(t *testing.T) {
    tests := []struct {
        name  string
        flags []string
        want  error
    }{
        {"normal mode", []string{"/path"}, nil},
        {"incremental only", []string{"--incremental", "/path"}, nil},
        {"with JSON export", []string{"--incremental", "-o", "out.json", "/path"}, nil},
        {"with storage", []string{"--use-storage", "/path"}, nil},
        {"both storage modes", []string{"--use-storage", "--incremental", "/path"}, ErrConflictingFlags},
    }
    // ...
}
```

---

## 10. Future Enhancements

### Phase 6+: Advanced Features (Post-MVP)

#### 1. Cache Warming

**Concept**: Pre-populate cache in background

```bash
gdu --incremental-warm /mnt/nfs/data &
# Later: instant results
gdu --incremental /mnt/nfs/data
```

**Use Case**: Scheduled nightly scans for next-day analysis

---

#### 2. Distributed Cache Sharing

**Concept**: Share cache across team via network storage

```bash
gdu --incremental --cache-shared=/mnt/shared/gdu-cache /data
```

**Benefits**: Team shares cache, reduces redundant scans

**Challenges**: Locking, concurrency, version conflicts

---

#### 3. Smart Cache Eviction

**Concept**: LRU eviction when cache exceeds size limit

```bash
gdu --incremental --cache-max-size=1GB /data
```

**Algorithm**: Track access time, evict least recently used

---

#### 4. Predictive Prefetching

**Concept**: Predict which directories will be accessed next

```go
// Based on access patterns, prefetch likely paths
func (a *IncrementalAnalyzer) prefetchLikelyPaths(currentPath string) {
    // If user navigated /home/user/projects, prefetch sibling dirs
}
```

---

#### 5. Change Notifications (inotify/FSEvents)

**Concept**: Use filesystem watchers to invalidate cache in real-time

```bash
gdu --incremental --watch /data
# Real-time updates as files change
```

**Platforms**: Linux (inotify), macOS (FSEvents), Windows (ReadDirectoryChangesW)

---

#### 6. Compression for Large Caches

**Concept**: Compress cache entries with zstd or similar

**Trade-off**: Lower disk usage vs. CPU overhead

---

#### 7. Cloud Storage Support

**Concept**: Cache S3/GCS directory listings

```bash
gdu --incremental s3://bucket/prefix
```

**Benefit**: Avoid expensive cloud API calls

---

#### 8. Machine Learning for Change Prediction

**Concept**: Learn patterns to predict which dirs will change

**Example**: "/var/log changes daily, /usr/lib never changes"

---

## 11. File Structure Summary

### New Files to Create

```
pkg/analyze/
├── incremental.go              # Main IncrementalAnalyzer (500 lines)
├── incremental_storage.go      # BadgerDB operations (300 lines)
├── incremental_stats.go        # Statistics tracking (150 lines)
├── throttle.go                 # I/O throttling (100 lines)
├── incremental_test.go         # Unit tests (600 lines)
├── incremental_bench_test.go   # Benchmarks (200 lines)
└── incremental_storage_test.go # Storage tests (300 lines)

cmd/gdu/app/
└── app_incremental_test.go     # Integration tests (400 lines)

docs/
├── incremental-caching.md      # User guide (1000 lines)
└── incremental-caching-implementation-plan.md  # This document
```

### Files to Modify

```
cmd/gdu/app/app.go              # Add flags, create analyzer
configuration.md                 # Document new options
README.md                        # Add incremental section
tui/tui.go                      # Show cache stats (optional)
stdout/stdout.go                # Print cache stats
```

**Total New Code**: ~3,550 lines (including tests)

---

## 12. Implementation Checklist

### Pre-Implementation
- [ ] Review this plan with stakeholders
- [ ] Set up development branch
- [ ] Create initial issue/ticket
- [ ] Set up test environment with NFS

### Phase 1: Core Incremental Analyzer
- [ ] Implement IncrementalAnalyzer struct
- [ ] Implement processDir with mtime checking
- [ ] Implement cache storage layer
- [ ] Write unit tests (>80% coverage)
- [ ] Manual testing with sample directories

### Phase 2: I/O Throttling
- [ ] Implement IOThrottle
- [ ] Integrate with IncrementalAnalyzer
- [ ] Test max-iops functionality
- [ ] Test io-delay functionality
- [ ] Benchmark performance impact

### Phase 3: Cache Statistics
- [ ] Implement CacheStats tracking
- [ ] Add CLI output for stats
- [ ] Add TUI integration (optional)
- [ ] Test statistics accuracy

### Phase 4: Edge Cases
- [ ] Handle clock skew
- [ ] Handle deleted directories
- [ ] Handle permission errors
- [ ] Handle cache corruption
- [ ] Add error recovery tests

### Phase 5: Integration & Documentation
- [ ] Test with all existing flags
- [ ] Write comprehensive docs
- [ ] Update README and configuration.md
- [ ] Create example configurations
- [ ] Performance benchmarking

### Release
- [ ] Code review
- [ ] Final testing on real NFS
- [ ] Update CHANGELOG
- [ ] Create PR
- [ ] Merge to main

---

## 13. Risk Assessment & Mitigation

### Risk 1: mtime Unreliability
**Risk**: NFS mtime may not update immediately
**Likelihood**: Medium
**Impact**: High (stale cache)
**Mitigation**:
- Document mtime limitations
- Add `--no-cache-trust-mtime` flag for paranoid mode
- Fall back to full scan if suspicious

### Risk 2: Cache Corruption
**Risk**: BadgerDB corruption loses all cache
**Likelihood**: Low
**Impact**: Medium (need to rebuild)
**Mitigation**:
- Graceful fallback to full scan
- Regular cache validation
- Export/import cache backup feature

### Risk 3: Excessive Cache Size
**Risk**: Cache grows unbounded
**Likelihood**: Medium
**Impact**: Low (disk space)
**Mitigation**:
- Document cache size expectations
- Add cache cleanup command
- Future: automatic eviction

### Risk 4: Performance Regression
**Risk**: Incremental mode slower than regular scan
**Likelihood**: Low
**Impact**: High (defeats purpose)
**Mitigation**:
- Comprehensive benchmarking
- Performance tests in CI
- Keep regular mode as default

---

## 14. Success Metrics

### Quantitative Metrics
- **I/O Reduction**: >90% on subsequent scans (1% changes)
- **Scan Time**: <10% of full scan time on subsequent runs
- **Memory Usage**: <50% of regular in-memory mode
- **Cache Hit Rate**: >95% for daily scans
- **Cache Overhead**: <5% disk space vs. scanned data

### Qualitative Metrics
- User feedback positive on NFS performance
- No bug reports for cache corruption
- Documentation rated as clear and helpful
- Community adoption (GitHub stars, issues, PRs)

---

## 15. Summary

This implementation plan provides a complete roadmap for adding incremental caching to gdu with:

1. **Minimal invasiveness**: New analyzer type, no breaking changes
2. **Robust design**: Handles edge cases, graceful degradation
3. **Excellent performance**: 90%+ I/O reduction for typical use cases
4. **Production-ready**: Comprehensive testing, documentation, error handling
5. **Future-proof**: Extensible architecture for advanced features

**Estimated Timeline**: 5 weeks for full implementation including tests and docs

**Key Deliverable**: A production-ready `--incremental` mode that makes gdu viable for daily NFS scans without overloading storage systems.

---

## Next Steps

1. Review this plan and gather feedback from stakeholders
2. Create development branch: `feature/incremental-caching`
3. Set up test NFS environment for validation
4. Begin Phase 1 implementation
5. Iterate based on testing results and feedback

---

*Document Version: 1.0*
*Last Updated: 2025-10-02*
*Author: Generated via Claude Code collaboration*
