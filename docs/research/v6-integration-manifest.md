# v6 research pack integration manifest

Source: user-attached `Mamony_Engineering_Research_Pack_v6(2).zip`, SHA-256 `ba6026634ba8746c188f5cbc96a5bbca875e086bf28e975f0fd14baf4b8ae202`.

| ZIP member | Repository destination | Treatment |
|---|---|---|
| `Mamony_Go_Production_Workflow_v6_Literature_Research.md` | [Integrated v6 workflow](engineering-workflow-v6.md) | English translation of the exact original import (source Git blob `3bb2d3576551a1329318ef6c517e864e2ad94217`) |
| `repo-overlay/docs/standards/ai-agent-production-engineering.md` | [Production engineering addendum](../standards/ai-agent-production-engineering.md) | English translation of the exact original import (source Git blob `a76179e53021fa9724b437e8add628dd1875644c`) |
| `repo-overlay/docs/evals/mamony-agent-benchmark.md` | [Agent benchmark E01–E12](../evals/mamony-agent-benchmark.md) | Exact file bytes; Git blob SHA `710a68fbb9bcad2955fcbd280caafb1a17b03f27` |
| `repo-overlay/docs/research/source-register.md` | [Research source register](source-register.md) | All 31 references and status classifications retained, table adapted to repo |
| `repo-overlay/AGENTS.proposed.md` | [AGENTS.md](../../AGENTS.md) | Selective, non-duplicating merge with existing v5 instructions |
| `repo-overlay/docs/standards/engineering-principles.md` | [Existing v5 handbook](../standards/engineering-principles.md) | Shared v5 handbook retained and translated |
| `repo-overlay/.agents/skills/*/SKILL.md` | [Existing five agent skills](../../.agents/skills/) | Already present in PR; retained their validated metadata and domain focus |
| ZIP `README.md` | This manifest and [research README](README.md) | Adoption steps integrated; original ZIP README not added as a redundant repo README |

## Runtime status, not documentation aspirations

The full v6 materials are **reference guidance, not accepted Go ADRs**. Adding or linking them is not equivalent to having production hardening, security scans, database integration, agent evaluation or measured token savings. `make research` checks headings, source and eval registries, and routing only; the actual E01–E12 benchmark is **NOT RUN**.

The source Mamony Rust repository and Seev remain read-only references. Review the [adoption register](../standards/adoption-status.md) before treating a recommendation as a binding policy.
