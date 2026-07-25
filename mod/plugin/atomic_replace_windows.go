//go:build windows

package plugin

import "golang.org/x/sys/windows"

func replaceFileAtomic(oldPath, newPath string) error {
	oldPathUTF16, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	newPathUTF16, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(
		oldPathUTF16,
		newPathUTF16,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
}

func syncDirectory(string) error {
	// MoveFileEx with MOVEFILE_WRITE_THROUGH flushes the replacement metadata.
	return nil
}
