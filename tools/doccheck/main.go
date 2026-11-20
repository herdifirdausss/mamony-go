// Command doccheck verifies local links from the small repository entry documents.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var link = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)

func main() {
	for _, path := range []string{"README.md", "AGENTS.md", "docs/index.md", "docs/architecture.md", "docs/evidence/template.md"} {
		b, err := os.ReadFile(path)
		if err != nil {
			fatal(fmt.Errorf("read %s: %w", path, err))
		}
		if err := check(path, string(b)); err != nil {
			fatal(err)
		}
	}
	fmt.Println("local documentation links: PASS")
}

func check(path, contents string) error {
	for _, hit := range link.FindAllStringSubmatch(contents, -1) {
		target := hit[1]
		if strings.HasPrefix(target, "http:") || strings.HasPrefix(target, "https:") || strings.HasPrefix(target, "mailto:") || strings.HasPrefix(target, "#") {
			continue
		}
		target = strings.SplitN(target, "#", 2)[0]
		if target == "" {
			continue
		}
		if strings.HasPrefix(target, "/") {
			return fmt.Errorf("%s: absolute local link is disallowed: %s", path, target)
		}
		dest := filepath.Clean(filepath.Join(filepath.Dir(path), target))
		if dest == ".." || strings.HasPrefix(dest, ".."+string(filepath.Separator)) {
			return fmt.Errorf("%s: link escapes repo: %s", path, target)
		}
		if _, err := os.Stat(dest); err != nil {
			return fmt.Errorf("%s: broken local link %s: %w", path, target, err)
		}
	}
	return nil
}
func fatal(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(1) }
