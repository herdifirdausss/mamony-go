# Token-efficiency experiments — no assumed savings

**RTK is not installed.** Source: [byx-darwin/rtk](https://github.com/byx-darwin/rtk). The independent benefit varies with model/effort/tool output distribution; terminal compression can hide decisive failure details.

Do **not** enable any global command-output rewriting or shell hook in a security- or finance-sensitive agent until controlled evaluations are completed.

Compare matched E01–E12 tasks from [Mamony benchmark](../evals/mamony-agent-benchmark.md) using:
- A: normal output + current short context.
- B: manually selected low-risk commands with compressed output, while preserving the full log as an artifact.
- C: selective file context without compression, to isolate the benefit of retrieval.

Record per trial: agent/model/effort, commit, task ID, input/output/cached tokens, total API/tool cost, retries, wall time, accepted AC, negative tests, human review and missing debug evidence. The output format may differ by provider. A lower token count is not meaningful if risk gates or acceptance decrease.

**Decision:** only adopt RTK on a specific command class if repeated trials preserve all hard gates and yield a net improvement in total cost or time. Log all raw failure output separately; never compress security scanners, money invariants or CI failures by default.

Other tools also have overhead: gopls MCP tool schemas, overly broad agent skills, parallel reviewers, and repeated code scans. Measure the whole task, not a single tool invocation.
