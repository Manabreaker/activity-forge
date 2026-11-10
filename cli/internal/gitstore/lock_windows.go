package gitstore

import (
	"fmt"
	"io"
	"os"
	"syscall"
)

func lockRepository(path string) (io.Closer, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// Share mode zero prevents a second process from opening the same lock file.
	// Windows releases the handle when the process exits, so crashes leave no lock.
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0,
		nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, fmt.Errorf("cannot acquire repository lock (another operation may be running): %w", err)
	}
	return os.NewFile(uintptr(h), path), nil
}
