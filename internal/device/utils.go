package device

import "os"

// countFiles returns the number of non-directory entries in dir.
// Returns 0 if the directory does not exist or cannot be read.
func countFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			count++
		}
	}
	return count
}
