# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**gdu** (Go Disk Usage) is a fast disk usage analyzer written in Go, optimized for SSD disks with parallel processing. It provides both interactive TUI and non-interactive CLI modes for analyzing disk usage.

**Key Features:**
- Parallel directory scanning for SSDs (default mode)
- Sequential scanning mode for HDDs (`--sequential`)
- Interactive terminal UI with deletion capabilities
- JSON export/import for analysis data
- Persistent BadgerDB storage for memory-constrained environments
- **Incremental caching** (experimental) for network filesystems (NFS/DDN) to reduce I/O by 90%+

## Build and Development Commands

### Building
```bash
make build              # Build binary to dist/gdu
make build-static       # Build static binary (CGO_ENABLED=0)
make run               # Run directly with go run
```

### Testing
```bash
make test              # Run all tests with gotestsum
make coverage          # Generate coverage report
make coverage-html     # View coverage in browser

# Run specific test
go test ./pkg/analyze -run TestName -v

# Run single test with timeout
timeout 60 go test ./pkg/analyze -run "TestIncrementalAnalyzer_FirstScan" -v

# Benchmark
make gobench           # Run benchmarks in pkg/analyze
go test -bench=. ./pkg/analyze
```

### Linting
```bash
make lint              # Run golangci-lint with .golangci.yml config
golangci-lint run      # Same as above
```

### Installing Development Tools
```bash
make install-dev-dependencies  # Installs gotestsum, gox, gotraceui, golangci-lint
```

### Other Commands
```bash
./test_unit.sh         # Custom unit test runner script
./test_live.sh         # Live integration testing script
make clean             # Remove build artifacts, coverage, test directories
```

## Architecture Overview

### Core Analyzer Types

gdu uses a **polymorphic analyzer architecture** where different analyzer implementations satisfy the `common.Analyzer` interface:

```go
// internal/common/analyze.go
type Analyzer interface {
    AnalyzeDir(path string, ignore ShouldDirBeIgnored, constGC bool) fs.Item
    SetFollowSymlinks(bool)
    SetShowAnnexedSize(bool)
    GetProgressChan() chan CurrentProgress
    GetDone() SignalGroup
    ResetProgress()
}
```

**Available Analyzer Implementations:**

1. **ParallelAnalyzer** (`pkg/analyze/parallel.go`)
   - Default analyzer for parallel directory scanning
   - Uses goroutines with concurrency limiting
   - Best for SSD storage

2. **SequentialAnalyzer** (`pkg/analyze/sequential.go`)
   - Single-threaded scanning for rotating HDDs
   - Activated with `--sequential` flag

3. **StoredAnalyzer** (`pkg/analyze/stored.go`)
   - Stores analysis data in BadgerDB for memory efficiency
   - Used with `--use-storage` flag
   - ~10x slower but uses much less memory

4. **IncrementalAnalyzer** (`pkg/analyze/incremental.go`) **[NEW - Experimental]**
   - Caches directory metadata with mtime-based validation
   - Only rescans changed directories (90%+ I/O reduction)
   - Designed for NFS/DDN network storage
   - Activated with `--incremental` flag
   - Key components:
     - `incremental.go` - Main analyzer logic
     - `incremental_storage.go` - BadgerDB cache operations
     - `incremental_stats.go` - Cache statistics tracking
     - `throttle.go` - I/O rate limiting (`--max-iops`, `--io-delay`)

### Directory Structure

```
pkg/analyze/          # Core analysis engines
  ├── parallel.go           # Parallel analyzer (default)
  ├── sequential.go         # Sequential analyzer for HDDs
  ├── stored.go             # BadgerDB-backed analyzer
  ├── incremental.go        # NEW: Incremental caching analyzer
  ├── incremental_storage.go # Cache storage layer
  ├── incremental_stats.go  # Cache statistics
  ├── throttle.go           # I/O throttling
  ├── dir.go                # Directory scanning logic
  ├── file.go               # File metadata structures
  └── memory.go             # Memory management

pkg/device/           # Device/disk enumeration (platform-specific)
pkg/fs/              # Filesystem abstraction layer
pkg/remove/          # File/directory deletion logic

cmd/gdu/             # Application entry point
  ├── main.go              # CLI flag parsing with cobra
  └── app/app.go           # Application orchestration

tui/                 # Terminal UI (tview-based)
  ├── tui.go               # Main UI controller
  ├── actions.go           # User interactions (delete, sort, etc.)
  ├── show.go              # Display logic
  └── keys.go              # Keybindings

stdout/              # Non-interactive output mode
report/              # JSON import/export
internal/common/     # Shared interfaces and utilities
```

### Key Data Structures

**File and Directory Hierarchy:**
- `pkg/fs/file.go` - Base `Item` interface
- `pkg/analyze/file.go` - `File` struct (concrete file/symlink)
- `pkg/analyze/dir.go` - `Dir` struct (directories with children)
- Parent-child relationships via `Parent` field or `ParentDir` wrapper

**Incremental Caching Structures:**
```go
// pkg/analyze/incremental_storage.go
type IncrementalDirMetadata struct {
    Path         string
    Mtime        time.Time      // Directory modification time
    Size         int64
    Usage        int64
    ItemCount    int
    Flag         rune
    Files        []FileMetadata // Direct children
    CachedAt     time.Time      // When cached
    ScanDuration time.Duration
}

type FileMetadata struct {
    Name      string
    IsDir     bool
    Size      int64
    Usage     int64
    Mtime     time.Time
    Flag      rune
    Mli       uint64  // Multi-linked inode (hardlinks)
}
```

### Platform-Specific Code

- Many files have platform-specific implementations:
  - `dir_linux-openbsd.go`, `dir_unix.go`, `dir_other.go`
  - `device/dev_linux.go`, `dev_bsd.go`, `dev_freebsd_darwin_other.go`
- Use build tags to control compilation per platform

### Important Patterns

1. **Analyzer Selection** (`cmd/gdu/app/app.go`):
   - Checks flags to determine which analyzer to instantiate
   - Incremental analyzer takes precedence over stored analyzer
   - Default is parallel analyzer

2. **Progress Tracking**:
   - Analyzers use channels (`progressChan`, `progressOutChan`) for real-time progress
   - `GetDone()` returns signal group for completion notification
   - Used by TUI to show scanning progress

3. **Memory Management**:
   - `manageMemoryUsage()` in `pkg/analyze/memory.go`
   - Automatically adjusts GC based on free memory
   - Can be overridden with `--const-gc` flag

4. **I/O Throttling** (Incremental Mode):
   - `pkg/analyze/throttle.go` - Token bucket rate limiter
   - Protects shared storage from I/O storms
   - Configurable via `--max-iops` and `--io-delay`

## Incremental Caching Feature

### Overview
The incremental caching feature dramatically reduces I/O on network filesystems by caching directory metadata and only rescanning directories whose mtime has changed.

### Key Files
- `pkg/analyze/incremental.go` - Main analyzer implementation
- `pkg/analyze/incremental_storage.go` - BadgerDB cache operations
- `pkg/analyze/incremental_stats.go` - Statistics tracking
- `pkg/analyze/throttle.go` - I/O rate limiting
- `docs/incremental-caching-implementation-plan.md` - Complete design doc

### CLI Flags
```bash
--incremental                    # Enable incremental mode
--incremental-path string        # Cache directory path
--cache-max-age duration         # Max cache age before refresh
--force-full-scan               # Ignore cache, full rescan
--max-iops int                  # I/O operations per second limit
--io-delay duration             # Fixed delay between dir scans
--show-cache-stats              # Display cache statistics
```

### Algorithm
1. Check if directory exists in cache
2. Compare current mtime with cached mtime
3. If unchanged and not expired → load from cache
4. If changed → rescan directory and update cache
5. Recursively apply to subdirectories

### Testing Incremental Mode
```bash
# First scan (cold cache)
./dist/gdu --incremental --show-cache-stats /tmp/test-dir

# Second scan (warm cache - should be much faster)
./dist/gdu --incremental --show-cache-stats /tmp/test-dir

# Force full rescan
./dist/gdu --incremental --force-full-scan /tmp/test-dir

# With I/O throttling
./dist/gdu --incremental --max-iops 100 /tmp/test-dir
```

### Common Issues
- **Cache path errors**: Ensure `--incremental-path` directory is writable
- **Stale cache**: Use `--force-full-scan` to rebuild cache
- **Performance**: First scan is slower due to cache writes; subsequent scans are 10-15x faster

## Testing Strategy

### Unit Tests
- Each analyzer has comprehensive tests (e.g., `incremental_test.go`)
- Use `t.TempDir()` for temporary test directories
- Mock filesystem operations where possible

### Integration Tests
- `cmd/gdu/app/app_test.go` - Full application tests
- Test flag combinations and analyzer selection

### Benchmark Tests
- `pkg/analyze/*_bench_test.go`
- Compare analyzer performance

### Live Testing
- `test_unit.sh` - Custom test runner
- `test_live.sh` - Live integration testing
- Real NFS testing recommended for incremental mode

## Configuration Files

### User Config
- `~/.config/gdu/gdu.yaml` or `~/.gdu.yaml`
- Supports all flags in YAML format
- Example:
  ```yaml
  use-incremental: true
  incremental-path: /home/user/.cache/gdu/incremental
  cache-max-age: 24h
  max-iops: 1000
  ```

### Project Config
- `.golangci.yml` - Linter configuration
- `go.mod` - Go 1.24.0, uses BadgerDB v3, tcell v2, cobra, etc.

## Dependencies

Key external libraries:
- **github.com/dgraph-io/badger/v3** - Embedded key-value store for caching
- **github.com/gdamore/tcell/v2** - Terminal UI framework
- **github.com/rivo/tview** - High-level TUI components
- **github.com/spf13/cobra** - CLI framework
- **golang.org/x/time/rate** - Rate limiting for I/O throttling

## Git Workflow

Current branch: `feature/incremental-caching`
Main branch: `master`

### Commit Guidelines
- Use descriptive commit messages
- Prefix with category: `feat:`, `fix:`, `test:`, `docs:`
- Example: `feat: implement I/O throttling for incremental caching (Phase 2)`

## Special Considerations

1. **Platform Compatibility**: Test on Linux, macOS, Windows
2. **Performance**: Benchmark new analyzers against baseline
3. **Memory**: Watch memory usage, especially with large directory trees
4. **NFS/Network Storage**: Incremental caching specifically designed for high-latency storage
5. **BadgerDB**: Always call `Close()` on storage to avoid corruption

## Code Style

- Follow standard Go conventions
- Use `gofmt` / `goimports`
- Pass `golangci-lint` checks
- Document exported functions and types
- Use meaningful variable names (avoid single-letter unless loop counters)

## Current Development Status

**Incremental Caching Feature:**
- ✅ Phase 1: Core incremental analyzer (complete)
- ✅ Phase 2: I/O throttling (complete)
- ⚠️  Phase 3: Cache statistics display (in progress)
- ⏳ Phase 4: Edge case handling (planned)
- ⏳ Phase 5: Integration & documentation (planned)

See `docs/incremental-caching-implementation-plan.md` for complete roadmap.
