# AI skills and engineering tool adoption — Mamony Go

**Status:** concrete integration plan and first implementation. This list records **what is actually in the repo**, rather than claiming that an upstream plugin/skill is installed in every agent. Source review 2026-11-21; recheck releases and URLs when updating.

| Candidate | Actual Mamony integration | Activation and evidence | Current adoption |
|---|---|---|---|
| [Ponytail](https://github.com/DietrichGebert/ponytail) | Minimal-diff/YAGNI criteria incorporated into `mamony-engineering-review` | Trigger when proposing abstraction/extra layers; demand real alternative and acceptance test | **Native adaptation**, upstream NOT installed |
| [Superpowers](https://github.com/obra/superpowers) | `mamony-debugging` (reproduce → hypothesis → RED/GREEN → verify); `mamony-pr-evidence` closes the loop | Bugs and failed tests; evidence of root cause and reproducible checks | **Native adaptation**, upstream NOT installed |
| [gopls MCP](https://go.dev/gopls/features/mcp) | Optional local guide: [gopls MCP](gopls-mcp.md) | Only for installed local assistant with gopls MCP; experimental; not CI | **OPT-IN, NOT CONNECTED here** |
| [Trail of Bits Skills](https://github.com/trailofbits/skills) | `mamony-security-diff` scoped to auth, money, RLS, CI, and external calls | Security-sensitive PR; file/line evidence + attack path | **Native adaptation**, upstream NOT installed |
| [ASD-STE100](https://github.com/danyuchn/asd-ste100-skill) | `mamony-technical-writing` for requirements/error semantics | Agent instructions and technical error text; human review for meaning | **Native adaptation**, upstream NOT installed |
| [ast-grep agent skill](https://github.com/ast-grep/agent-skill) | [Structural search usage](structural-search.md), no automatic rewrite | On-demand structural Go searches; compare against `rg` and `gopls` | **OPT-IN local, NOT INSTALLED** |
| [go-arch-lint](https://github.com/fe3dback/go-arch-lint) | [`.go-arch-lint.yml`](../../.go-arch-lint.yml), pinned `v1.19.2` in [engineering-tools CI](../../.github/workflows/engineering-tools.yml) | `make archlint` and PR CI; complements dynamic `tools/archcheck` | **CI configured; check GitHub run for execution** |
| [oasdiff](https://github.com/oasdiff/oasdiff) | [OpenAPI compatibility script](../../scripts/check-openapi-compat.sh) with explicit baseline/current files | `BASE_SPEC=... CURRENT_SPEC=... make api-compat` | **Prepared; no Go OpenAPI contract yet — NOT RUN** |
| [gotestsum](https://github.com/gotestyourself/gotestsum) | Pinned `v1.13.0` in [engineering-tools CI](../../.github/workflows/engineering-tools.yml); JUnit + raw JSON evidence | `make test-report` or PR tooling workflow, artifacts retained 7 days | **CI configured; check GitHub run for execution** |
| [RTK](https://github.com/byx-darwin/rtk) | [Evaluation protocol](token-economics.md); **no** global command rewriting | Test only with matched tasks and complete token/cost records | **Experiment only — NOT INSTALLED or BENCHMARKED** |
| [NVIDIA SkillSpector](https://github.com/NVIDIA/skillspector) | [Third-party skill preflight](third-party-security.md) and manual `skillspector scan --no-llm` procedure | Scan source+commit before any install; review findings and scripts | **Pre-install protocol; scanner execution NOT RUN** |

## Design choices

- **Upstream skill ≠ in-repo adaptation.** A Mamony `SKILL.md` with attribution is an independently written, narrower workflow. Do not call it the original upstream skill or claim it was audited by the vendor.
- **No implicit agent plugin installation.** User/agent host installation is a separate explicit step, after provenance and security review. Never run `curl | sh`, auto-clone and execute, or a global RTK hook by default.
- **Prefer deterministic evidence first.** The import checker plus `go-arch-lint` test dependency boundaries. `gotestsum` changes test presentation, not test semantics. A successful scanner is not permission to waive review.
- **Avoid speculative tooling.** `oasdiff` must compare actual released OpenAPI to a candidate; no placeholder contract can count as a passing API compatibility check. `gopls MCP` and `ast-grep` should only be loaded for navigation problems that justify them.
- **Measure full session cost.** Compressing tool output may reduce visible tokens but increase additional prompts or retries. See [evaluation](token-economics.md).

## Quick paths

```sh
make check            # base deterministic checks, plus skill/tooling metadata
make archlint         # go-arch-lint binary required; no silent skip
make test-report      # gotestsum binary required, creates test-reports tests
make api-compat       # oasdiff, BASE_SPEC and CURRENT_SPEC required
```

The specialized CI job installs pinned Go tools with a read-only GitHub token. Treat new package versions as a dependency review task. For supported tool semantics, consult the linked authoritative upstream documentation.
