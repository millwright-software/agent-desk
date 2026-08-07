package session

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetDirectoryCompletions(t *testing.T) {
	// Setup temporary directory structure
	tmpDir := t.TempDir()

	// Create some directories
	dirs := []string{
		"projects",
		"playground",
		"personal",
		"work/agent-desk",
		"work/other",
	}
	for _, d := range dirs {
		err := os.MkdirAll(filepath.Join(tmpDir, d), 0755)
		require.NoError(t, err)
	}

	// Create some files (should be ignored)
	files := []string{
		"README.md",
		"projects/todo.txt",
	}
	for _, f := range files {
		err := os.WriteFile(filepath.Join(tmpDir, f), []byte("test"), 0644)
		require.NoError(t, err)
	}

	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Absolute path prefix",
			input:    filepath.Join(tmpDir, "p"),
			expected: []string{filepath.Join(tmpDir, "personal"), filepath.Join(tmpDir, "playground"), filepath.Join(tmpDir, "projects")},
		},
		{
			name:     "Nested absolute path",
			input:    filepath.Join(tmpDir, "work/a"),
			expected: []string{filepath.Join(tmpDir, "work/agent-desk")},
		},
		{
			name:     "No matches",
			input:    filepath.Join(tmpDir, "xyz"),
			expected: nil,
		},
		{
			name:     "Exact match directory (should return itself and any subdirs starting with it)",
			input:    filepath.Join(tmpDir, "projects"),
			expected: []string{filepath.Join(tmpDir, "projects")},
		},
		{
			name:     "Trailing slash lists directory contents",
			input:    filepath.Join(tmpDir, "work") + string(os.PathSeparator),
			expected: []string{filepath.Join(tmpDir, "work/agent-desk"), filepath.Join(tmpDir, "work/other")},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := GetDirectoryCompletions(tt.input)
			assert.NoError(t, err)
			sort.Strings(results)
			sort.Strings(tt.expected)
			assert.Equal(t, tt.expected, results)
		})
	}
}
