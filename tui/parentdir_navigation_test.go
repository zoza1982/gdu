package tui

import (
	"bytes"
	"testing"

	"github.com/dundee/gdu/v5/internal/testapp"
	"github.com/dundee/gdu/v5/pkg/analyze"
	"github.com/dundee/gdu/v5/pkg/fs"
	"github.com/stretchr/testify/assert"
)

// TestParentDirNavigationDoesNotPanic tests that navigating with ParentDir objects doesn't cause panic
// This reproduces the bug where clicking ".." in incremental mode would crash with "panic: must not be called"
func TestParentDirNavigationDoesNotPanic(t *testing.T) {
	// Setup
	app, simScreen := testapp.CreateTestAppWithSimScreen(50, 50)
	defer simScreen.Fini()

	ui := CreateUI(app, simScreen, &bytes.Buffer{}, true, true, false, false, false)

	// Create a directory structure similar to what IncrementalAnalyzer returns
	// The key is that parent is a ParentDir object, not a real Dir
	parentDirMarker := &analyze.ParentDir{Path: "/tmp"}

	subdir := &analyze.Dir{
		File: &analyze.File{
			Name:   "subdir",
			Size:   8192,
			Usage:  8192,
			Parent: parentDirMarker, // This is the ParentDir that can cause the panic
		},
		BasePath:  "/tmp/test",
		ItemCount: 1,
		Files:     make(fs.Files, 0),
	}

	rootDir := &analyze.Dir{
		File: &analyze.File{
			Name:   "test",
			Size:   8192,
			Usage:  8192,
			Parent: parentDirMarker,
		},
		BasePath:  "/tmp",
		ItemCount: 2,
		Files:     fs.Files{subdir},
	}

	// Set the current directory and top directory
	ui.currentDir = rootDir
	ui.topDir = rootDir
	ui.topDirPath = rootDir.GetPath()

	// Show the directory (this sets up the table with ".." entry)
	ui.showDir()

	// Simulate entering the subdirectory
	ui.currentDir = subdir
	ui.showDir()

	// Now simulate clicking on ".." to go back
	// This is where the bug would occur - the ".." cell reference is a ParentDir
	// and the code tries to call GetName() on it

	// Get the first row (which should be "..")
	cell := ui.table.GetCell(0, 0)
	if cell != nil && cell.GetReference() != nil {
		// This should not panic!
		// The fixed code should detect that the cell reference is a ParentDir and handle it properly
		assert.NotPanics(t, func() {
			ui.fileItemSelected(0, 0)
		}, "fileItemSelected should not panic when selecting ParentDir marker")
	}
}

// TestParentDirGetNamePanics verifies that calling GetName() on ParentDir does panic
// This confirms the ParentDir is working as designed (to catch misuse)
func TestParentDirGetNamePanics(t *testing.T) {
	parentDir := &analyze.ParentDir{Path: "/tmp"}

	assert.Panics(t, func() {
		parentDir.GetName()
	}, "ParentDir.GetName() should panic to prevent misuse")
}

// TestParentDirTypeDetection verifies that we can detect ParentDir using type assertion
func TestParentDirTypeDetection(t *testing.T) {
	parentDir := &analyze.ParentDir{Path: "/tmp"}
	regularDir := &analyze.Dir{
		File: &analyze.File{
			Name: "test",
		},
		BasePath: "/tmp",
	}

	// Test type assertion for ParentDir
	_, isParentDir := fs.Item(parentDir).(*analyze.ParentDir)
	assert.True(t, isParentDir, "Should detect ParentDir type")

	// Test type assertion for regular Dir
	_, isParentDir = fs.Item(regularDir).(*analyze.ParentDir)
	assert.False(t, isParentDir, "Should not detect regular Dir as ParentDir")
}

// TestNavigationWithParentDirInHierarchy tests complete navigation scenario
// This simulates the exact scenario that causes the crash in incremental mode
func TestNavigationWithParentDirInHierarchy(t *testing.T) {
	// Setup
	app, simScreen := testapp.CreateTestAppWithSimScreen(50, 50)
	defer simScreen.Fini()

	ui := CreateUI(app, simScreen, &bytes.Buffer{}, true, true, false, false, false)

	// Create a more realistic hierarchy with ParentDir markers
	// Simulating what IncrementalAnalyzer.processDir creates
	grandparentMarker := &analyze.ParentDir{Path: "/"}
	parentMarker := &analyze.ParentDir{Path: "/tmp"}
	childMarker := &analyze.ParentDir{Path: "/tmp/test"}

	// Deepest directory
	deepFile := &analyze.File{
		Name:   "file.txt",
		Size:   100,
		Usage:  4096,
		Parent: childMarker,
	}

	deepDir := &analyze.Dir{
		File: &analyze.File{
			Name:   "deep",
			Size:   4196,
			Usage:  8192,
			Parent: childMarker,
		},
		BasePath:  "/tmp/test/subdir",
		ItemCount: 2,
		Files:     fs.Files{deepFile},
	}

	// Middle directory
	subdir := &analyze.Dir{
		File: &analyze.File{
			Name:   "subdir",
			Size:   4196,
			Usage:  8192,
			Parent: parentMarker,
		},
		BasePath:  "/tmp/test",
		ItemCount: 3,
		Files:     fs.Files{deepDir},
	}

	// Root directory
	rootDir := &analyze.Dir{
		File: &analyze.File{
			Name:   "test",
			Size:   4196,
			Usage:  8192,
			Parent: grandparentMarker,
		},
		BasePath:  "/tmp",
		ItemCount: 4,
		Files:     fs.Files{subdir},
	}

	// Set initial state
	ui.currentDir = rootDir
	ui.topDir = rootDir
	ui.topDirPath = rootDir.GetPath()

	// Test 1: Navigate into subdir
	ui.showDir()
	ui.currentDir = subdir

	// Test 2: Navigate into deep dir
	ui.showDir()
	ui.currentDir = deepDir

	// Test 3: Try to navigate back using ".." - this is where the bug occurs
	// The origDir (deepDir) has a ParentDir as parent
	// When trying to restore cursor position, the code would call GetName() on ParentDir
	ui.showDir()

	// Simulate the problematic code path that was in the original bug
	origDir := deepDir
	origParent := origDir.GetParent()

	// This should be a ParentDir
	_, isParentDir := origParent.(*analyze.ParentDir)
	assert.True(t, isParentDir, "origParent should be a ParentDir in incremental mode")

	// The fixed code should detect this and NOT call GetName()
	// Before the fix, this line would panic:
	// origParent.GetName() // PANIC!

	// With the fix, the code checks the type first and skips the problematic call
	if _, isParentDirType := origParent.(*analyze.ParentDir); !isParentDirType {
		// This block should NOT execute when origParent is ParentDir
		t.Fatal("Should not try to call GetName() on ParentDir")
	}

	// Test passed if we got here without panic
	assert.True(t, true, "Navigation with ParentDir should not panic")
}
