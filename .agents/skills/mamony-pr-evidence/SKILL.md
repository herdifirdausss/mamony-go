---
name: mamony-pr-evidence
description: Use when reviewing or preparing a Mamony Go PR, before claiming a feature or fix is complete. Not for speculative design with no code.
---
# Mamony PR evidence

1. Identify the effect on money, access, or recovery. Load issue ACs, changed files, relevant contracts, and currently configured make/CI commands.
2. Produce an AC-to-file-to-test map. Check boundary imports, finance/security invariants and API/schema compatibility if touched.
3. Run smallest useful targeted checks to diagnose failures, then all applicable repo gates. Do not substitute a local targeted check for PR CI.
4. Summarize execution as `PASS`, `FAIL`, or `NOT RUN` for each check, including command and key error; retain full logs as artifacts when available.
5. Report scope changes, unresolved decisions, migration/recovery/rollout plan when risk demands it, and a short recommendation to merge or block.
6. Never claim a check ran, a release is safe, or a production deployment occurred without primary evidence.
