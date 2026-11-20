#!/usr/bin/env python3
"""Check that the v6 research overlay is discoverable and has its required cases.

This validates file structure, not literature accuracy, security or agent quality.
"""
from pathlib import Path
import re
import sys

REQUIRED_FILES = (
    "docs/research/engineering-workflow-v6.md",
    "docs/standards/ai-agent-production-engineering.md",
    "docs/research/source-register.md",
    "docs/evals/mamony-agent-benchmark.md",
    "docs/standards/adoption-status.md",
    "docs/index.md",
    "AGENTS.md",
)


def section_ids(text: str) -> list[int]:
    return [int(s) for s in re.findall(r"^## (1[89]|2[0-4])\. ", text, re.MULTILINE)]


def eval_case_ids(text: str) -> list[str]:
    return re.findall(r"^\| (E\d{2}) \|", text, re.MULTILINE)


def registry_urls(text: str) -> list[str]:
    return re.findall(r"^\|.+\| \[Original source\]\((https://[^)]+)\) \|$", text, re.MULTILINE)


def check(root: Path) -> list[str]:
    errors: list[str] = []
    for rel in REQUIRED_FILES:
        if not (root / rel).is_file():
            errors.append(f"missing v6 document: {rel}")
    if errors:
        return errors

    workflow = (root / REQUIRED_FILES[0]).read_text(encoding="utf-8")
    addendum = (root / REQUIRED_FILES[1]).read_text(encoding="utf-8")
    register = (root / REQUIRED_FILES[2]).read_text(encoding="utf-8")
    benchmark = (root / REQUIRED_FILES[3]).read_text(encoding="utf-8")
    adoption = (root / REQUIRED_FILES[4]).read_text(encoding="utf-8")
    index = (root / REQUIRED_FILES[5]).read_text(encoding="utf-8")
    agents = (root / REQUIRED_FILES[6]).read_text(encoding="utf-8")

    expected_sections = list(range(18, 25))
    if section_ids(workflow) != expected_sections:
        errors.append("integrated v6 workflow must include sections 18–24 exactly once")
    if section_ids(addendum) != expected_sections:
        errors.append("v6 addendum must include sections 18–24 exactly once")
    cases = eval_case_ids(benchmark)
    if cases != [f"E{i:02d}" for i in range(1, 13)]:
        errors.append("agent benchmark must include E01–E12 exactly once, in order")
    urls = registry_urls(register)
    if len(urls) != 31 or len(set(urls)) != len(urls):
        errors.append("source register must include 31 distinct references")
    for label, content in (("AGENTS.md", agents), ("docs/index.md", index)):
        if "ai-agent-production-engineering.md" not in content:
            errors.append(f"{label} must link on-demand v6 addendum")
    if "draft" not in adoption.lower() or "NOT RUN" not in adoption:
        errors.append("adoption status must distinguish draft guidance and NOT RUN evidence")
    return errors


def main() -> int:
    errors = check(Path("."))
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print("v6 research, routing, 31 sources and 12 eval cases: PASS (agent benchmark NOT RUN)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
