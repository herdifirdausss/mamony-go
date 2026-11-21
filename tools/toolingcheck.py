#!/usr/bin/env python3
"""Structural adoption guard for the eleven explicitly evaluated tools.

This check does NOT install external tooling or prove any third-party skill is safe.
"""
from pathlib import Path
import re
import sys

NAMES = (
    "Ponytail", "Superpowers", "gopls MCP", "Trail of Bits Skills",
    "ASD-STE100", "ast-grep agent skill", "go-arch-lint",
    "oasdiff", "gotestsum", "RTK", "NVIDIA SkillSpector",
)
FILES = (
    ".go-arch-lint.yml", ".github/workflows/engineering-tools.yml",
    "scripts/check-openapi-compat.sh", "docs/tooling.md",
)


def check(root: Path) -> list[str]:
    problems = []
    for name in FILES:
        if not (root / name).is_file():
            problems.append(f"missing adoption artifact: {name}")
    if problems:
        return problems

    doc = (root / "docs/tooling.md").read_text(encoding="utf-8")
    for name in NAMES:
        if not re.search(r"^\| \[{}\]\(".format(re.escape(name)), doc, re.MULTILINE):
            problems.append(f"missing tool entry: {name}")

    ci = (root / ".github/workflows/engineering-tools.yml").read_text(encoding="utf-8")
    for marker in (
        "go-arch-lint@v1.19.2", "gotestsum@v1.13.0",
        "go-arch-lint check", "--junitfile", "--jsonfile",
    ):
        if marker not in ci:
            problems.append(f"CI tools missing pin or evidence: {marker}")

    config = (root / ".go-arch-lint.yml").read_text(encoding="utf-8")
    for marker in ("version: 3", "ledger_domain:", "platform:", "app:"):
        if marker not in config:
            problems.append(f"architecture config missing: {marker}")

    api = (root / "scripts/check-openapi-compat.sh").read_text(encoding="utf-8")
    for marker in ("BASE_SPEC", "CURRENT_SPEC", "oasdiff breaking", "--fail-on ERR"):
        if marker not in api:
            problems.append(f"OpenAPI script missing: {marker}")

    if "NOT RUN" not in doc or "--no-llm" not in doc:
        problems.append("skill preflight documentation lacks exact scan mode/status")
    return problems


def main() -> int:
    errors = check(Path("."))
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print("11-tool adoption registry: PASS (tool execution, external safety, token savings NOT PROVEN)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
