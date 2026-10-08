//go:build linux

package evidence

import "golang.org/x/sys/unix"

func exchangeLatestPaths(source, destination string) (bool, error) {
	err := unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, destination, unix.RENAME_EXCHANGE)
	if err == unix.ENOSYS || err == unix.EINVAL || err == unix.EOPNOTSUPP {
		return false, nil
	}
	return err == nil, err
}
