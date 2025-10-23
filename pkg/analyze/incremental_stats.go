package analyze

import (
	"fmt"
	"sync"
	"time"
)

// CacheStats tracks statistics for incremental caching
type CacheStats struct {
	TotalDirs      int64
	CacheHits      int64
	CacheMisses    int64
	CacheExpired   int64
	DirsRescanned  int64
	BytesFromCache int64
	BytesScanned   int64
	ScanStartTime  time.Time
	ScanEndTime    time.Time
	TotalScanTime  time.Duration
	CacheLoadTime  time.Duration

	mu sync.RWMutex
}

// NewCacheStats creates a new CacheStats instance
func NewCacheStats() *CacheStats {
	return &CacheStats{}
}

// IncrementTotalDirs increments the total directories counter
func (s *CacheStats) IncrementTotalDirs() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.TotalDirs++
}

// IncrementCacheHits increments the cache hits counter
func (s *CacheStats) IncrementCacheHits() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CacheHits++
}

// IncrementCacheMisses increments the cache misses counter
func (s *CacheStats) IncrementCacheMisses() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CacheMisses++
}

// IncrementCacheExpired increments the cache expired counter
func (s *CacheStats) IncrementCacheExpired() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CacheExpired++
}

// IncrementDirsRescanned increments the directories rescanned counter
func (s *CacheStats) IncrementDirsRescanned() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.DirsRescanned++
}

// AddBytesFromCache adds to the bytes loaded from cache counter
func (s *CacheStats) AddBytesFromCache(bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BytesFromCache += bytes
}

// AddBytesScanned adds to the bytes scanned counter
func (s *CacheStats) AddBytesScanned(bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BytesScanned += bytes
}

// HitRate calculates the cache hit rate as a percentage
func (s *CacheStats) HitRate() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := s.CacheHits + s.CacheMisses
	if total == 0 {
		return 0
	}
	return float64(s.CacheHits) / float64(total) * 100
}

// IOReduction calculates the I/O reduction percentage
func (s *CacheStats) IOReduction() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	total := s.BytesFromCache + s.BytesScanned
	if total == 0 {
		return 0
	}
	return float64(s.BytesFromCache) / float64(total) * 100
}

// String returns a formatted string representation of statistics
func (s *CacheStats) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return fmt.Sprintf(`Cache Statistics:
  Hit Rate:         %.1f%% (%d hits, %d misses)
  I/O Reduction:    %.1f%% (%s cached, %s scanned)
  Directories:      %d total, %d rescanned, %d expired
  Performance:      Scan: %v, Total: %v`,
		s.HitRate(),
		s.CacheHits,
		s.CacheMisses,
		s.IOReduction(),
		formatBytes(s.BytesFromCache),
		formatBytes(s.BytesScanned),
		s.TotalDirs,
		s.DirsRescanned,
		s.CacheExpired,
		s.TotalScanTime-s.CacheLoadTime,
		s.TotalScanTime,
	)
}

// formatBytes formats byte count as human-readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
