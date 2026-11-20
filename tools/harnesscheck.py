#!/usr/bin/env python3
"""Check v7 harness integration integrity, not the performance or safety of the ledger."""
from pathlib import Path
import csv
import re
import sys

REQUIRED_FILES = (
    "docs/contracts/postgresql.md",
    "docs/contracts/ledger-gate-a.md",
    "docs/standards/agent-workflow.md",
    "docs/standards/security.md",
    "docs/evals/harness-v7-fixture-cases.md",
    "docs/evals/results.csv",
    "docs/evidence/archived-harness-v7-local-run.md",
    "docs/research/harness-v7-provenance.md",
)
DIGEST = "538990b9fe7c8321f6fa2d2e5c8890a0d54bcfe75b9a9df77f6d9090fe563c88"


def check(root: Path) -> list[str]:
    errors: list[str] = []
    required = REQUIRED_FILES + ("docs/index.md", "AGENTS.md", "README.md", ".github/workflows/ci.yml")
    for rel in required:
        if not (root / rel).is_file():
            errors.append(f"missing v7 integration file: {rel}")
    if errors:
        return errors

    evidence = (root / "docs/research/harness-v7-provenance.md").read_text(encoding="utf-8")
    if DIGEST not in evidence or "39 files" not in evidence:
        errors.append("missing source ZIP checksum / 39-file scope in v7 manifest")
    historical = (root / "docs/evidence/archived-harness-v7-local-run.md").read_text(encoding="utf-8")
    if "historical" not in historical.lower() or "NOT RUN" not in historical:
        errors.append("historical evidence must not masquerade as current CI success")

    eval_text = (root / "docs/evals/harness-v7-fixture-cases.md").read_text(encoding="utf-8")
    cases = re.findall(r"^\| (H07-E\d{2}) \|", eval_text, re.MULTILINE)
    if cases != [f"H07-E{i:02d}" for i in range(1, 9)]:
        errors.append("v7 harness must list scoped H07-E01..H07-E08 once, in order")

    with (root / "docs/evals/results.csv").open(encoding="utf-8", newline="") as fp:
        rows = list(csv.reader(fp))
    expected_columns = (
        "case", "config", "task_success", "hard_gates_passed",
        "elapsed_seconds", "input_tokens", "output_tokens",
        "cached_input_tokens", "estimated_cost_usd", "review_notes",
    )
    if not rows or tuple(rows[0]) != expected_columns:
        errors.append("v7 evaluation CSV header does not match source schema")

    ci = (root / ".github/workflows/ci.yml").read_text(encoding="utf-8")
    for token in (
        "  foundation:", "  lint:", "  vulnerability:",
        "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0",
        "golang.org/x/vuln/cmd/govulncheck@v1.10.0",
        "make lint", "make security",
    ):
        if token not in ci:
            errors.append(f"v7 security/lint CI missing: {token}")

    index = (root / "docs/index.md").read_text(encoding="utf-8")
    for path in (
        "contracts/postgresql.md", "contracts/ledger-gate-a.md",
        "standards/agent-workflow.md", "standards/security.md",
        "research/harness-v7-provenance.md",
    ):
        if path not in index:
            errors.append(f"docs index missing v7 route: {path}")
    return errors


def main() -> int:
    errors = check(Path("."))
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print("v7 harness integration metadata: PASS (actual lint/vuln/DB/agent evaluation must be reported separately)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
