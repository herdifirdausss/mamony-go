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
	for _, path := range []string{
		"README.md",
		"AGENTS.md",
		"docs/index.md",
		"docs/product.md",
		"docs/architecture.md",
		"docs/adr/README.md",
		"docs/adr/0001-go-modular-monolith.md",
		"docs/adr/0002-ledger-and-data-ownership.md",
		"docs/adr/0003-public-http-contract.md",
		"docs/contracts/ledger-gate-a.md",
		"docs/contracts/postgresql.md",
		"docs/delivery/00-platform.md",
		"docs/delivery/02-identity.md",
		"docs/delivery/03-screening-onboarding.md",
		"docs/delivery/04-fees-limits-accounts.md",
		"docs/delivery/05-provider.md",
		"docs/delivery/06-fx.md",
		"docs/delivery/07-money-flows.md",
		"docs/delivery/08-operations-events.md",
		"docs/delivery/09-risk-monitoring.md",
		"docs/delivery/10-settlement-accounting.md",
		"docs/delivery/11-disputes-developer.md",
		"docs/delivery/12-platform-expansion.md",
		"docs/delivery/README.md",
		"docs/delivery/decisions.md",
		"docs/delivery/recovery.md",
		"docs/delivery/validation.md",
		"docs/evidence/template.md",
	} {
		//nolint:gosec // G304: path comes only from the fixed local documentation allowlist above.
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
		//nolint:gosec // G703: the resolved path is checked against the physical repo root using filepath.Rel and IsLocal.
		if _, err := os.Stat(resolved); err != nil {
			return fmt.Errorf("%s: broken local link %s: %w", path, target, err)
		}
	}
	return nil
}
func fatal(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(1) }
