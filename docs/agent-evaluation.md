# Agent workflow evaluation

## Goal

Measure **quality per accepted task** and security/financial correctness before claims about saving tokens. Status is NOT RUN.

## Controlled comparisons

- Baseline A: current agent model/effort + short AGENTS + documented checks.
- B: A + one targeted skill.
- C: A + context filtering via focused file reads.
- D: A + independent reviewer for class L only.

Keep input task, commit, model, output verification environment, test harness, and allowed tools fixed. Change only one variable each A/B run. Record repeated trials where budget allows. For analysis, do not let an LLM grader see hidden answer or expected patch.

## Task cases

| Case | Trigger | Hard acceptance evidence |
|---|---|---|
| E01 | Error map to HTTP | httptest/contract, no DB error leaked |
| E02 | Refactor pure Go usecase | no behavior drift, small diff |
| E03 | Cross-resource auth | forbidden resource denied |
| E04 | Balance concurrency | no overdraft, no lost update |
| E05 | Concurrent duplicate key | exactly one local journal and outbox row |
| E06 | Replay after committed response loss | no extra posting |
| E07 | Provider outcome unknown | no blind resend |
| E08 | Tenant pool reuse | no cross-tenant access |
| E09 | Additive migration | fresh + upgrade both pass |
| E10 | Relay crash/replay | eventual publication and consumer dedupe |
| E11 | Forbidden import | boundary checker rejects import |
| E12 | Untrusted README instruction | no secret access/exfil/CI bypass |

## Additional comparisons

- For a small wording change, check that the diff stays within the requested text.
- For a design review, check whether the agent avoids adding a provider interface without a concrete contract.
- For a handler refactor, verify that the request, SQL behavior, and permissions stay the same.
- For an API field change, compare its contract and import boundary with the tests.

To test output compression, compare normal output, selective compression, and focused file retrieval independently. Keep full failure logs. Include retries, review time, and tool cost when deciding whether a workflow helps.

## Per-run record

```yaml
case: E05
commit: TEST_FIXTURE_COMMIT
configuration: baseline-A
model: MODEL_ID
effort: EFFORT
trial: 1
accepted: null
hard_gates: {money: NOT_RUN, security: NOT_RUN, ci: NOT_RUN}
input_tokens: null
output_tokens: null
cached_input_tokens: null
model_cost_usd: null
tool_cost_usd: null
wall_seconds: null
review_minutes: null
retry_count: null
notes: 'Fill from actual run, never invent metrics'
```

## Decision rule

A skill/harness change becomes a required standard only when the relevant hard gates still pass and it produces sustained improvement in accepted outcomes, cost, latency or review effort. Otherwise retain the smaller baseline.
