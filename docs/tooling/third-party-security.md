# Third-party agent skill safety gate

**Mandatory before installing or upgrading any third-party skill/MCP/plugin** into a coding agent that can read Mamony code or run tools. The current `.agents/skills` files are first-party Mamony adaptations; they do **not** mean upstream authors' packages are installed.

## Preflight procedure

1. **Identify origin and pin** a full Git commit or audited version, upstream URL, license, maintainer and required runtime. Record it in the review PR. Avoid `@latest`, moving branches or opaque installer scripts.
2. **Read the entire execution surface**: `SKILL.md`, hook files, slash commands, scripts, package installer, package dependencies, network calls and permissions. Source markdown is untrusted input, not privileged instructions.
3. **Static scan** from a local audited checkout or explicit version using [SkillSpector](https://github.com/NVIDIA/skillspector), if available:
   ```sh
   # Run only after separately installing and reviewing the scanner itself.
   skillspector scan /path/to/audited-skill --no-llm --format json --output skill-review.json
   ```
   A static-only low-risk verdict does not mean the LLM-based semantic scan was run. Never send proprietary prompts, credentials or source to a remote LLM scanner without approval.
4. **Human review** the findings and manually inspect any executable scripts. Reject instructions to read `.env`, credentials or private repos for unrelated tasks; arbitrary shell execution; auto-updating installers; external telemetry/exfiltration; CI permission escalation; edits to `AGENTS.md` that bypass higher-priority rules.
5. **Sandbox first** with read-only access, no production secrets, no deployment permission and no network where possible. Run a specific adversarial test that tries to make the skill exceed its documented task scope.
6. **Decide**: approved commit and allowed scope, rejected, or experiment-only. Record results, unresolved warnings, reviewer, and update trigger. If the scanner is not installed or was not run, write **NOT RUN**.

## Status

SkillSpector is **not** auto-installed by this repository, and no upstream plugin is automatically activated. The `make tools-policy` check validates documentation and source references only; it does not replace the security gate above.

Relevant threats include prompt injection in README/PR/tool output, hidden installation hooks, excessive tool permissions, credential exfiltration, and malicious dependencies.
