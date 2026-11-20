# Historical standalone v7 evidence — not the current GitHub CI

This is the **source ZIP's local execution record**, originally produced with Go 1.23.2 on Linux amd64 for a standalone scaffold, **before** the actual GitHub repositories became accessible. Preserve the source's distinctions rather than reusing it as today's CI evidence.

| Historical check | Source record |
|---|---|
| `make check` | PASS — gofmt, vet, unit, architecture, docs, build, module verification |
| `make race` | PASS with Go 1.23.2, only for exercised code paths |
| Deliberate `payment` → `ledger` direct-import injection | PASS — architecture checker rejected with nonzero and `ARCH VIOLATION`; source mutation removed |
| Runtime smoke on loopback | PASS — public `/livez` 200, private `/readyz` 200, public `/readyz` 404 |
| Source YAML parsing | PASS for initial job structure, **not equivalent to job execution** |
| GitHub CI, Go 1.27.4, golangci-lint, govulncheck | NOT RUN in the standalone source ZIP |
| PostgreSQL 18, RLS, migration, backup/restore, C01 Gate A | NOT RUN / NOT IMPLEMENTED |
| Agent/RTK token/cost A/B experiment | NOT RUN |

The original standalone test first failed because `doccheck` assumed an incorrect working directory; that fixture was fixed and checks rerun. **This record is not a fresh result for `mamony-go`**. Use the current linked [GitHub Actions runs](https://github.com/herdifirdausss/mamony-go/actions) for evidence on this repository.
