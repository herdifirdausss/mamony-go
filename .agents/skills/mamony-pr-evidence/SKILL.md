---
name: mamony-pr-evidence
description: Use when reviewing or preparing a Mamony Go PR, before claiming a feature or fix is complete. Not for speculative design with no code.
---
# Mamony PR evidence

1. Identify the [change path](../../../docs/development-workflow.md#choose-a-change-path) and effect on money, access, or recovery. Load the intake issue, accepted PRD/technical design when required, owning contract ACs, changed files, and configured make/CI commands. Flag an unapproved Critical design as BLOCKED; do not infer approval from a document existing.
2. Produce an AC-to-file-to-test map and identify undocumented scope changes, unverified assumptions, and tests not executed. Check boundary imports, finance/security invariants and API/schema compatibility if touched.
3. Run smallest useful targeted checks to diagnose failures, then all applicable repo gates. Do not substitute a local targeted check for PR CI.
4. Summarize execution as `PASS`, `FAIL`, or `NOT RUN` for each check, including command and key error; retain full logs as artifacts when available.
5. Report scope changes, unresolved decisions, migration/recovery/rollout plan when risk demands it, and a short recommendation to merge or block.
6. Never claim a check ran, a release is safe, or a production deployment occurred without primary evidence.
