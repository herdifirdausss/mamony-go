// Command doccheck verifies local links and Markdown heading anchors.
package main

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

var link = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
var heading = regexp.MustCompile(`^#{1,6}[ \t]+(.+?)(?:[ \t]+#+)?[ \t]*$`)

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
	fmt.Println("local documentation paths and heading anchors: PASS")
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
		if strings.HasPrefix(target, "http:") || strings.HasPrefix(target, "https:") || strings.HasPrefix(target, "mailto:") {
			continue
		}
		local, fragment, hasFragment := strings.Cut(target, "#")
		source := contents
		if local != "" {
			if strings.HasPrefix(local, "/") {
				return fmt.Errorf("%s: absolute local link is disallowed: %s", path, target)
			}
			dest := filepath.Clean(filepath.Join(filepath.Dir(path), local))
			if dest == ".." || strings.HasPrefix(dest, ".."+string(filepath.Separator)) {
				return fmt.Errorf("%s: link escapes repo: %s", path, target)
			}
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
			info, err := os.Stat(resolved)
			if err != nil {
				return fmt.Errorf("%s: broken local link %s: %w", path, target, err)
			}
			if hasFragment && fragment != "" {
				if info.IsDir() {
					return fmt.Errorf("%s: heading anchor on a directory: %s", path, target)
				}
				doc, err := os.ReadFile(resolved)
				if err != nil {
					return fmt.Errorf("%s: read local link %s: %w", path, target, err)
				}
				source = string(doc)
			}
		}
		if hasFragment && fragment != "" {
			decoded, err := url.PathUnescape(fragment)
			if err != nil {
				return fmt.Errorf("%s: invalid heading anchor %s: %w", path, target, err)
			}
			if !headingAnchors(source)[decoded] {
				return fmt.Errorf("%s: missing heading anchor: %s", path, target)
			}
		}
	}
	return nil
}

func headingAnchors(contents string) map[string]bool {
	anchors := make(map[string]bool)
	seen := make(map[string]int)
	for _, line := range strings.Split(contents, "\n") {
		match := heading.FindStringSubmatch(line)
		if len(match) == 0 {
			continue
		}
		raw := strings.ToLower(strings.ReplaceAll(match[1], "`", ""))
		var b strings.Builder
		for _, r := range raw {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == ' ' {
				b.WriteRune(r)
			}
		}
		slug := strings.ReplaceAll(strings.TrimSpace(b.String()), " ", "-")
		if slug == "" {
			continue
		}
		number := seen[slug]
		seen[slug]++
		if number > 0 {
			slug = fmt.Sprintf("%s-%d", slug, number)
		}
		anchors[slug] = true
	}
	return anchors
}

func fatal(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(1) }
