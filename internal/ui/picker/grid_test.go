package picker

import (
	"io/fs"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
)

// MockDirEntry for testing
type MockDirEntry struct {
	name  string
	isDir bool
}

func (m MockDirEntry) Name() string               { return m.name }
func (m MockDirEntry) IsDir() bool                { return m.isDir }
func (m MockDirEntry) Type() fs.FileMode          { return 0 }
func (m MockDirEntry) Info() (fs.FileInfo, error) { return nil, nil }

func TestNewFileGrid(t *testing.T) {
	test.NewApp()
	grid := NewFileGrid()

	assert.NotNil(t, grid, "FileGrid should be created")
	assert.NotNil(t, grid.Table, "Grid should have a Table widget")
	assert.Equal(t, 5, grid.Columns, "Default columns should be 5")
}

func TestFileGrid_SetItems(t *testing.T) {
	test.NewApp()
	grid := NewFileGrid()

	items := []fs.DirEntry{
		MockDirEntry{name: "Folder1", isDir: true},
		MockDirEntry{name: "File1.jpg", isDir: false},
	}

	grid.SetItems(items, "/tmp")

	assert.Equal(t, 2, len(grid.Items), "Items should be set")
	assert.Equal(t, "/tmp", grid.CurrentDir, "CurrentDir should be set")
	assert.Equal(t, "", grid.Selected, "Selection should be reset")
}
