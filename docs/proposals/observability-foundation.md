# Observability foundation proposal

- **Title / owning capability:** Cross-cutting observability foundation / Platform
- **Issue / decision owner / technical reviewer:** No issue opened for this proposal. Repository owner owns product decisions. Independent technical review is pending.
- **Status:** PROPOSED
- **Risk path and rationale:** Critical. Request and worker correlation crosses trust boundaries, and telemetry may disclose identity or financial information or leave the application through an exporter.
- **Affected contracts and requirement IDs:** Platform FND-04, FND-10, FND-11 and FND-12; HTTP contract in ADR-0003; package boundaries in architecture and conventions.
- **Last evidence review date:** 2026-11-27 (WIB)
- **Related ADR or open decision:** No ADR is proposed until backend, propagation, data-retention and deployment decisions are accepted. FND-04 remains the owning requirement.

## G1 — Research and problem validation

**Current behavior and problem:** The `main` snapshot at `f3896e8bcc0bfdea431ce0e986abd7231c24b3cc` configures JSON `log/slog` in `cmd/mamony` and several independent Identity commands. The application passes a logger to the HTTP composition root. The default process serves health routes only. No OpenTelemetry, Prometheus or GoMock dependencies were found in `go.mod`. Existing Identity use-case tests use explicit fakes. The repository's FND-04 contract requires structured logs and correlation across a request, job and consumer. The current logging setup alone does not provide trace context, a telemetry export path or proof across an asynchronous boundary.

The current repository has independent worker and outbox candidates, but this review did not demonstrate a specific end-to-end synthetic operation that starts at an HTTP request and reaches an existing worker without enabling live email or mounting Identity routes. Selection of that safe path is a design blocker. A stand-alone trace fixture can check SDK and exporter behavior, but it cannot satisfy FND-04 by itself.

**Evidence ledger:**

| Claim or option | Source / exact section | Version / checked date | Observation | Confidence and limitation |
|---|---|---|---|---|
| GoMock is maintained by Uber as the successor to the archived Google repository. | [Uber GoMock README](https://github.com/uber-go/mock); [Google repository notice](https://github.com/golang/mock) | Repository documentation; 2026-11-27 | Use `go.uber.org/mock` if a generated mock materially improves a test. | High for maintenance status. Does not show Mamony needs generated mocks. |
| Go 1.24+ can pin runnable tools in a module `tool` directive. | [Go module reference, tool directive](https://go.dev/doc/modules/gomod-ref#tool) | Current documentation; 2026-11-27 | Go 1.27.0 baseline can pin the chosen mock generator with the module. | High. CI and toolchain compatibility still require verification. |
| Go's standard `log/slog` supports structured key-value records and context-aware logging. | [slog API](https://pkg.go.dev/log/slog); [handler guide](https://pkg.go.dev/golang.org/x/example/slog-handler-guide) | Current Go documentation; 2026-11-27 | Existing JSON `slog` is a suitable baseline; add trace fields at the logging boundary if required. | High for API. A handler wrapper needs tests for `WithAttrs`, `WithGroup`, concurrency and redaction. |
| OpenTelemetry Go lists traces and metrics as stable and logs as release candidate. | [OpenTelemetry Go status](https://opentelemetry.io/docs/languages/go/) | Current documentation; 2026-11-27 | Start with OTel traces and metrics, and keep application logs on JSON stdout. | High for status. Stability does not decide backend, costs, security or deployment. |
| Go's HTTP OTel instrumentation can create server spans. | [otelhttp package](https://pkg.go.dev/go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp) | Current package docs; 2026-11-27 | It is a candidate for HTTP instrumentation after the incoming trace-context policy is decided. | High for available API. Default propagation behavior must be reviewed for public ingress. |
| OTel Go supports parent-based sampling with a trace-ID-ratio root sampler as a production consideration. | [OpenTelemetry Go sampling](https://opentelemetry.io/docs/languages/go/sampling/) | Current documentation; 2026-11-27 | A low-volume synthetic lab can sample every trace. Production sampling requires measured volume and a separately accepted rate. | High for documented pattern. No production rate is selected. |
| Prometheus accepts OTLP metrics over HTTP when its receiver is explicitly enabled. | [Prometheus OTLP guide](https://prometheus.io/docs/guides/opentelemetry/) | Current documentation; 2026-11-27 | One candidate path is OTel SDK → Collector → Prometheus OTLP receiver. | High. The feature must be enabled and restricted. No deployment is configured. |
| Prometheus recommends avoiding unbounded label dimensions such as user IDs and email addresses. | [Prometheus instrumentation practices](https://prometheus.io/docs/practices/instrumentation/) | Current documentation; 2026-11-27 | Metric labels must use bounded operation, route-template and outcome values. | High. Mamony's final metric set and cardinality budget remain open. |
| Jaeger v2 is based on the OTel Collector framework and can receive OTLP; the project recommends the standard Collector when a broader telemetry pipeline is needed. | [Jaeger deployment and architecture](https://www.jaegertracing.io/docs/2.23/deployment/); [Collector components](https://www.jaegertracing.io/docs/2.23/architecture/) | 2.23 documentation; 2026-11-27 | Jaeger is a suitable trace backend for a local lab, with a separate OTel Collector when processing/routing is required. | High for supported architecture. Persistence and production deployment are not established. |
| OTel trace context can disclose internal context. Baggage requires review before crossing trust boundaries. | [Context propagation](https://opentelemetry.io/docs/concepts/context-propagation/); [Baggage security](https://opentelemetry.io/docs/concepts/signals/baggage/); [sensitive-data guidance](https://opentelemetry.io/docs/security/handling-sensitive-data/) | Current documentation; 2026-11-27 | Treat incoming headers and exported spans as data crossing a trust boundary. Start with no baggage and no third-party propagation. | High. The actual threat model and deployment boundary require review. |
| OTel logs API status is release candidate, while JSON logs can still be bridged by instrumentation. | [OpenTelemetry Go instrumentation](https://opentelemetry.io/docs/languages/go/instrumentation/) | Current documentation; 2026-11-27 | Keep JSON stdout independent from direct OTLP log export until log API and deployment needs are accepted. | High for status. Backend selection remains open. |

Alternatives considered include Uber Zap for logging, `client_golang` with a scrape endpoint for metrics, Jaeger as an all-in-one local lab, and Grafana Tempo as a separate trace backend. The existing standard logger and OTLP path keep the smallest candidate implementation free of parallel logger and metric APIs. This is a proposal for the next design review, not an accepted dependency decision.

## G2 — PRD / accepted requirements

**Actors and intended outcome:** An operator investigates a synthetic request, sees its structured log record, and follows it to the corresponding synthetic background operation. Developers can observe bounded service health and latency measurements without inspecting customer or financial records.

**Scope and non-goals:** The proposed first instrumentation slice covers one already-supported synthetic HTTP-to-job/consumer path, if a safe path can be identified. It adds correlation and tracing at the composition/adapter boundaries. A separate later slice may add OTel metrics and alert rules. A mock-generation pilot may be considered independently for one test interface. Production deployment, real user data, real funds, mounted Identity routes, live provider calls, audit-system changes, and production readiness are out of scope.

**Functional and non-functional requirements:** Preserve the health-only route inventory and loopback-only private listener. Keep domain packages independent from concrete log and telemetry SDKs. Generate a local request identifier at trusted ingress. Treat public `traceparent` as untrusted unless a separately reviewed trusted-proxy policy is adopted. Do not accept or emit baggage in the initial slice. Do not inject trace context into Mailgun or another external provider. The telemetry path must fail closed with respect to data minimization and must not block or change business outcomes when the collector is unavailable. Define startup, flush and shutdown deadlines before implementation.

**Acceptance criteria and traceability:**

| AC ID in owning contract | Given / When / Then or precise invariant | Test or measurable evidence | Decision status |
|---|---|---|---|
| FND-04-A | Given a selected synthetic request-to-worker flow, when it is executed, each application-owned operation can be located through a stable request correlation value and trace context. | Execute the actual synthetic path. Check structured logs and exported spans across producer, job and consumer, including a retry. Identify the exact path before coding. | PROPOSED; path selection BLOCKED |
| FND-04-B | Given a public request with missing, malformed, sampled-out or attacker-controlled trace headers, when the handler runs, it creates a valid local request context and does not trust an attacker-selected trace parent. | Positive and negative header tests, including stable request correlation when no span is exported. | PROPOSED |
| FND-04-C | Given a worker that processes one or more jobs, when a job is retried or handled as a batch, then the trace represents the actual parent/child or link relationship and preserves a separate searchable job correlation value where needed. | Exercise retry, batch and delayed-consumer cases. Do not infer trace continuity from mocked calls. | PROPOSED |
| FND-04-D | Given a controlled operation that requires actor/action/target/outcome/time/capability evidence, when telemetry is sampled or unavailable, then the owning durable audit requirement remains satisfied independently. | Verify separation of restricted audit record from sampled operational logs and spans. | PROPOSED; audit-owner review required |
| FND-10-A | Given sentinel passwords, proofs, session tokens, cookies, email addresses, DSNs, SQL parameters and provider errors, when records are emitted and exported, none appear in log fields, span attributes, events, resources, baggage or metric labels. | Sentinel scan of captured stdout and OTLP test receiver output for every sentinel. | PROPOSED |
| FND-11-A | Given telemetry export failure, slow collector, shutdown or untrusted headers, when the service operates, telemetry has bounded resource use and no change to handler result or persistence behavior. | Inject exporter outage and timeout. Verify request deadline, bounded memory/queue, flush deadline and server shutdown. | PROPOSED |
| FND-11-B | Given a private metrics or OTLP receiver, when it is deployed, network reachability is restricted and does not expose new public routes. | Route inventory and network-policy test in the chosen deployment fixture. | PROPOSED; deployment choice OPEN |
| FND-04-E | Given approved metric definitions, when synthetic traffic is observed, latency histograms and counts use bounded labels and can be queried in the selected backend. | Validate cardinality, units, histogram buckets and backend query results against a fixed synthetic workload. | PROPOSED; metric schema and backend OPEN |
| DEV-TOOL-A | Given one selected consumer-owned interface that benefits from interaction assertions, when mock generation runs from a clean checkout, generated output is reproducible and stays within the owning test package. | Regenerate the exact mock and compare the committed file; unit tests verify relevant call counts and failure behavior. | PROPOSED; optional tooling pilot |

The table proposes test obligations. It does not amend FND-04 or create accepted platform requirements. If accepted, promote stable ACs into the owning contract through its review process.

## G3 — Technical design and security review

**Alternatives considered:**

1. Continue with `slog` only and add correlation IDs. This is the smallest path, but it cannot represent cross-process timing, parent/child spans or trace-to-span navigation.
2. Adopt OTel traces and metrics with JSON logs on stdout. This keeps signal APIs portable, supports Collector routing and avoids direct application coupling to a chosen vendor. It adds SDK, exporter, configuration, sampling, and collector operations.
3. Adopt separate vendor SDKs and clients, such as Zap, `client_golang`, and Jaeger-specific trace code. This gives direct backend integration, but duplicates instrumentation and makes later backend changes more costly.
4. Keep telemetry local and disabled outside tests. This avoids an exporter boundary, but does not meet operator trace lookup without a configured collection backend.

**Candidate direction:** Retain JSON `slog` and add OTel trace/metric SDKs at the application composition root. Use OTLP as the export protocol. Consider `otelhttp` for server spans after explicitly selecting an ingress-propagation policy. Keep domain and use-case packages unaware of exporters and vendors. The first lab can use an OTel Collector and Jaeger v2 for traces. Metric routing to Prometheus may be a separate slice, since the current contract does not define metric names, SLOs or alert thresholds. Emit JSON logs to stdout for the platform log agent. Configure each process once in its command/composition root. Do not add redundant HTTP client, pgx, SQL or provider auto-instrumentation until its attributes and overhead have been reviewed.

Use `request_id` for request lookup, `trace_id` and `span_id` from a valid span context, and an opaque `job_id` for a logical delayed job where needed. `slog` should receive trace fields through one handler boundary so caller-owned contextual attributes, `WithAttrs` and `WithGroup` remain correct. Metric labels should be low-cardinality values such as route template, method, bounded operation and result class. Never put request/trace/job IDs, identities, raw paths, SQL text or money amounts in metric labels.

**Ownership and trust boundaries:** Platform owns telemetry configuration and the common composition wiring. HTTP and worker adapters own span boundaries around external I/O and job lifecycle. Capability code may pass `context.Context` and approved low-cardinality events but must not import exporter or backend packages. The designated audit owner retains actor/action/target evidence in its access-controlled durable store. Telemetry is diagnostic and may be sampled or lost.

Public ingress accepts arbitrary client headers. The candidate generates its own trace root at the public HTTP boundary and does not adopt external `traceparent` fields by default. If a future deployment terminates TLS at a proxy, proxy trust and propagation need separate acceptance. Do not accept baggage in the first slice. Keep outbound propagation disabled to external providers. A future trusted internal service or queue may carry W3C trace context in a bounded, validated job envelope. That envelope design must account for retries, batch links, replay, payload size and untrusted queue data.

**Flow and contracts:**

```text
Public HTTP handler
  ├── generate request_id and local trace
  ├── emit structured log with trace_id/span_id when sampled
  └── selected synthetic job producer
        └── job context (explicit approved propagation)
              └── selected synthetic consumer and retry
                    ├── structured logs
                    └── OTLP trace → Collector → trace backend

Service metrics → OTLP → Collector → Prometheus (separate proposed slice)
JSON stdout → platform log agent → selected log backend
```

The exact request-to-job path is unresolved and must be identified before implementation. Do not create a new production event type, queue schema or Identity route solely to make the diagram real. If no existing safe synthetic path is executable without live sends or route activation, revise the slice boundary and record that FND-04 acceptance cannot yet be demonstrated.

**Consistency and failure model:** Export is best-effort diagnostic work, with bounded buffers and timeouts. Collector or backend outage must not roll back or delay a committed business operation past its request deadline. Shutdown uses a bounded provider flush and then stops. Sampling does not affect audit durability or application results. Tail sampling, persistent Collector queue, automatic database instrumentation and trace-based security decisions are excluded until explicitly reviewed. A retry is a new processing attempt with a link or parent relationship chosen from the actual job semantics. A trace ID alone does not prove one-time processing, commit, delivery or audit completeness.

**Security and privacy:** Source-side allowlisting is the primary control. Exclude passwords, proof tokens, session/access tokens, cookies, Authorization values, email/contact data, raw DSNs, SQL query text/parameters, full provider errors, account IDs, customer/tenant IDs and transaction amounts. Do not assume Collector redaction can correct data that is already exposed at the application boundary. Use synthetic sentinels and inspect both JSON and exported OTLP payloads. Do not use user or resource identifiers as metric labels. Restrict Collector and backend ingress to the private network, use authenticated TLS for remote export when enabled, and decide retention, access control and deletion policy before any real Identity or Ledger telemetry leaves the service.

**Compatibility, observability and rollout:** Keep telemetry export disabled unless endpoint and credentials are explicitly configured. Never configure a production endpoint by default. Do not add a public scrape route. Start with local disposable lab services. Check host, port and receiver reachability restrictions. No migration is proposed. No production rollout is approved. A later deployment design must set sampling, retention, redaction, budgets, access policy, alert ownership and rollback.

**Decision status:** Proposed and blocked on: (1) exact synthetic HTTP-to-job/consumer path; (2) approved public trace-parent handling; (3) telemetry endpoint credentials and network boundary; (4) selected log and trace retention/access policy; (5) exact AC and verification scope; (6) whether metrics and mock generation belong in independent slices. No code, dependency addition, runtime wiring, route activation, merge or production activation is authorized by this document.

## G4 — Implementation slices

| PR / slice | Owner module | AC IDs | Tests and negative cases | Dependencies | Status |
|---|---|---|---|---|---|
| S0 — identify safe synthetic path and decide ingress/egress policy | Platform / HTTP / affected capability | FND-04-A–D, FND-10-A, FND-11-A | Existing executable path review, trusted-header tests, telemetry data inventory, acceptance and threat review | Owner decisions and technical review | BLOCKED; documentation only |
| S1 — logging correlation and traces in a synthetic fixture | `internal/app` and selected adapters | Accepted subset of FND-04 and FND-10 | Trace/log correlation, hostile headers, retries, no outbound provider propagation, sentinel leak checks, exporter outage and overhead | S0 accepted; OTel SDK and Collector lab approved | NOT AUTHORIZED |
| S2 — bounded service metrics | `internal/app` and selected adapters | Accepted FND-04-E and FND-11 | Metric schema, route cardinality, histogram queries, backend outage | Separate metric schema/backend decision | NOT AUTHORIZED |
| T1 — mock generation pilot (independent change) | Owning package test directory | DEV-TOOL-A | Clean generation, drift check, call assertions on one interface | Select interface; pin tool version | NOT AUTHORIZED |

Do not open implementation slices until the required G3 decisions are accepted and their owning-contract changes are reviewed.

## G5–G7 — Verification and outcome

| Requirement / threat | Test / command | Commit and evidence | Result |
|---|---|---|---|
| Source facts and repository baseline | Review repository snapshot and official documentation listed above | Main `f3896e8bcc0bfdea431ce0e986abd7231c24b3cc`; sources checked 2026-11-27 WIB | PASS for documentation review |
| Exact HTTP-to-worker correlation | Not executed; no selected safe end-to-end path | No implementation commit | NOT RUN |
| Header trust, sensitive-data redaction, export outage, shutdown and overhead | Requires an accepted design and synthetic implementation | No implementation commit | NOT RUN |
| Production retention, access, egress, sampling, alert ownership and deployment controls | Requires platform decisions and deployed evidence | No production configuration | NOT RUN |

This proposal records a design candidate only. Independent technical acceptance is pending. It does not establish production readiness or authorize implementation or merge.