# Research source register — 2026-11-21

Adapted from the attached Mamony Engineering Research Pack v6. This is a literature registry, **not proof of implementation**. Follow original links to verify versions and claims before promoting guidance to repository policy. Company reports are experience, research benchmarks are context-dependent, and standards can be draft or final.

| Source | Evidence class | Version/date at review | Mamony use | Link |
|---|---|---|---|---|
| Go 1.27 release notes | Go team; official release | 2026-08 | Go v1.27 JSON v2, platform, API changes; §20 | [Original source](https://go.dev/doc/go1.27) |
| Go release history | Go team; official release | rolling | Patch/security updates and pinned toolchain; §20 | [Original source](https://go.dev/doc/devel/release) |
| Go code review comments | Go team; official guidance | rolling | Consumer-owned interfaces and errors; §20 | [Original source](https://go.dev/wiki/CodeReviewComments) |
| Google Go style best practices | Google; company engineering style | rolling | Interfaces, naming, error handling; §20 | [Original source](https://google.github.io/styleguide/go/best-practices) |
| Uber Go Style Guide | Uber; company engineering style | rolling | Goroutines and lifecycle review; §20 | [Original source](https://github.com/uber-go/guide) |
| Go security | Go team; official guidance | rolling | govulncheck, fuzzing and security releases; §20/22 | [Original source](https://go.dev/doc/security/) |
| Go diagnostics | Go team; official guidance | rolling | Profiling/tracing before optimization; §20 | [Original source](https://go.dev/doc/diagnostics) |
| Go PGO | Go team; official guidance | rolling | Optional profile-guided optimization; §20 | [Original source](https://go.dev/doc/pgo) |
| OpenAI Harness Engineering | OpenAI; company engineering experience | 2026-02 | Repository map, agent legibility, enforced invariants; §18/19 | [Original source](https://openai.com/index/harness-engineering/) |
| OpenAI Rethinking Skills and Prompts | OpenAI; company developer guidance | 2026-09 | Selective skill triggers and context size; §18/19 | [Original source](https://developers.openai.com/blog/rethinking-skills-and-prompts-for-gpt-6-astra) |
| OpenAI Agent Evals | OpenAI; official product guidance | rolling | Traces, graders and eval runs; §23 | [Original source](https://developers.openai.com/api/docs/guides/agent-evals) |
| OpenAI Prompt Caching | OpenAI; official API guidance | rolling | Model/API specific cache economics; §23 | [Original source](https://developers.openai.com/api/docs/guides/prompt-caching) |
| Anthropic Harness Design | Anthropic; company engineering experiment | 2026-03 | Planner/generator/evaluator and resets; §18/19/23 | [Original source](https://www.anthropic.com/research/harness-design-long-running-apps) |
| Anthropic MCP Code Execution | Anthropic; company engineering example | 2025-11 | Selective tool/schema loading and filtered output; §19/23 | [Original source](https://www.anthropic.com/research/code-execution-with-mcp) |
| VS Code Agent Best Practices | Microsoft; company developer guidance | rolling | Acceptance criteria and verification; §18/19 | [Original source](https://github.com/microsoft/vscode-docs/blob/main/docs/agents/best-practices.md) |
| GitHub Copilot custom instructions | GitHub; official product guidance | rolling | Path-specific instructions depend on client; §19 | [Original source](https://docs.github.com/en/copilot/concepts/prompting-and-context/custom-instructions) |
| SWE-Skills-Bench | Research preprint; empirical | 2026-03 | Skill overhead: 39/49 not improved in tested setting; §18/23 | [Original source](https://arxiv.org/abs/2603.15401) |
| SWE-Bench Pro Verified limitations | Research preprint; empirical | 2026-09 | Benchmark leakage/task quality; §23 | [Original source](https://arxiv.org/abs/2609.08149) |
| DORA AI 2025 | DORA; research report | 2025 | AI amplifies engineering strengths and weaknesses; §18/24 | [Original source](https://dora.dev/research/2025/dora-report/) |
| AWS Builders Library Idempotent APIs | AWS; company engineering experience | 2021 | Safe retries and client tokens; §21 | [Original source](https://aws.amazon.com/builders-library/making-retries-safe-with-idempotent-APIs/) |
| AWS Well Architected Failure Interactions | AWS; official guidance | rolling | Timeouts, capped retries, jitter, overload; §21 | [Original source](https://docs.aws.amazon.com/wellarchitected/latest/framework/rel05.html) |
| PostgreSQL 18 Transaction Isolation | PostgreSQL; versioned technical spec | v18 | Transaction isolation and retry semantics; §21 | [Original source](https://www.postgresql.org/docs/18/transaction-iso.html) |
| PostgreSQL 18 Row Security | PostgreSQL; versioned technical spec | v18 | RLS policies and role boundaries; §21/22 | [Original source](https://www.postgresql.org/docs/18/ddl-rowsecurity.html) |
| Google SRE SLO workbook | Google; company engineering guidance | rolling | Reliability objectives and budgets; §21 | [Original source](https://sre.google/workbook/implementing-slos/) |
| OWASP ASVS | OWASP; standard | 5.0.0 stable | Application security verification; §22 | [Original source](https://asvs.owasp.org/) |
| NIST SSDF 1.1 | NIST; framework | 2022 final | Secure development baseline; §22 | [Original source](https://csrc.nist.gov/pubs/sp/800/218/final) |
| NIST SSDF 1.2 | NIST; draft | 2025 initial public draft | Monitor; do not treat as final; §18/22 | [Original source](https://csrc.nist.gov/pubs/sp/800/218/r1/ipd) |
| OWASP LLM Top 10 2026 | OWASP; community guidance | 2026 | Prompt injection/agent risks; §22 | [Original source](https://genai.owasp.org/resource/owasp-genai-llm-top-10-2026/) |
| OWASP Agentic Top 10 | OWASP; community guidance | 2025-12 | Tool misuse, excess agency, context poisoning; §22 | [Original source](https://genai.owasp.org/2025/12/09/owasp-top-10-for-agentic-applications-the-benchmark-for-agentic-security-in-the-age-of-autonomous-ai/) |
| SLSA 1.2 spec | SLSA; industry specification | v1.2 approved | Build/source assurance and provenance; §22 | [Original source](https://slsa.dev/spec/v1.2/) |
| GitHub Code scanning | GitHub; official product guidance | rolling | Plan-dependent repository code scanning; §22 | [Original source](https://docs.github.com/en/code-security/how-tos/find-and-fix-code-vulnerabilities/configure-code-scanning/configure-code-scanning) |

## Assessment protocol

- A durable engineering policy needs the exact source claim, known version, owner decision and an executable acceptance test.
- Resolve implementation semantics using current official documentation, but do not silently override an Accepted Mamony product ADR/PRD.
- Run matched-task agent evaluations before claiming external study results apply to Mamony or that token costs decreased.
- **NIST SSDF 1.1 was final and 1.2 an Initial Public Draft at this pack's review date.** Recheck statuses before upgrades.
- **Go version, toolchain patch and analyzer compatibility must be tested.** Release notes do not prove lint/tool compatibility.
