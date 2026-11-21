# Test plan checklist

Use this section inside an issue, critical proposal or PR. A separate test-plan file is **not** mandatory. Copy the cases relevant to the affected capability; omit irrelevant sections explicitly rather than inventing tests.

**Owner / change:** TBD  
**Risk path:** Routine | Feature | Critical  
**Accepted AC IDs / contracts:** TBD  
**Baseline commit / migration version:** TBD

| AC ID | Kind | Given / When / Then or invariant | Test name and fixture | Evidence | Status |
|---|---|---|---|---|---|
| TBD | Unit positive | Expected state change | TBD | TBD | NOT RUN |
| TBD | Unit negative | Incorrect input denied without mutation | TBD | TBD | NOT RUN |
| TBD | Integration (if DB) | Runtime role/tenant constraints enforced by real DB | TBD | TBD | NOT RUN |
| TBD | Concurrency (if money) | Two simultaneous operations produce one permitted effect | TBD | TBD | NOT RUN |
| TBD | Failure/recovery | Crash/timeout/duplicate resumes safely | TBD | TBD | NOT RUN |

## Required review questions

- Are requirements and expected outputs drawn from accepted contracts rather than invented by the AI?
- Can a failing test pass after removing an assertion or weakening a fixture? Review all test changes and the observed failure before the fix when practical.
- Does the test check **durable state and side effects** as well as the HTTP response?
- Is the database test actually using runtime/migration roles, real PostgreSQL and separate connections?
- Are authentication, authorization, tenant, financial and provider boundaries tested negatively?
- Were tests skipped, marked `N/A-disabled`, or disabled through build tags, CI configuration, fake return values or `continue-on-error`?
- Which cases are still `NOT RUN` and what blocks them?

Record reproducible commands, test names, runner/OS where relevant, fixture versions, and failures. Do not accept generated screenshots or model summaries as a substitute for the underlying executed evidence.
