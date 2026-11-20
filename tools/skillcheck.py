#!/usr/bin/env python3
"""Structural validation for repository-owned SKILL.md files.

This checks presence and metadata, not whether an agent discovered/obeyed a skill.
Uses only Python's standard library.
"""
from pathlib import Path
import sys

EXPECTED = {
    "mamony-context-pack",
    "mamony-boundary-review",
    "mamony-ledger-review",
    "mamony-pr-evidence",
    "mamony-engineering-review",
}


def validate_skill(path: Path) -> list[str]:
    problems = []
    text = path.read_text(encoding="utf-8")
    lines = text.splitlines()
    if not lines or lines[0] != "---":
        return [f"{path}: missing YAML frontmatter start"]
    try:
        end = lines.index("---", 1)
    except ValueError:
        return [f"{path}: missing YAML frontmatter end"]
    fields = {}
    for line in lines[1:end]:
        if not line.strip():
            continue
        if ":" not in line:
            problems.append(f"{path}: invalid frontmatter line: {line}")
            continue
        key, value = line.split(":", 1)
        fields[key.strip()] = value.strip().strip('"').strip("'")
    name = fields.get("name")
    if name != path.parent.name:
        problems.append(f"{path}: name mismatch: {name!r} != {path.parent.name!r}")
    if not fields.get("description"):
        problems.append(f"{path}: missing description")
    if not any(line.startswith("# ") for line in lines[end + 1:]):
        problems.append(f"{path}: missing skill heading")
    if len(text.strip()) < 150:
        problems.append(f"{path}: skill unexpectedly short")
    return problems


def validate_registry(root: Path) -> list[str]:
    errors = []
    actual = {p.name for p in root.iterdir() if p.is_dir()} if root.is_dir() else set()
    for missing in sorted(EXPECTED - actual):
        errors.append(f"missing skill: {missing}")
    for extra in sorted(actual - EXPECTED):
        errors.append(f"unexpected skill folder (review before adding): {extra}")
    for name in sorted(EXPECTED & actual):
        path = root / name / "SKILL.md"
        if not path.is_file():
            errors.append(f"missing {path}")
        else:
            errors.extend(validate_skill(path))
    return errors


def main() -> int:
    errors = validate_registry(Path(".agents/skills"))
    if errors:
        print("\n".join(errors), file=sys.stderr)
        return 1
    print("Mamony skills: metadata/presence PASS (agent discovery NOT TESTED)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
