//go:build darwin || linux

package gitstore

import (
	"errors"
	"fmt"
	"io"
	"os"
	"syscall"
)

type repositoryFileLock struct{ *os.File }

func (f repositoryFileLock) Close() error {
	// Unlock explicitly before closing: an in-flight fork may still hold a copy
	// of this descriptor until exec closes it, delaying an unlock by close alone.
	return errors.Join(syscall.Flock(int(f.Fd()), syscall.LOCK_UN), f.File.Close())
}

func lockRepository(path string) (io.Closer, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another activityforge operation holds the repository lock: %w", err)
	}
	return repositoryFileLock{f}, nil
}
