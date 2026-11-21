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

func TestHeadingAnchorLinks(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		markdown  string
		wantError bool
	}{
		{"local heading", "# Overview\n[go](#overview)\n", false},
		{"duplicate heading", "# Overview\n# Overview\n[go](#overview-1)\n", false},
		{"missing local heading", "# Overview\n[bad](#missing)\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := check("main_test.go", tc.markdown)
			if (err != nil) != tc.wantError {
				t.Fatalf("check error=%v wantError=%v", err, tc.wantError)
			}
		})
	}

	// go test runs in this package directory, not the repository root.
	// Use a fixture within that directory to exercise real relative links.
	dir, err := os.MkdirTemp(".", "doccheck-anchors-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Error(err)
		}
	})
	target := filepath.Join(dir, "target.md")
	if err := os.WriteFile(target, []byte("# Verification\n"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.ToSlash(target)
	for _, tc := range []struct {
		fragment  string
		wantError bool
	}{
		{"verification", false},
		{"missing-heading", true},
	} {
		err := check("main_test.go", "[target]("+link+"#"+tc.fragment+")")
		if (err != nil) != tc.wantError {
			t.Fatalf("anchor=%q error=%v wantError=%v", tc.fragment, err, tc.wantError)
		}
	}
}
