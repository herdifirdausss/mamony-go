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
