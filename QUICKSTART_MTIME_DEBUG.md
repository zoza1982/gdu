# Quick Start: Debugging Mtime Detection Issue

## TL;DR

Run this diagnostic script to identify the problem:

```bash
cd /path/to/gdu
./diagnose_mtime_bug.sh
```

Read the output - it will tell you exactly what's wrong.

## What This Does

The script will:
1. Create a test directory with 10 subdirectories
2. Run gdu to cache them
3. Add 2 new subdirectories
4. Run gdu again to see if it detects the change
5. Tell you if the problem is:
   - Your filesystem not updating mtime
   - A bug in gdu (unlikely - we tested extensively)
   - Working correctly (you may have misread the output)

## Expected Output

### If Working Correctly

```
✓ WORKING CORRECTLY
Mtime change was detected and directories were rescanned
The incremental cache is functioning as expected
```

### If Filesystem Issue

```
✗ FILESYSTEM ISSUE DETECTED
Your filesystem does not update directory mtime when children are added.
This is why incremental caching cannot detect changes.

Possible causes:
  - Network filesystem (NFS) with attribute caching
  - Unusual filesystem configuration
  - Filesystem bug
```

### If Bug Confirmed

```
✗ BUG REPRODUCED
Mtime changed but cache was not invalidated!
```

## What to Do Next

### If Filesystem Issue

Your filesystem doesn't update directory mtime. This is not a gdu bug.

**Workaround:**
```bash
# Use --force-full-scan to bypass cache
gdu --incremental --force-full-scan /your/path
```

**Permanent solutions:**
1. Use a different filesystem (not NFS, not /tmp if it's tmpfs)
2. Check NFS mount options: `mount | grep nfs`
3. Look for `actimeo=` setting - it controls attribute cache time

### If Bug Confirmed

1. Save the diagnostic output
2. Run these commands and save output:
   ```bash
   ./dist/gdu --version
   uname -a
   df -T /tmp/gdu-test-cache  # or just df on macOS
   mount | grep /tmp
   ```
3. Report the bug with all this information

### If Working Correctly

You may have misread the cache stats. Check:
- Are you looking at the right scan's output?
- Did you wait between scans for mtime to change? (>1 second)
- Are the directories actually showing up?

## Manual Verification

If you want to verify manually that your filesystem updates mtime:

```bash
# Create test directory
mkdir /tmp/test-mtime
stat /tmp/test-mtime  # Note the mtime

# Wait
sleep 2

# Add subdirectory
mkdir /tmp/test-mtime/subdir

# Check mtime again
stat /tmp/test-mtime  # Should be different!
```

If the mtime didn't change, you have a filesystem issue, not a gdu issue.

## Understanding Cache Stats

When you see:
```
Directories: 503 total, 1 rescanned
```

This means:
- **503 total** = Total directories processed (root + 502 subdirectories)
- **1 rescanned** = Directories that were scanned from filesystem (not cache)

In the user's case:
- Expected: 1 rescanned (the root, because its mtime changed)
- Reported: 0 rescanned

If you see "0 rescanned" but new directories appear, run the diagnostic!

## Common Issues

### 1. NFS Attribute Caching

**Symptom:** `stat` shows old mtime, but directory was modified

**Check:**
```bash
mount | grep nfs
# Look for actimeo= option
```

**Fix:**
- Remount with shorter attribute cache: `actimeo=1`
- Or use local filesystem

### 2. Docker/Virtual Filesystem

**Symptom:** Directory is in Docker container or virtual filesystem

**Fix:**
- Test on native filesystem
- Some virtual filesystems don't update directory mtime

### 3. Extremely Fast Operations

**Symptom:** Modify directory immediately after first scan

**Fix:**
- Wait at least 1 second between scan and modification
- Some filesystems have 1-second mtime granularity

## Need Help?

1. Run `./diagnose_mtime_bug.sh`
2. Read the output
3. If issue persists, report with:
   - Full diagnostic output
   - `gdu --version`
   - `uname -a`
   - Filesystem type
   - Mount options (for network filesystems)

## Technical Details

For a deep dive into the investigation, see:
- `BUG_ANALYSIS_MTIME_DETECTION.md` - Detailed code analysis
- `MTIME_BUG_INVESTIGATION_SUMMARY.md` - Investigation summary
- `pkg/analyze/*_test.go` - Comprehensive test suite

All tests pass, confirming mtime detection works correctly in standard scenarios.
