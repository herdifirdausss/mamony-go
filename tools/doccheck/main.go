// Command doccheck verifies local links in repository documentation.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var link = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)

func main() {
	repo := os.DirFS(".")
	for _, name := range []string{"README.md", "AGENTS.md", ".github/pull_request_template.md"} {
		if err := checkFile(repo, name); err != nil {
			fatal(err)
		}
	}
	for _, dir := range []string{"docs", ".agents/skills"} {
		err := fs.WalkDir(repo, dir, func(name string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.Type().IsRegular() && filepath.Ext(name) == ".md" {
				return checkFile(repo, name)
			}
			return nil
		})
		if err != nil {
			fatal(err)
		}
	}
	fmt.Println("local documentation links: PASS")
}

func checkFile(repo fs.FS, name string) error {
	doc, err := fs.ReadFile(repo, name)
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	return check(name, string(doc))
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
		// A path may be lexically in the repo but escape through a symlink.
		// Resolve it and recheck against the physical current directory.
		resolved, err := filepath.EvalSymlinks(dest)
		if err != nil {
			return fmt.Errorf("%s: broken local link %s: %w", path, target, err)
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return fmt.Errorf("%s: resolve local link %s: %w", path, target, err)
		}
		root, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("%s: determine repo root: %w", path, err)
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil || !filepath.IsLocal(rel) {
			return fmt.Errorf("%s: local link escapes repo via symlink: %s", path, target)
		}
		if _, err := os.Stat(resolved); err != nil {
			return fmt.Errorf("%s: broken local link %s: %w", path, target, err)
		}
	}
	return nil
}
func fatal(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(1) }
