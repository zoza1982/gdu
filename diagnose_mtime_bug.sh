#!/bin/bash
# Diagnostic script for incremental cache mtime detection bug
# This script reproduces the exact user scenario and checks if mtime detection works

set -e

echo "=========================================="
echo "GDU Mtime Detection Diagnostic"
echo "=========================================="
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
TEST_DIR="/tmp/gdu-test-cache"
CACHE_DIR="/tmp/gdu-cache-storage"
GDU_BIN="./dist/gdu"

# Check if gdu binary exists
if [ ! -f "$GDU_BIN" ]; then
    echo -e "${RED}ERROR: gdu binary not found at $GDU_BIN${NC}"
    echo "Please build gdu first with: make build"
    exit 1
fi

# Get gdu version
echo "=== GDU VERSION ==="
$GDU_BIN --version
echo ""

# Get system information
echo "=== SYSTEM INFORMATION ==="
echo "OS: $(uname -s) $(uname -r)"
echo "Platform: $(uname -m)"
echo ""

# Clean up any previous test data
echo "=== CLEANUP ==="
rm -rf "$TEST_DIR" "$CACHE_DIR"
echo "Removed old test directories"
echo ""

# Create test directory
echo "=== CREATING TEST DIRECTORY ==="
mkdir -p "$TEST_DIR"
echo "Created $TEST_DIR"
echo ""

# Check filesystem type
echo "=== FILESYSTEM TYPE ==="
df -T "$TEST_DIR" 2>/dev/null || df "$TEST_DIR"
echo ""

# Create initial directories (10 for faster test, user had 500)
echo "=== CREATING INITIAL DIRECTORIES ==="
for i in {1..10}; do
    mkdir "$TEST_DIR/dir$i"
done
echo "Created 10 initial empty directories (dir1..dir10)"
echo ""

# Wait for filesystem timestamp stability
echo "=== WAITING FOR FILESYSTEM TIMESTAMP STABILITY ==="
sleep 2
echo "Waited 2 seconds"
echo ""

# Get mtime before first scan
echo "=== MTIME BEFORE FIRST SCAN ==="
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    stat -f "mtime: %Sm" -t "%Y-%m-%d %H:%M:%S.%N" "$TEST_DIR"
    MTIME_BEFORE_SCAN=$(stat -f "%m" "$TEST_DIR")
else
    # Linux
    stat -c "mtime: %y" "$TEST_DIR"
    MTIME_BEFORE_SCAN=$(stat -c "%Y" "$TEST_DIR")
fi
echo "Timestamp (epoch): $MTIME_BEFORE_SCAN"
echo ""

# First scan
echo "=== FIRST SCAN (Building Cache) ==="
echo "Running: $GDU_BIN --incremental --incremental-path $CACHE_DIR --no-color $TEST_DIR"
$GDU_BIN --incremental --incremental-path "$CACHE_DIR" --no-color "$TEST_DIR" | head -20
echo "... (output truncated)"
echo ""

# Get mtime after first scan (should be unchanged)
echo "=== MTIME AFTER FIRST SCAN ==="
if [[ "$OSTYPE" == "darwin"* ]]; then
    stat -f "mtime: %Sm" -t "%Y-%m-%d %H:%M:%S.%N" "$TEST_DIR"
    MTIME_AFTER_SCAN=$(stat -f "%m" "$TEST_DIR")
else
    stat -c "mtime: %y" "$TEST_DIR"
    MTIME_AFTER_SCAN=$(stat -c "%Y" "$TEST_DIR")
fi
echo "Timestamp (epoch): $MTIME_AFTER_SCAN"

if [ "$MTIME_BEFORE_SCAN" != "$MTIME_AFTER_SCAN" ]; then
    echo -e "${YELLOW}WARNING: Mtime changed during scan!${NC}"
    echo "This might indicate filesystem or timing issues"
else
    echo -e "${GREEN}OK: Mtime unchanged during scan${NC}"
fi
echo ""

# Wait before modification
echo "=== WAITING BEFORE MODIFICATION ==="
sleep 2
echo "Waited 2 seconds"
echo ""

# Get mtime before modification
echo "=== MTIME BEFORE MODIFICATION ==="
if [[ "$OSTYPE" == "darwin"* ]]; then
    stat -f "mtime: %Sm" -t "%Y-%m-%d %H:%M:%S.%N" "$TEST_DIR"
    MTIME_BEFORE_MOD=$(stat -f "%m" "$TEST_DIR")
else
    stat -c "mtime: %y" "$TEST_DIR"
    MTIME_BEFORE_MOD=$(stat -c "%Y" "$TEST_DIR")
fi
echo "Timestamp (epoch): $MTIME_BEFORE_MOD"
echo ""

# Wait to ensure timestamp will change
sleep 2

# Modify directory - add two new directories
echo "=== ADDING NEW DIRECTORIES ==="
echo "Running: mkdir $TEST_DIR/dir11 $TEST_DIR/dir12"
mkdir "$TEST_DIR/dir11" "$TEST_DIR/dir12"
echo "Created dir11 and dir12"
echo ""

# List directories
echo "=== DIRECTORY LISTING ==="
ls -la "$TEST_DIR" | grep "^d" | tail -5
echo "... (showing last 5 directories)"
echo ""

# Get mtime after modification
echo "=== MTIME AFTER MODIFICATION ==="
if [[ "$OSTYPE" == "darwin"* ]]; then
    stat -f "mtime: %Sm" -t "%Y-%m-%d %H:%M:%S.%N" "$TEST_DIR"
    MTIME_AFTER_MOD=$(stat -f "%m" "$TEST_DIR")
else
    stat -c "mtime: %y" "$TEST_DIR"
    MTIME_AFTER_MOD=$(stat -c "%Y" "$TEST_DIR")
fi
echo "Timestamp (epoch): $MTIME_AFTER_MOD"
echo ""

# Check if mtime changed
echo "=== MTIME CHANGE VERIFICATION ==="
echo "Before modification: $MTIME_BEFORE_MOD"
echo "After modification:  $MTIME_AFTER_MOD"

if [ "$MTIME_BEFORE_MOD" = "$MTIME_AFTER_MOD" ]; then
    echo -e "${RED}ERROR: Mtime did NOT change after mkdir!${NC}"
    echo "Your filesystem may not update directory mtime for child creation"
    echo "This would explain why incremental caching doesn't detect changes"
    MTIME_CHANGED=false
else
    DIFF=$((MTIME_AFTER_MOD - MTIME_BEFORE_MOD))
    echo -e "${GREEN}SUCCESS: Mtime changed by $DIFF seconds${NC}"
    MTIME_CHANGED=true
fi
echo ""

# Second scan with cache stats
echo "=== SECOND SCAN (Using Cache) ==="
echo "Running: $GDU_BIN --incremental --incremental-path $CACHE_DIR --show-cache-stats --no-color $TEST_DIR"
OUTPUT=$($GDU_BIN --incremental --incremental-path "$CACHE_DIR" --show-cache-stats --no-color "$TEST_DIR")

# Show first 20 lines of output
echo "$OUTPUT" | head -20
echo ""

# Extract cache statistics
echo "=== CACHE STATISTICS ==="
echo "$OUTPUT" | grep -A 10 "Cache Statistics:" || echo "No cache statistics found in output"
echo ""

# Analyze results
echo "=========================================="
echo "=== DIAGNOSTIC RESULTS ==="
echo "=========================================="
echo ""

# Count directories in output
DIR_COUNT=$(echo "$OUTPUT" | grep -c "^.\s\+[0-9]\+.*dir[0-9]" || true)
echo "Directories found in output: $DIR_COUNT"
echo "Expected: 12 (10 original + 2 new)"

if [ "$DIR_COUNT" -ge 12 ]; then
    echo -e "${GREEN}✓ All directories detected${NC}"
else
    echo -e "${YELLOW}⚠ Not all directories detected${NC}"
fi
echo ""

# Check cache stats
CACHE_STATS=$(echo "$OUTPUT" | grep "Directories:")
echo "Cache stats line: $CACHE_STATS"

if echo "$CACHE_STATS" | grep -q "0 rescanned"; then
    RESCANNED=0
else
    RESCANNED=$(echo "$CACHE_STATS" | grep -oE "[0-9]+ rescanned" | grep -oE "[0-9]+" || echo "unknown")
fi

echo "Directories rescanned: $RESCANNED"
echo ""

# Final verdict
echo "=========================================="
echo "=== FINAL VERDICT ==="
echo "=========================================="
echo ""

if [ "$MTIME_CHANGED" = false ]; then
    echo -e "${RED}✗ FILESYSTEM ISSUE DETECTED${NC}"
    echo "Your filesystem does not update directory mtime when children are added."
    echo "This is why incremental caching cannot detect changes."
    echo ""
    echo "Possible causes:"
    echo "  - Network filesystem (NFS) with attribute caching"
    echo "  - Unusual filesystem configuration"
    echo "  - Filesystem bug"
    echo ""
    echo "Recommendations:"
    echo "  1. Try on a different filesystem (not /tmp)"
    echo "  2. Check mount options: mount | grep /tmp"
    echo "  3. Use --force-full-scan to bypass cache"
elif [ "$RESCANNED" = "0" ]; then
    echo -e "${RED}✗ BUG REPRODUCED${NC}"
    echo "Mtime changed but cache was not invalidated!"
    echo ""
    echo "This confirms the reported bug. Please report this with:"
    echo "  - Output of this diagnostic script"
    echo "  - GDU version (shown above)"
    echo "  - Filesystem type (shown above)"
elif [ "$RESCANNED" = "unknown" ]; then
    echo -e "${YELLOW}? INCONCLUSIVE${NC}"
    echo "Could not parse cache statistics from output"
    echo "Please review the output manually"
else
    echo -e "${GREEN}✓ WORKING CORRECTLY${NC}"
    echo "Mtime change was detected and directories were rescanned"
    echo "The incremental cache is functioning as expected"
fi

echo ""
echo "=========================================="
echo "Diagnostic complete. Keeping test data for inspection."
echo "Test directory: $TEST_DIR"
echo "Cache directory: $CACHE_DIR"
echo ""
echo "To clean up, run:"
echo "  rm -rf $TEST_DIR $CACHE_DIR"
echo "=========================================="
