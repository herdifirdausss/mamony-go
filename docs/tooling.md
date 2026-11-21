# Development tools

Use the [engineering checks](engineering.md#verification) for day-to-day work. The repository owns its own agent skills. Review and install upstream tools separately.

| Tool or reference | Use in Mamony Go |
|---|---|
| [Ponytail](https://github.com/DietrichGebert/ponytail) | The repository engineering review asks for the smallest valid design. |
| [Superpowers](https://github.com/obra/superpowers) | The repository debugging skill asks for observed failure, root cause, and verification. |
| [gopls MCP](https://go.dev/gopls/features/mcp) | Optional local tool for Go symbols and references. Verify it in the agent client before use. |
| [Trail of Bits Skills](https://github.com/trailofbits/skills) | The repository security review applies a focused diff workflow. |
| [ASD-STE100](https://github.com/danyuchn/asd-ste100-skill) | The repository writing skill uses short instructions and consistent terms. |
| [ast-grep agent skill](https://github.com/ast-grep/agent-skill) | Optional syntax-aware search. Use `rg` for text and `gopls` for Go symbols first. |
| [go-arch-lint](https://github.com/fe3dback/go-arch-lint) | CI runs `make archlint` with [the module rules](../.go-arch-lint.yml). |
| [oasdiff](https://github.com/oasdiff/oasdiff) | Candidate for comparing released and candidate specs after the first public API release. |
| [gotestsum](https://github.com/gotestyourself/gotestsum) | CI runs `make test-report` and stores JSON and JUnit reports. |
| [RTK](https://github.com/byx-darwin/rtk) | Evaluate locally with matched tasks before enabling output compression. |
| [NVIDIA SkillSpector](https://github.com/NVIDIA/skillspector) | Optional static scan in a third-party tool review. Scanner execution is `NOT RUN` here. |

## OpenAPI compatibility

Add compatibility checks when there is a released OpenAPI baseline and a candidate spec. Use the [HTTP decision](adr/0003-public-http-contract.md) for the public behavior.

## Third-party tool review

1. Record the upstream source, pinned version or commit, license, and required permissions.
2. Review scripts, hooks, installer behavior, dependencies, and network access.
3. If an audited SkillSpector installation is available, run `skillspector scan /path/to/reviewed-skill --no-llm`. Record which scan ran. Use `NOT RUN` for omitted scans.
4. Review findings and test in a sandbox with synthetic data and the minimum privileges.
5. Record the approved scope and the condition for another review. Do not send private prompts, source, or credentials to a remote scanner without approval.

Keep full failure logs when testing output compression.
