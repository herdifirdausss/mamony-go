package secretfile

import (
	"errors"
	"os"
	"path/filepath"
)

var errUnavailable = errors.New("private secret file unavailable")

// OpenPrivate opens an absolute path to a private regular file.
//
// It rejects a final-component symlink, group or world permissions, and path replacement
// between the metadata check and open. The caller must close the returned file.
func OpenPrivate(path string) (*os.File, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, errUnavailable
	}

	before, err := os.Lstat(path)
	if err != nil || !privateRegularFile(before.Mode()) {
		return nil, errUnavailable
	}

	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return nil, errUnavailable
	}
	defer func() { _ = root.Close() }()

	file, err := root.Open(filepath.Base(path))
	if err != nil {
		return nil, errUnavailable
	}

	after, err := file.Stat()
	if err != nil || !os.SameFile(before, after) || !privateRegularFile(after.Mode()) {
		_ = file.Close()
		return nil, errUnavailable
	}

	return file, nil
}

func privateRegularFile(mode os.FileMode) bool {
	return mode.IsRegular() && mode.Perm()&0077 == 0 && mode.Perm()&0400 != 0
}
