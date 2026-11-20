package main

import "testing"

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
