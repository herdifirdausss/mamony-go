package secretfile

import (
	"errors"
	"io"
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

// ReadPrivateLimited reads a non-empty private regular file with a strict byte limit.
// It rejects size changes observed during the read and returns a generic error
// that does not reveal the path or underlying filesystem error.
func ReadPrivateLimited(path string, maxBytes int64) ([]byte, error) {
	if maxBytes < 1 || maxBytes == 1<<63-1 {
		return nil, errUnavailable
	}

	file, err := OpenPrivate(path)
	if err != nil {
		return nil, errUnavailable
	}
	defer func() { _ = file.Close() }()

	before, err := file.Stat()
	if err != nil || before.Size() < 1 || before.Size() > maxBytes {
		return nil, errUnavailable
	}

	contents, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	after, statErr := file.Stat()
	if err != nil || statErr != nil || len(contents) == 0 ||
		int64(len(contents)) > maxBytes || int64(len(contents)) != before.Size() ||
		after.Size() != before.Size() {
		return nil, errUnavailable
	}
	return contents, nil
}

func privateRegularFile(mode os.FileMode) bool {
	return mode.IsRegular() && mode.Perm()&0077 == 0 && mode.Perm()&0400 != 0
}
