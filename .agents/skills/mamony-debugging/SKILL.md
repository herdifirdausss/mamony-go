---
name: mamony-debugging
description: Use for Mamony Go bugs, failing tests, flaky CI, crash/restart anomalies, or unexplained behavior. Activate before proposing a fix; skip routine feature requests.
---
# Evidence-first debugging

Inspired by [Superpowers: systematic-debugging](https://github.com/obra/superpowers/blob/main/skills/systematic-debugging/SKILL.md), authored independently for Mamony.

1. Record the observed failure, environment, exact command, expected result and error. Do not silently treat a flaky test as passed.
2. Reproduce the behavior. Read errors and identify the failing layer; inspect recent changes and a comparable working path.
3. State one falsifiable root-cause hypothesis. Add the smallest possible reproducer or failing regression test (RED). A DB lock/order issue requires PostgreSQL, not a mock.
4. Apply one minimal fix. Observe GREEN on the regression, then run targeted checks, affected contracts and full applicable CI. Run race detector for goroutine changes and real concurrency tests for database races.
5. Report **root cause, evidence, change, commands with PASS/FAIL/NOT RUN, residual risk**. If the root cause is unconfirmed, call it a hypothesis.

For high-risk ledger incidents, also use `mamony-ledger-review`, focusing on idempotency, atomicity and unknown outcomes.
