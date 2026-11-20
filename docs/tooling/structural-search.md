# On-demand Go structural search with ast-grep

Inspired by [ast-grep's official agent skill](https://github.com/ast-grep/agent-skill); the CLI is **not installed** or run automatically by Mamony.

First prefer `rg` for exact literals and `gopls` for Go symbols/references. Use ast-grep when the query requires syntax-aware matching rather than line text.

```sh
# With a separately reviewed local installation of ast-grep:
sg --version
sg run --lang go --pattern 'go $CALL' internal/
sg run --lang go --pattern 'panic($X)' internal/
```

These are **search examples, not security verification gates**. Search results may be incomplete for patterns with differing syntax or generated code. Review every match and update a precise rule/fixture before using `sg scan` as an automated check. Do not auto-rewrite ledger, authorization or migration files from a broad AST pattern without contract tests and code review.

A future structural rule must document: rule ID, intended match, positive/negative fixtures, allowed packages, false-positive handling and why `go vet`/`staticcheck`/the package checker are insufficient.
