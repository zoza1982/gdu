# Incremental Caching Guide

## Overview

Incremental caching is a powerful feature of gdu designed to dramatically improve performance when scanning network filesystems (NFS, SMB/CIFS) and other storage systems where I/O operations are expensive. By caching directory metadata and validating it using modification times (mtime), gdu can achieve **90%+ reduction in I/O operations** on subsequent scans.

### Key Benefits

- **90%+ I/O reduction** on subsequent scans of unchanged directories
- **10-50x faster** scanning of network filesystems (depending on cache hit rate)
- **Minimal memory overhead** - metadata is stored on disk, not in RAM
- **Automatic invalidation** - directories are rescanned when changes are detected
- **Configurable I/O throttling** - protect shared storage from excessive load
- **Production-ready** - thoroughly tested with comprehensive error handling

### When to Use Incremental Caching

Incremental caching is ideal for:

- **Network filesystems** (NFS, SMB/CIFS, etc.) where I/O latency is high
- **Large directories** that change infrequently (logs, backups, archives)
- **Regular monitoring** of storage usage (daily/weekly scans)
- **Shared storage** where you need to limit I/O impact on other users

Incremental caching is **not** recommended for:

- **Local SSDs** where parallel scanning is already very fast
- **Rapidly changing directories** where cache hit rates will be low
- **One-time scans** where cache provides no benefit

## Quick Start

### Basic Usage

First scan (creates cache):
```bash
gdu --incremental /mnt/nfs-storage
```

Subsequent scans (uses cache):
```bash
gdu --incremental /mnt/nfs-storage
```

That's it! Gdu automatically creates and manages the cache.

### Verify It's Working

To see cache statistics and verify caching is working:
```bash
gdu --incremental --show-cache-stats /mnt/nfs-storage
```

Example output:
```
Cache Statistics:
  Total Directories: 1,234
  Cache Hits: 1,180 (95.6%)
  Cache Misses: 54 (4.4%)
  Directories Rescanned: 54
  Bytes Scanned: 45.2 MB
  Bytes From Cache: 1.2 GB
  I/O Reduction: 96.4%
  Total Scan Time: 2.3s
```

## How It Works

### mtime-Based Cache Validation

Incremental caching uses a simple but effective strategy:

1. **First Scan**: Gdu scans the entire directory tree and caches:
   - Directory modification time (mtime)
   - Directory size and usage
   - File metadata (name, size, mtime, flags)
   - Child directory structure

2. **Subsequent Scans**: For each directory, gdu:
   - Checks the current mtime
   - Compares with cached mtime
   - If unchanged: reconstructs directory from cache (no I/O)
   - If changed: rescans directory and updates cache

3. **Automatic Invalidation**: When a directory's mtime changes (due to file additions, deletions, or modifications), gdu automatically rescans that directory and its parents.

### Cache Storage Location

By default, the cache is stored at:
```
~/.cache/gdu/incremental/
```

You can customize this location with:
```bash
gdu --incremental --incremental-path /custom/cache/path /mnt/storage
```

The cache uses a lightweight key-value store (BadgerDB) that:
- Efficiently stores metadata for millions of directories
- Automatically compacts and cleans up old data
- Provides fast lookups (sub-millisecond)
- Uses minimal disk space (typically <1% of scanned data size)

### What Gets Cached

For each directory, gdu caches:
- Directory path and modification time
- Size (apparent size) and usage (disk usage)
- Number of items in directory
- Flag status (errors, empty, etc.)
- List of child files and directories with metadata
- Cache timestamp and scan duration

Gdu does **not** cache:
- File contents (only metadata)
- Symbolic link targets (they are followed on demand)
- Real-time statistics (these are recalculated)

## Command-Line Flags

### Core Incremental Caching Flags

#### `--incremental`
Enable incremental caching for the current scan.

```bash
gdu --incremental /path/to/scan
```

**Default**: Disabled
**Cache Location**: `~/.cache/gdu/incremental/`

---

#### `--incremental-path <path>`
Specify a custom cache storage location.

```bash
gdu --incremental --incremental-path /fast/ssd/cache /mnt/nfs-storage
```

**Default**: `~/.cache/gdu/incremental/`
**Tip**: Place cache on fast local storage (SSD) for best performance

---

#### `--cache-max-age <duration>`
Set maximum age for cached entries. Entries older than this are automatically invalidated.

```bash
# Expire cache after 24 hours
gdu --incremental --cache-max-age 24h /mnt/storage

# Expire cache after 7 days
gdu --incremental --cache-max-age 168h /mnt/storage
```

**Default**: No expiration (0)
**Format**: Duration string (e.g., `1h`, `24h`, `168h`, `30m`)
**Use Case**: Ensure data freshness even for unchanged directories

---

#### `--force-full-scan`
Force a complete rescan, ignoring all cached data (but still update the cache).

```bash
gdu --incremental --force-full-scan /mnt/storage
```

**Default**: Disabled
**Use Case**: Weekly deep scans to ensure accuracy

---

#### `--show-cache-stats`
Display detailed cache statistics after the scan.

```bash
gdu --incremental --show-cache-stats /mnt/storage
```

**Default**: Disabled
**Output**: Cache hits, misses, I/O reduction, scan time, etc.

---

### I/O Throttling Flags

#### `--max-iops <number>`
Limit the maximum I/O operations per second.

```bash
# Limit to 100 IOPS
gdu --incremental --max-iops 100 /mnt/shared-nfs
```

**Default**: Unlimited (0)
**Use Case**: Protect shared storage from excessive load
**Note**: Applies to directory reads during scanning

---

#### `--io-delay <duration>`
Add a fixed delay between directory scans.

```bash
# 10ms delay between directory reads
gdu --incremental --io-delay 10ms /mnt/storage

# 100ms delay for very cautious scanning
gdu --incremental --io-delay 100ms /mnt/storage
```

**Default**: No delay (0)
**Format**: Duration string (e.g., `10ms`, `100ms`, `1s`)
**Use Case**: Alternative to max-iops for rate limiting

## Best Practices

### 1. Set Appropriate Cache Max Age

For data that changes frequently:
```bash
gdu --incremental --cache-max-age 6h /mnt/active-storage
```

For archival storage:
```bash
gdu --incremental --cache-max-age 168h /mnt/archives
```

### 2. Use I/O Throttling on Shared Storage

Limit IOPS to avoid impacting other users:
```bash
gdu --incremental --max-iops 100 /mnt/shared-nfs
```

For even gentler scanning:
```bash
gdu --incremental --max-iops 50 --io-delay 20ms /mnt/shared-nfs
```

### 3. Run Periodic Deep Scans

Weekly deep scan to ensure accuracy:
```bash
# Add to cron: Weekly full scan on Sunday at 2 AM
0 2 * * 0 gdu --incremental --force-full-scan /mnt/storage
```

Daily incremental scans:
```bash
# Daily scan at 6 AM
0 6 * * * gdu --incremental /mnt/storage
```

### 4. Monitor Cache Statistics

Regularly check cache performance:
```bash
gdu --incremental --show-cache-stats /mnt/storage
```

Good cache performance indicators:
- **Cache hit rate >90%** - excellent cache efficiency
- **I/O reduction >90%** - significant performance improvement
- **Scan time <10% of initial scan** - cache is working well

Poor cache performance indicators:
- **Cache hit rate <50%** - data changing too frequently
- **Many rescans** - consider shorter cache-max-age
- **Slow scans despite cache** - check cache storage performance

### 5. Place Cache on Fast Storage

For optimal performance, store cache on local SSD:
```bash
gdu --incremental --incremental-path /fast/ssd/cache /mnt/slow-nfs
```

Avoid placing cache on:
- Network filesystems (defeats the purpose)
- Slow rotational drives
- Filesystems that will be unmounted frequently

### 6. Clean Up Old Cache Data

The cache automatically manages itself, but you can manually clear it:
```bash
# Remove all cache data
rm -rf ~/.cache/gdu/incremental/

# Remove cache for specific path
rm -rf ~/.cache/gdu/incremental/mnt/storage/
```

Cache cleanup is useful when:
- Directories have been moved or deleted
- Cache corruption is suspected
- Disk space is needed

## Troubleshooting

### Cache Not Being Used

**Symptoms**: Low cache hit rate, slow scans, no performance improvement

**Possible Causes**:

1. **First Scan**: The initial scan always populates the cache
   - **Solution**: Run a second scan to see cache benefits

2. **Data Changing Frequently**: High directory modification rate
   - **Solution**: Check cache stats to see rescan percentage
   - **Solution**: Consider if incremental caching is appropriate for this data

3. **Cache Expired**: Cache max age setting is too short
   - **Solution**: Increase `--cache-max-age` or remove it

4. **Force Full Scan Enabled**: `--force-full-scan` flag is set
   - **Solution**: Remove the flag for normal scans

### Performance Issues

**Symptoms**: Scans are slower than expected even with cache

**Possible Causes**:

1. **Cache on Slow Storage**: Cache stored on slow filesystem
   - **Solution**: Move cache to local SSD with `--incremental-path`

2. **I/O Throttling Too Aggressive**: max-iops or io-delay too restrictive
   - **Solution**: Increase max-iops or reduce io-delay

3. **Low Cache Hit Rate**: Most directories being rescanned
   - **Solution**: Check `--show-cache-stats` for rescan percentage
   - **Solution**: Verify data isn't changing constantly

4. **Network Latency**: High latency to network storage
   - **Solution**: This is the expected use case; ensure cache is working
   - **Solution**: Check cache hit rate with `--show-cache-stats`

### Cache Corruption or Errors

**Symptoms**: Errors when reading cache, incorrect data, crashes

**Possible Causes**:

1. **Disk Full**: No space for cache storage
   - **Solution**: Free up disk space or use different cache location

2. **Permission Issues**: Cannot write to cache directory
   - **Solution**: Check permissions on `~/.cache/gdu/incremental/`
   - **Solution**: Create directory with proper permissions

3. **Corrupted Cache Files**: BadgerDB corruption
   - **Solution**: Delete cache and rescan: `rm -rf ~/.cache/gdu/incremental/`

4. **Concurrent Access**: Multiple gdu instances using same cache
   - **Solution**: Use separate cache paths for concurrent scans
   - **Solution**: Wait for one scan to complete

### Debugging with Cache Statistics

Enable detailed statistics to diagnose issues:
```bash
gdu --incremental --show-cache-stats --log-file gdu.log /mnt/storage
```

Analyze the statistics:
- **High cache miss rate**: Normal for first scan, investigate if persistent
- **Many expired entries**: Consider increasing cache-max-age
- **Low I/O reduction**: Indicates cache not providing benefit
- **Long scan time**: Check cache storage performance

Check log file for errors:
```bash
grep -i error gdu.log
grep -i cache gdu.log
```

## Performance Tuning

### Optimal Cache Max Age Settings

Choose based on your data change frequency:

| Data Type | Recommended Max Age | Rationale |
|-----------|-------------------|-----------|
| Archival storage | 7-30 days | Changes very infrequently |
| Log directories | 24-48 hours | Accumulates over time |
| Backup storage | 3-7 days | Weekly backup cycles |
| Active projects | 1-6 hours | Frequent changes |
| Home directories | 12-24 hours | Regular user activity |

Example configurations:
```bash
# Archives - weekly validation
gdu --incremental --cache-max-age 168h /mnt/archives

# Logs - daily validation
gdu --incremental --cache-max-age 24h /var/log

# Active projects - hourly validation
gdu --incremental --cache-max-age 1h /mnt/projects
```

### I/O Throttling Configuration

Choose based on storage system and load:

| Storage Type | max-iops | io-delay | Rationale |
|--------------|----------|----------|-----------|
| Dedicated NFS | 500-1000 | 0 | Can handle high IOPS |
| Shared NFS | 100-200 | 10-20ms | Moderate load |
| Congested NFS | 50-100 | 20-50ms | Minimize impact |
| Cloud storage | 100-500 | 0-10ms | API rate limits |

Example configurations:
```bash
# Dedicated NFS - aggressive scanning
gdu --incremental --max-iops 1000 /mnt/dedicated-nfs

# Shared NFS - balanced approach
gdu --incremental --max-iops 200 --io-delay 10ms /mnt/shared-nfs

# Congested NFS - gentle scanning
gdu --incremental --max-iops 50 --io-delay 50ms /mnt/congested-nfs
```

### Cache Location Optimization

For best performance, consider these cache location strategies:

1. **Local SSD** (best performance):
   ```bash
   gdu --incremental --incremental-path /home/user/.cache/gdu /mnt/nfs
   ```

2. **RAM disk** (fastest, but not persistent):
   ```bash
   # Create RAM disk first
   mkdir /tmp/gdu-cache
   gdu --incremental --incremental-path /tmp/gdu-cache /mnt/nfs
   ```

3. **Separate disk** (good for isolation):
   ```bash
   gdu --incremental --incremental-path /cache/gdu /mnt/nfs
   ```

## Examples

### Example 1: Daily NFS Monitoring

Scan a shared NFS mount daily to track storage usage:

```bash
#!/bin/bash
# daily-scan.sh

# Incremental scan with cache
gdu --incremental \
    --incremental-path ~/.cache/gdu/nfs \
    --cache-max-age 24h \
    --max-iops 200 \
    --show-cache-stats \
    /mnt/nfs-storage
```

Add to crontab:
```bash
# Daily at 6 AM
0 6 * * * /home/user/daily-scan.sh
```

### Example 2: Weekly Deep Scan

Run a weekly deep scan to ensure accuracy, plus daily incremental scans:

```bash
#!/bin/bash
# weekly-deep-scan.sh

# Force full scan on Sunday
gdu --incremental \
    --force-full-scan \
    --incremental-path ~/.cache/gdu/storage \
    --show-cache-stats \
    /mnt/storage
```

```bash
#!/bin/bash
# daily-quick-scan.sh

# Incremental scan on other days
gdu --incremental \
    --incremental-path ~/.cache/gdu/storage \
    --cache-max-age 168h \
    --show-cache-stats \
    /mnt/storage
```

Crontab:
```bash
# Daily incremental scan at 6 AM
0 6 * * 1-6 /home/user/daily-quick-scan.sh

# Weekly deep scan on Sunday at 2 AM
0 2 * * 0 /home/user/weekly-deep-scan.sh
```

### Example 3: Multi-User Environment

Multiple users scanning shared storage with separate caches:

```bash
# User 1
gdu --incremental \
    --incremental-path ~/.cache/gdu/shared \
    --max-iops 100 \
    /mnt/shared

# User 2
gdu --incremental \
    --incremental-path ~/.cache/gdu/shared \
    --max-iops 100 \
    /mnt/shared
```

Each user maintains their own cache, limiting I/O impact on shared storage.

### Example 4: Large Archive Scan

Scan a large archival storage system that rarely changes:

```bash
gdu --incremental \
    --incremental-path /fast/ssd/cache/archives \
    --cache-max-age 720h \
    --max-iops 500 \
    --show-cache-stats \
    /mnt/archives
```

Key features:
- Cache on fast SSD for optimal performance
- 30-day cache max age (archives change rarely)
- Moderate IOPS limit (archives can handle more load)
- Show stats to verify 90%+ cache hit rate

### Example 5: Non-Interactive Monitoring Script

Generate daily storage reports:

```bash
#!/bin/bash
# storage-report.sh

OUTPUT_FILE="/var/reports/storage-$(date +%Y%m%d).json"

gdu --incremental \
    --non-interactive \
    --no-progress \
    --output-file "$OUTPUT_FILE" \
    --incremental-path ~/.cache/gdu/reports \
    --cache-max-age 24h \
    /mnt/storage

echo "Storage report generated: $OUTPUT_FILE"
```

### Example 6: I/O Throttled Background Scan

Run a low-priority background scan that doesn't impact other users:

```bash
# Very gentle scan during business hours
nice -n 19 gdu --incremental \
    --max-iops 50 \
    --io-delay 100ms \
    --non-interactive \
    --summarize \
    /mnt/shared-storage
```

Features:
- `nice -n 19`: Lowest CPU priority
- `--max-iops 50`: Very low IOPS limit
- `--io-delay 100ms`: Additional throttling
- `--non-interactive`: No TUI overhead

## Configuration File

You can also configure incremental caching in your `~/.gdu.yaml`:

```yaml
# Enable incremental caching by default
use-incremental: true

# Set default cache path
incremental-path: /fast/ssd/.cache/gdu

# Default cache max age (24 hours)
cache-max-age: 24h

# I/O throttling for shared storage
max-iops: 200
io-delay: 10ms
```

Then simply run:
```bash
gdu /mnt/storage
```

## Advanced Topics

### Understanding I/O Reduction

The I/O reduction percentage shown in cache statistics represents:

```
I/O Reduction = (Bytes From Cache / (Bytes Scanned + Bytes From Cache)) × 100
```

Example:
- Bytes Scanned: 100 MB (new/changed data)
- Bytes From Cache: 900 MB (unchanged data)
- I/O Reduction: 90%

This means the scan only needed to read 10% of the data from disk.

### Cache Statistics Explained

| Statistic | Meaning |
|-----------|---------|
| Total Directories | Total number of directories processed |
| Cache Hits | Directories loaded from cache (no I/O) |
| Cache Misses | Directories not in cache (required I/O) |
| Cache Expired | Directories with expired cache entries |
| Directories Rescanned | Directories rescanned due to mtime change |
| Bytes Scanned | Data read from filesystem (I/O performed) |
| Bytes From Cache | Data loaded from cache (no I/O) |
| I/O Reduction | Percentage of data loaded from cache |
| Total Scan Time | Wall clock time for entire scan |

### Feature Compatibility

Incremental caching is compatible with most gdu features:

| Feature | Compatible | Notes |
|---------|-----------|-------|
| `--output-file` | Yes | Export includes all scanned data |
| `--input-file` | Yes | Can import previously exported data |
| `--sequential` | Yes | Use separate analyzers |
| `--use-storage` | No | Cannot use both (will error) |
| `--no-cross` | Yes | Cache respects filesystem boundaries |
| `--ignore-dirs` | Yes | Ignored directories not cached |
| `--ignore-dir-patterns` | Yes | Patterns applied before caching |
| `--follow-symlinks` | Yes | Symlink targets evaluated on demand |
| Interactive TUI | Yes | Fully supported |
| Non-interactive mode | Yes | Works seamlessly |

## FAQ

**Q: How much disk space does the cache use?**

A: Typically less than 1% of the scanned data size. For 1 TB of scanned data, expect 1-10 GB of cache data.

**Q: Can I use incremental caching with local SSD?**

A: You can, but it's not recommended. The parallel analyzer is already very fast on SSDs, and caching adds unnecessary overhead.

**Q: What happens if the cache becomes corrupted?**

A: Gdu will detect the corruption, log an error, and fall back to a full scan. You can manually delete the cache to start fresh.

**Q: Can multiple gdu instances share the same cache?**

A: No, this can cause corruption. Use separate cache paths for concurrent scans.

**Q: Does cache persist across reboots?**

A: Yes, the cache is stored on disk and persists across reboots.

**Q: How do I know if caching is actually working?**

A: Use `--show-cache-stats` to see cache hit rate and I/O reduction. A good cache hit rate is >90%.

**Q: What's the difference between `--max-iops` and `--io-delay`?**

A: `--max-iops` limits operations per second dynamically, while `--io-delay` adds a fixed delay between operations. They can be used together.

**Q: Should I use `--force-full-scan` regularly?**

A: Yes, consider running it weekly to ensure cache accuracy and detect any subtle changes.

## See Also

- [gdu GitHub Repository](https://github.com/dundee/gdu)
- [gdu Documentation](https://github.com/dundee/gdu#readme)
- [Configuration Guide](https://github.com/dundee/gdu/blob/master/configuration)
- [Issue Tracker](https://github.com/dundee/gdu/issues)

## Getting Help

If you encounter issues with incremental caching:

1. Check this guide for troubleshooting steps
2. Run with `--show-cache-stats` to see cache performance
3. Check log files with `--log-file gdu.log`
4. Search existing [GitHub issues](https://github.com/dundee/gdu/issues)
5. Open a new issue with:
   - Gdu version (`gdu --version`)
   - Command used
   - Cache statistics output
   - Relevant log excerpts
   - Description of the problem

## License

Incremental caching is part of gdu and is licensed under the MIT License.
