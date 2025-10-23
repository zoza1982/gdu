package analyze

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/dundee/gdu/v5/internal/common"
	"github.com/dundee/gdu/v5/pkg/fs"
	log "github.com/sirupsen/logrus"
)

const (
	// DefaultDirBlockSize is the typical directory block size on most filesystems.
	// This is used as a fallback when the actual directory size cannot be determined.
	// Actual sizes vary by filesystem (ext4, XFS, NFS, etc.) and can range from
	// 512 bytes to 64KB or more depending on the filesystem configuration.
	DefaultDirBlockSize = 4096
)

// IncrementalAnalyzer implements Analyzer with incremental caching based on mtime
type IncrementalAnalyzer struct {
	storage          *IncrementalStorage
	storagePath      string
	cacheMaxAge      time.Duration
	forceFullScan    bool
	throttle         *IOThrottle // I/O rate limiting to protect shared storage
	stats            *CacheStats
	progress         *common.CurrentProgress
	progressChan     chan common.CurrentProgress
	progressOutChan  chan common.CurrentProgress
	progressDoneChan chan struct{}
	doneChan         common.SignalGroup
	wait             *WaitGroup
	ignoreDir        common.ShouldDirBeIgnored
	followSymlinks   bool
	gitAnnexedSize   bool
}

// IncrementalOptions contains configuration for IncrementalAnalyzer
type IncrementalOptions struct {
	StoragePath   string
	CacheMaxAge   time.Duration
	ForceFullScan bool
	MaxIOPS       int           // Maximum I/O operations per second (0 = unlimited)
	IODelay       time.Duration // Fixed delay between directory scans (0 = no delay)
}

// CreateIncrementalAnalyzer returns a new IncrementalAnalyzer instance
func CreateIncrementalAnalyzer(opts IncrementalOptions) *IncrementalAnalyzer {
	return &IncrementalAnalyzer{
		storagePath:   opts.StoragePath,
		cacheMaxAge:   opts.CacheMaxAge,
		forceFullScan: opts.ForceFullScan,
		throttle:      NewIOThrottle(opts.MaxIOPS, opts.IODelay),
		stats:         NewCacheStats(),
		progress: &common.CurrentProgress{
			ItemCount: 0,
			TotalSize: int64(0),
		},
		progressChan:     make(chan common.CurrentProgress, 1),
		progressOutChan:  make(chan common.CurrentProgress, 1),
		progressDoneChan: make(chan struct{}),
		doneChan:         make(common.SignalGroup),
		wait:             (&WaitGroup{}).Init(),
	}
}

// GetProgressChan returns channel for getting progress
func (a *IncrementalAnalyzer) GetProgressChan() chan common.CurrentProgress {
	return a.progressOutChan
}

// GetDone returns channel for checking when analysis is done
func (a *IncrementalAnalyzer) GetDone() common.SignalGroup {
	return a.doneChan
}

// SetFollowSymlinks sets whether to follow symlinks
func (a *IncrementalAnalyzer) SetFollowSymlinks(v bool) {
	a.followSymlinks = v
}

// SetShowAnnexedSize sets whether to show git-annexed file sizes
func (a *IncrementalAnalyzer) SetShowAnnexedSize(v bool) {
	a.gitAnnexedSize = v
}

// ResetProgress resets progress tracking
func (a *IncrementalAnalyzer) ResetProgress() {
	a.progress = &common.CurrentProgress{}
	a.progressChan = make(chan common.CurrentProgress, 1)
	a.progressOutChan = make(chan common.CurrentProgress, 1)
	a.progressDoneChan = make(chan struct{})
	a.doneChan = make(common.SignalGroup)
	a.wait = (&WaitGroup{}).Init()
	a.stats = NewCacheStats()
}

// GetCacheStats returns cache statistics
func (a *IncrementalAnalyzer) GetCacheStats() *CacheStats {
	return a.stats
}

// AnalyzeDir analyzes given path with incremental caching
func (a *IncrementalAnalyzer) AnalyzeDir(
	path string, ignore common.ShouldDirBeIgnored, constGC bool,
) fs.Item {
	if !constGC {
		defer debug.SetGCPercent(debug.SetGCPercent(-1))
		go manageMemoryUsage(a.doneChan)
	}

	startTime := time.Now()
	a.stats.ScanStartTime = startTime

	// Start progress updates early to prevent hanging if there's an error
	go a.updateProgress()

	a.storage = NewIncrementalStorage(a.storagePath, path)
	closeFn, err := a.storage.Open()
	if err != nil {
		// Return a descriptive error directory instead of nil
		errMsg := fmt.Sprintf(`Failed to initialize incremental cache at %s: %v

Possible causes and solutions:
  1. Directory doesn't exist
     → Create it with: mkdir -p %s

  2. Permission denied
     → Check directory permissions: ls -ld %s
     → Ensure you have write access: chmod u+w %s

  3. Disk full
     → Check disk space: df -h
     → Free up space or use a different location

  4. Invalid path or filesystem issues
     → Verify the path is correct and accessible
     → Check if the filesystem is mounted and writable

To specify a different cache location, use:
  --incremental-path /path/to/cache

For more help, see: https://github.com/dundee/gdu#incremental-caching
`, a.storagePath, err, a.storagePath, a.storagePath, a.storagePath)

		log.Error(errMsg)
		fmt.Fprintf(os.Stderr, "%s\n", errMsg)

		// Signal completion even on error to prevent hanging
		a.progressDoneChan <- struct{}{}
		a.doneChan.Broadcast()

		return &Dir{
			File: &File{
				Name: filepath.Base(path),
				Flag: '!',
			},
			BasePath:  filepath.Dir(path),
			ItemCount: 0,
			Files:     make(fs.Files, 0),
		}
	}
	defer closeFn()

	a.ignoreDir = ignore

	dir := a.processDir(path)

	a.wait.Wait()

	a.progressDoneChan <- struct{}{}
	a.doneChan.Broadcast()

	a.stats.ScanEndTime = time.Now()
	a.stats.TotalScanTime = a.stats.ScanEndTime.Sub(startTime)

	return dir
}

// processDir processes a single directory with incremental caching logic
func (a *IncrementalAnalyzer) processDir(path string) *Dir {
	// Step 1: Get current filesystem state
	stat, err := os.Stat(path)
	if err != nil {
		log.Printf("Error stating directory %s: %v", path, err)
		return a.createErrorDir(path, err)
	}
	currentMtime := stat.ModTime()

	// Step 2: Check if force full scan is enabled
	if a.forceFullScan {
		a.stats.IncrementDirsRescanned()
		return a.scanAndCache(path, currentMtime)
	}

	// Step 3: Try to load from cache
	cached, err := a.storage.LoadDirMetadata(path)
	if err != nil {
		// Cache miss - new directory or cache error
		a.stats.IncrementCacheMisses()
		a.stats.IncrementTotalDirs()
		return a.scanAndCache(path, currentMtime)
	}

	// Step 4: Validate cache age if max age is set
	if a.cacheMaxAge > 0 {
		age := time.Since(cached.CachedAt)
		if age > a.cacheMaxAge {
			a.stats.IncrementCacheExpired()
			a.stats.IncrementDirsRescanned() // Expired cache requires rescan
			a.stats.IncrementTotalDirs()
			return a.scanAndCache(path, currentMtime)
		}
	}

	// Step 5: Compare mtime to determine if directory changed
	if !cached.Mtime.Equal(currentMtime) {
		// Directory modified - rescan
		a.stats.IncrementDirsRescanned()
		a.stats.IncrementTotalDirs()
		return a.scanAndCache(path, currentMtime)
	}

	// Step 6: Cache hit - rebuild from cache
	a.stats.IncrementCacheHits()
	a.stats.IncrementTotalDirs()
	a.stats.AddBytesFromCache(cached.Size)
	return a.rebuildFromCache(cached)
}

// createErrorDir creates a directory entry for errors
func (a *IncrementalAnalyzer) createErrorDir(path string, _ error) *Dir {
	// Send progress update to prevent hanging
	a.progressChan <- common.CurrentProgress{
		CurrentItemName: path,
		ItemCount:       0,
		TotalSize:       0,
	}

	return &Dir{
		File: &File{
			Name: filepath.Base(path),
			Flag: '!',
		},
		BasePath:  filepath.Dir(path),
		ItemCount: 0,
		Files:     make(fs.Files, 0),
	}
}

// scanAndCache performs a full scan of directory and caches the results
func (a *IncrementalAnalyzer) scanAndCache(path string, currentMtime time.Time) *Dir {
	scanStartTime := time.Now()

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
		ScanDuration: time.Since(scanStartTime),
	}

	// Store in cache
	err := a.storage.StoreDirMetadata(meta)
	if err != nil {
		log.Printf("Warning: Failed to cache %s: %v", path, err)
	}

	a.stats.AddBytesScanned(dir.Size)
	return dir
}

// performFullScan performs an actual filesystem scan of a directory
func (a *IncrementalAnalyzer) performFullScan(path string) *Dir {
	var (
		file       *File
		err        error
		totalSize  int64
		totalUsage int64
		itemCount  int
		info       os.FileInfo
	)

	a.wait.Add(1)
	defer a.wait.Done()

	// Apply I/O throttling before directory read (if enabled)
	if a.throttle != nil {
		if err := a.throttle.Acquire(context.Background()); err != nil {
			// This should only happen on context cancellation, which we don't use yet
			log.Printf("Throttle error for %s: %v", path, err)
		}
	}

	files, err := os.ReadDir(path)
	if err != nil {
		log.Printf("Error reading directory %s: %v", path, err)
	}

	dir := &Dir{
		File: &File{
			Name: filepath.Base(path),
			Flag: getDirFlag(err, len(files)),
		},
		BasePath:  filepath.Dir(path),
		ItemCount: 1,
		Files:     make(fs.Files, 0, len(files)),
	}
	parent := &ParentDir{Path: path}

	setDirPlatformSpecificAttrs(dir, path)

	// Get actual directory size from filesystem
	dirInfo, statErr := os.Stat(path)
	if statErr == nil {
		totalSize = dirInfo.Size()
		// Try to get actual usage from platform-specific attributes
		// This will be set by setPlatformSpecificAttrs if available
		setPlatformSpecificAttrs(&File{}, dirInfo)
		// For directories, we use Size() as it reflects the directory metadata size
		totalUsage = totalSize
	} else {
		// Fallback to conservative estimate if stat fails
		log.Printf("Warning: Could not stat directory %s, using default size: %v", path, statErr)
		totalSize = DefaultDirBlockSize
		totalUsage = DefaultDirBlockSize
	}

	for _, f := range files {
		name := f.Name()
		entryPath := filepath.Join(path, name)

		if f.IsDir() {
			if a.ignoreDir(name, entryPath) {
				continue
			}

			// Recursively process subdirectories
			subdir := a.processDir(entryPath)
			if subdir != nil {
				subdir.Parent = parent
				dir.AddFile(subdir)
				// Accumulate size from subdirectory
				totalSize += subdir.Size
				totalUsage += subdir.Usage
				itemCount += subdir.ItemCount
			}
		} else {
			info, err = f.Info()
			if err != nil {
				log.Printf("Error getting file info for %s: %v", entryPath, err)
				continue
			}

			file = &File{
				Name:   name,
				Flag:   getFlag(info),
				Size:   info.Size(),
				Parent: parent,
			}
			setPlatformSpecificAttrs(file, info)

			// Handle symlinks if enabled
			if a.followSymlinks && info.Mode()&os.ModeSymlink != 0 {
				infoF, err := followSymlink(entryPath, a.gitAnnexedSize)
				if err != nil {
					log.Printf("Error following symlink %s: %v", entryPath, err)
				} else if infoF != nil {
					file.Size = infoF.Size()
					setPlatformSpecificAttrs(file, infoF)
				}
			}

			totalSize += file.Size
			totalUsage += file.Usage
			itemCount++
			dir.AddFile(file)
		}
	}

	// Set the accumulated totals on the directory
	dir.Size = totalSize
	dir.Usage = totalUsage
	dir.ItemCount = itemCount + 1 // +1 for the directory itself

	// Update progress
	a.progressChan <- common.CurrentProgress{
		CurrentItemName: path,
		ItemCount:       len(files),
		TotalSize:       totalSize,
	}

	return dir
}

// extractFileMetadata extracts file metadata from a Dir for caching
func (a *IncrementalAnalyzer) extractFileMetadata(dir *Dir) []FileMetadata {
	if dir.Files == nil {
		return []FileMetadata{}
	}

	files := make([]FileMetadata, 0, len(dir.Files))
	for _, item := range dir.Files {
		meta := FileMetadata{
			Name:  item.GetName(),
			IsDir: item.IsDir(),
			Size:  item.GetSize(),
			Usage: item.GetUsage(),
			Mtime: item.GetMtime(),
			Flag:  item.GetFlag(),
		}

		// Store multi-link inode for hardlinks
		if file, ok := item.(*File); ok {
			meta.Mli = file.Mli
		}

		files = append(files, meta)
	}

	return files
}

// rebuildFromCache reconstructs a Dir from cached metadata
func (a *IncrementalAnalyzer) rebuildFromCache(cached *IncrementalDirMetadata) *Dir {
	log.Printf("Rebuilding from cache: %s (children: %d)", cached.Path, len(cached.Files))

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
	parent := &ParentDir{Path: cached.Path}

	// Reconstruct child items from cached metadata
	for _, fileMeta := range cached.Files {
		if fileMeta.IsDir {
			// FIX: Load child from cache directly, don't call processDir()
			// This prevents loading the entire tree twice into memory
			childPath := filepath.Join(cached.Path, fileMeta.Name)
			childCached, err := a.storage.LoadDirMetadata(childPath)
			if err != nil {
				// Child cache miss shouldn't happen in normal operation
				// Fall back to processDir() only as last resort
				log.Printf("Warning: Child cache miss for %s: %v", childPath, err)
				childDir := a.processDir(childPath)
				if childDir != nil {
					childDir.Parent = parent
					dir.AddFile(childDir)
				}
				continue
			}

			// Recursively rebuild child from its cache entry
			// Note: Statistics are tracked in processDir(), not here to avoid double-counting
			childDir := a.rebuildFromCache(childCached)
			if childDir != nil {
				childDir.Parent = parent
				dir.AddFile(childDir)
			}
		} else {
			// For files, reconstruct directly from metadata
			file := &File{
				Name:   fileMeta.Name,
				Size:   fileMeta.Size,
				Usage:  fileMeta.Usage,
				Mtime:  fileMeta.Mtime,
				Flag:   fileMeta.Flag,
				Mli:    fileMeta.Mli,
				Parent: parent,
			}
			dir.AddFile(file)
		}
	}

	// Send progress update (similar to performFullScan)
	a.progressChan <- common.CurrentProgress{
		CurrentItemName: cached.Path,
		ItemCount:       len(cached.Files),
		TotalSize:       cached.Size,
	}

	return dir
}

// updateProgress sends progress updates to the progress channel
// This goroutine ensures proper cleanup by checking the done signal
// in both select statements to prevent goroutine leaks
func (a *IncrementalAnalyzer) updateProgress() {
	for {
		select {
		case <-a.progressDoneChan:
			return
		case progress := <-a.progressChan:
			a.progress.CurrentItemName = progress.CurrentItemName
			a.progress.ItemCount += progress.ItemCount
			a.progress.TotalSize += progress.TotalSize

			// Check done signal again before sending to avoid blocking on a closed channel
			select {
			case a.progressOutChan <- *a.progress:
			case <-a.progressDoneChan:
				return
			default:
				// Progress update dropped (non-blocking, acceptable for UI updates)
			}
		}
	}
}
