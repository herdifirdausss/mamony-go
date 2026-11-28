package secretfile

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpenPrivateAcceptsOwnerOnlyRegularFiles(t *testing.T) {
	for _, mode := range []os.FileMode{0400, 0600} {
		t.Run(mode.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credential")
			writePrivateTestFile(t, path, mode)

			file, err := OpenPrivate(path)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = file.Close() }()

			got, err := io.ReadAll(file)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != "secret" {
				t.Fatalf("OpenPrivate() read %q, want %q", got, "secret")
			}
		})
	}
}

func TestOpenPrivateRejectsInvalidPaths(t *testing.T) {
	for _, path := range []string{"", "relative/credential"} {
		if file, err := OpenPrivate(path); err == nil {
			if file != nil {
				_ = file.Close()
			}
			t.Errorf("OpenPrivate(%q) succeeded, want error", path)
		}
	}
}

func TestOpenPrivateRejectsNonRegularFiles(t *testing.T) {
	dir := t.TempDir()
	if file, err := OpenPrivate(dir); err == nil {
		if file != nil {
			_ = file.Close()
		}
		t.Fatal("OpenPrivate(directory) succeeded, want error")
	}

	target := filepath.Join(dir, "target")
	writePrivateTestFile(t, target, 0600)
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	if file, err := OpenPrivate(link); err == nil {
		if file != nil {
			_ = file.Close()
		}
		t.Fatal("OpenPrivate(symlink) succeeded, want error")
	}
}

func TestOpenPrivateRejectsInsecurePermissions(t *testing.T) {
	for _, mode := range []os.FileMode{0000, 0200, 0640, 0604} {
		t.Run(mode.String(), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "credential")
			writePrivateTestFile(t, path, mode)

			if file, err := OpenPrivate(path); err == nil {
				if file != nil {
					_ = file.Close()
				}
				t.Fatalf("OpenPrivate(mode %04o) succeeded, want error", mode.Perm())
			}
		})
	}
}

func TestOpenPrivateErrorDoesNotExposePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret-token-must-not-appear")
	_, err := OpenPrivate(path)
	if err == nil {
		t.Fatal("OpenPrivate(missing path) succeeded, want error")
	}
	if strings.Contains(err.Error(), filepath.Base(path)) {
		t.Fatalf("OpenPrivate() error exposes the path: %q", err)
	}
}

func writePrivateTestFile(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.WriteFile(path, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// SF-07 and SF-08 remain blocked: this test-only slice cannot inject read or
// stat failures at deterministic points in the concrete file implementation.
func TestReadPrivateLimited(t *testing.T) {
	t.Run("SF-01_ExactContents", func(t *testing.T) {
		const contents = "synthetic\x00credential\n"
		for _, mode := range []os.FileMode{0400, 0600} {
			t.Run(mode.String(), func(t *testing.T) {
				path := writeLimitedReaderTestFile(t, contents, mode)
				assertLimitedReaderContents(t, path, 512, contents)
			})
		}
	})

	bounds := []struct {
		name  string
		limit int
	}{
		{name: "1", limit: 1},
		{name: "512", limit: 512},
		{name: "2048", limit: 2048},
		{name: "4096", limit: 4096},
	}
	t.Run("SF-02_InclusiveLimit", func(t *testing.T) {
		for _, bound := range bounds {
			t.Run(bound.name, func(t *testing.T) {
				contents := strings.Repeat("x", bound.limit)
				path := writeLimitedReaderTestFile(t, contents, 0600)
				assertLimitedReaderContents(t, path, int64(bound.limit), contents)
			})
		}
	})
	t.Run("SF-03_OversizedFile", func(t *testing.T) {
		for _, bound := range bounds {
			t.Run(bound.name, func(t *testing.T) {
				path := writeLimitedReaderTestFile(t, strings.Repeat("x", bound.limit+1), 0600)
				assertLimitedReaderRejected(t, path, int64(bound.limit))
			})
		}
	})
	t.Run("SF-04_EmptyFile", func(t *testing.T) {
		path := writeLimitedReaderTestFile(t, "", 0600)
		assertLimitedReaderRejected(t, path, 512)
	})
	t.Run("SF-05_InvalidLimit", func(t *testing.T) {
		path := writeLimitedReaderTestFile(t, "synthetic", 0600)
		for _, tc := range []struct {
			name  string
			limit int64
		}{
			{name: "Zero", limit: 0},
			{name: "Negative", limit: -1},
			{name: "MinInt64", limit: -1 << 63},
			{name: "MaxInt64", limit: 1<<63 - 1},
		} {
			t.Run(tc.name, func(t *testing.T) {
				assertLimitedReaderRejected(t, path, tc.limit)
			})
		}
	})
	t.Run("SF-06_InvalidFileProperties", func(t *testing.T) {
		t.Run("EmptyPath", func(t *testing.T) {
			assertLimitedReaderRejected(t, "", 512)
		})
		t.Run("RelativePath", func(t *testing.T) {
			assertLimitedReaderRejected(t, "relative/credential", 512)
		})
		t.Run("MissingFile", func(t *testing.T) {
			assertLimitedReaderRejected(t, filepath.Join(t.TempDir(), "missing"), 512)
		})
		t.Run("Directory", func(t *testing.T) {
			assertLimitedReaderRejected(t, t.TempDir(), 512)
		})
		t.Run("FinalSymlink", func(t *testing.T) {
			target := writeLimitedReaderTestFile(t, "synthetic", 0600)
			link := filepath.Join(filepath.Dir(target), "link")
			if err := os.Symlink(target, link); err != nil {
				t.Fatal("cannot create required synthetic symlink fixture")
			}
			assertLimitedReaderRejected(t, link, 512)
		})
		for _, mode := range []os.FileMode{0000, 0200, 0640, 0604} {
			t.Run("Permission_"+mode.String(), func(t *testing.T) {
				path := writeLimitedReaderTestFile(t, "synthetic", mode)
				assertLimitedReaderRejected(t, path, 512)
			})
		}
	})
	t.Run("SF-09_GenericError", func(t *testing.T) {
		const pathSentinel = "synthetic-path-must-not-appear"
		const contentSentinel = "synthetic-data-must-not-appear"
		path := filepath.Join(t.TempDir(), pathSentinel)
		if err := os.WriteFile(path, []byte(contentSentinel), 0600); err != nil {
			t.Fatal("cannot create synthetic redaction fixture")
		}
		for _, tc := range []struct {
			name  string
			path  string
			limit int64
		}{
			{name: "Oversized", path: path, limit: 1},
			{name: "InvalidLimit", path: path, limit: 0},
			{name: "MissingFile", path: filepath.Join(t.TempDir(), pathSentinel), limit: 512},
		} {
			t.Run(tc.name, func(t *testing.T) {
				got, err := ReadPrivateLimited(tc.path, tc.limit)
				if got != nil || err == nil {
					t.Fatal("expected rejection without returned contents")
				}
				if strings.Contains(err.Error(), pathSentinel) || strings.Contains(err.Error(), contentSentinel) {
					t.Fatal("error exposes synthetic sensitive information")
				}
				if err != errUnavailable || err.Error() != "private secret file unavailable" {
					t.Fatal("expected the generic private-file error")
				}
			})
		}
	})
}

func writeLimitedReaderTestFile(t *testing.T, contents string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credential")
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal("cannot create synthetic file fixture")
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal("cannot set synthetic file permissions")
	}
	return path
}

func assertLimitedReaderContents(t *testing.T, path string, limit int64, want string) {
	t.Helper()
	got, err := ReadPrivateLimited(path, limit)
	if err != nil || string(got) != want {
		t.Fatal("expected exact synthetic contents without error")
	}
}

func assertLimitedReaderRejected(t *testing.T, path string, limit int64) {
	t.Helper()
	got, err := ReadPrivateLimited(path, limit)
	if got != nil || err != errUnavailable || err.Error() != "private secret file unavailable" {
		t.Fatal("expected generic rejection without returned contents")
	}
}
