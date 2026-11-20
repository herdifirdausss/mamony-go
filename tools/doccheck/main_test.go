package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLocalLinkRules(t *testing.T) {
	t.Parallel()
	if err := check("main_test.go", "[source](main.go)"); err != nil {
		t.Fatal(err)
	}
	if err := check("docs/index.md", "[evil](../../etc/passwd)"); err == nil {
		t.Fatal("expected path escape rejection")
	}
	if err := check("README.md", "[bad](not-exists.md)"); err == nil {
		t.Fatal("expected broken link")
	}
	if err := check("README.md", "[official](https://go.dev/doc/)"); err != nil {
		t.Fatal(err)
	}
}

func TestSymlinkOutsideRepositoryRejected(t *testing.T) {
	t.Parallel()
	target := filepath.Join(t.TempDir(), "external.md")
	if err := os.WriteFile(target, []byte("not part of repo"), 0600); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(".", "doccheck-symlink-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	link := filepath.Join(dir, "escape.md")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := check("main_test.go", "[outside]("+filepath.ToSlash(link)+")"); err == nil {
		t.Fatal("expected symlink escape rejection")
	}
}
