# Identity real-browser and PostgreSQL E2E — Draft PR #38

## Scope, not a deployed application

The test-only `TestIdentityZBrowserVerificationE2E` is **not an Identity public route**. It exercises a manually constructed HTTPS test mux using existing unmounted registration, first-party verification page and email-confirmation handlers, plus the already-established Identity restricted database roles. The actual `httpedge.PublicHandler()` and `httpedge.PrivateHandler()` still provide health-only responses: both must return **404** for `/verify-email`, `/v1/identity/register` and `/v1/identity/verify-email`.

The test uses a disposable PostgreSQL 18 fixture under `mamony_test_*` on loopback, not a shared service or production database. A synthetic registration password hasher is injected: **this is not a benchmark or end-to-end test of Argon2id**. The existing Go tests separately cover hashing/policy. The actual proof store, AES-GCM envelope, purpose-bound challenge, outbox, transactional activation, rate admission and separate PostgreSQL roles are exercised.

## Tested transaction chain

1. **Real HTTPS POST /v1/identity/register** (test-only mux) calls the existing enrollment HTTP handler, anonymous admission, registration workflow, restricted registration role and proof issuer. It commits a new `UNVERIFIED` subject, password-verifier-shaped synthetic fixture, challenge, encrypted delivery obligation and identifier-only events. The response remains a generic **202**.
2. The existing bounded delivery worker takes three eligible jobs from the actual delivery-role PostgreSQL queue. A Mailgun adapter uses its normal fixed-region API URL, but a custom test transport reroutes it to a local HTTP mock. The mock confirms Mailgun Basic Auth, plaintext email only, no tracking, TLS requested, and exactly one approved `https://app.example.test/verify-email#token=...` link per recipient. No external Mailgun API is called.
3. **Actual headless Chromium** opens each link with Playwright (pinned `1.65.1`). An in-process, test-only network bridge intercepts requests to the approved browser origin, forwards them over **loopback HTTPS** to the Go `httptest` TLS listener, and fulfills the browser request using the exact server HTML, status, CSP and privacy headers. Unlike the former Node VM test, Chromium actually evaluates CSP, history API, DOM, accessibility roles and browser `fetch` with its own Origin and Fetch Metadata headers. Requests to any other URL are rejected. No production reverse proxy/TLS offload configuration is added.
4. A mail-client-style **GET preview** must not consume a proof. Immediately after navigation, the browser URL must have lost its fragment and must not contain the secret. Only an explicit accessible **Verify my email** click triggers one same-origin credential-free, no-referrer JSON POST. The valid link produces HTTP **204** and exactly one `UNVERIFIED` → `ACTIVE` transition plus one `CONTACT_VERIFIED` security event.
5. The second link is consumed and commits **204 at the actual verifier**, but the test bridge deliberately **drops the response after commit**. Chromium must display **“may have succeeded”**, not retry and not leak the token. PostgreSQL must be **ACTIVE** with one event; a subsequent replay must return generic **400**.
6. The third link is expired using PostgreSQL's own clock. The browser receives generic **400** after a click; the account must stay **UNVERIFIED** with zero `CONTACT_VERIFIED` events. A visit without a token must not reveal an enabled action button.
7. The Go test verifies account states directly using the disposable admin fixture, asserts single-use audit evidence, and checks all running production router paths remain **404**. Synthetic proof links are **redacted from errors and test logs**.

## Required CI (separate gate)

The workflow `.github/workflows/identity-real-browser-e2e.yml` runs a fourth infrastructure-heavy suite *in addition to* the existing baseline, PostgreSQL, browser VM contract and Go architecture gates. It provisions PostgreSQL 18.7 as a local CI service, uses exactly the same synthetic role credentials as the established `postgres-integration.yml`, installs pinned Go and Node **22.16.0**, installs pinned Playwright **1.65.1**, downloads the corresponding **Chromium headless shell**, bootstraps/validates database roles and migrations with `make integration`, and then runs:

```sh
go test -tags=integration,browser_e2e \
  -run '^TestIdentityZBrowserVerificationE2E$' \
  -count=1 -timeout=3m ./internal/modules/identity
```

Reproduce locally only after creating the standard disposable PostgreSQL 18 loopback role fixture. Run `make integration-local` first only for the standard database suite, then keep a compatible disposable container available if running browser E2E. The browser job uses npm-installed packages **only for CI** and does not modify `go.mod`, ship npm assets or add a production JavaScript runtime. No Playwright trace, video, screenshot containing a verification fragment, or secret-bearing network log is uploaded automatically.

## Known limits — no production-readiness claim

- The request bridge preserves the browser's `app.example.test` origin, but it is **not** a proof of production DNS routing, certificate issuance, external TLS ingress, reverse-proxy header trust or edge WAF behavior. The Go handler's direct TLS and Host validation still executes, and standalone negative tests cover malicious Host/Origin.
- Chromium is exercised; Firefox and WebKit/Safari are **not**. A browser runner cannot establish Mailgun inbox deliverability, provider retry exactly-once semantics or behavior of all mail-scanner vendors, including scanners that actively click buttons.
- The test uses **real database authority but a synthetic password hasher**, and all remote provider calls are mocks. Real secrets/key rotation, actual sign-in, account status query after ambiguous verification, accessibility on actual assistive technology, production observability and global rate limits remain open.
- A missing or ambiguous HTTP response **never proves the commit did not occur**. The client may need a verified-account status check once a safe login flow exists; it must not silently retry or reissue a single-use bearer.
- These tests are opt-in by build tag and candidate-only. Passing CI is **not** approval to mount any Identity endpoints or merge stacked Draft PRs into `main`.
