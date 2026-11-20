# Mamony Engineering Harness v7 — provenance and integration decision

**User-attached ZIP:** `Mamony_Engineering_Harness_v7(1).zip` (39 files); SHA-256 `538990b9fe7c8321f6fa2d2e5c8890a0d54bcfe75b9a9df77f6d9090fe563c88`. ZIP integrity check: passed. The user-provided source is an **older standalone Go scaffold**, written before GitHub access was available, so it must not blindly overwrite the now-working `mamony-go` repository.

## Merge decision by source category

| Original v7 ZIP members | Disposition in this PR |
|---|---|
| 5 `.agents/skills/*/SKILL.md` | Already present and extended; existing eight repo-native skills retained |
| `.github/workflows/ci.yml` | Foundation job already passed; **lint and vulnerability jobs integrated** using v7 tool/version design |
| `.gitignore`, `.go-version`, `.golangci.yml`, `AGENTS.md`, `Makefile`, `README.md` | Current repository versions retained; added routing and CI updates without replacing v5/v6 configuration |
| `cmd/mamony/main.go`, `go.mod`, `internal/app/*`, `internal/platform/httpedge/*`, `internal/modules/ledger/internal/domain/*` | Existing source + Go tests retained; do not downgrade or claim ledger DB functionality |
| `tools/archcheck/*`, `tools/doccheck/*` | Existing checker and negative-test policy retained |
| `docs/architecture.md`, `docs/index.md`, `docs/evidence/template.md` | Existing more mature repo documentation retained and updated for new routers |
| `docs/standards/engineering-principles.md` | v5 complete handbook retained instead of overwriting with the short v7 summary |
| `docs/contracts/database-contract-gaps.md`, `docs/contracts/ledger-contract-gaps.md` | V7 implementation blockers integrated as separate gap docs |
| `docs/standards/agent-workflow.md`, `docs/standards/security.md` | V7 compact task/security policy integrated as separate entry points |
| `docs/evals/benchmark.md`, `docs/evals/results.csv` | V7 mechanical fixtures moved to `docs/evals/harness-v7-fixture-cases.md` (IDs explicitly namespaced), CSV template retained |
| `docs/evidence/local-run.md`, `VERIFICATION.txt` | Condensed historical record at `docs/evidence/archived-harness-v7-local-run.md`; **do not relabel as current PASS** |
| `docs/research/provenance.md`, `INTEGRATION_PLAN.md`, `SHA256SUMS.txt` | Superseded by this source archive digest/merge map; original stale 404/NOT-RUN GitHub access claims not copied as current state |

The source ZIP is available in this conversation; this GitHub repository does not silently accept old executable code, test results or assumptions. Existing v5/v6 specs and original Mamony Accepted ADRs keep their authority.

## Reality check

- Go repository foundation and the v5/v6 guidance **have been integrated** into draft PR #1.
- New `golangci-lint` and `govulncheck` CI jobs are **only verified if their latest actual GitHub Actions runs pass**.
- PostgreSQL roles/RLS, migrations, C01 Post/Reverse/idempotency/outbox/recovery and controlled token benchmarks remain **NOT IMPLEMENTED / NOT RUN**.
- A green docs/checker CI is not financial, security or deployment readiness.
