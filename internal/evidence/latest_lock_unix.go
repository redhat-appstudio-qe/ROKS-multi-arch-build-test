//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package evidence

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func acquireLatestUpdateLock(root string) (func(), error) {
	lockPath := filepath.Join(root, ".latest.lock")
	fd, err := unix.Open(lockPath, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	lockFile := os.NewFile(uintptr(fd), lockPath)
	if lockFile == nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("open lock file: invalid file descriptor")
	}
	info, err := lockFile.Stat()
	if err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("inspect lock file: %w", err)
	}
	if !info.Mode().IsRegular() {
		_ = lockFile.Close()
		return nil, fmt.Errorf("lock path is not a regular file")
	}
	if err := unix.Flock(fd, unix.LOCK_EX); err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("acquire file lock: %w", err)
	}
	return func() {
		_ = unix.Flock(fd, unix.LOCK_UN)
		_ = lockFile.Close()
	}, nil
}
