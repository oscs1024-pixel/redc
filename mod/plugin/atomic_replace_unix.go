//go:build !windows

package plugin

import "os"

func replaceFileAtomic(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
