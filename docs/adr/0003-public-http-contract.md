# ADR-0003 — Public HTTP contract

**Status:** Contract baseline. The amount format is open.

## Request and response

Use versioned REST endpoints with JSON. Add each released route to the OpenAPI document in this repository. Test requests, responses, and permissions against that contract.

Terminate TLS at the public edge. Authorize the action and resource in the caller's tenant. Use typed request and response fields for each route. Reject unknown input fields. Expose only the response fields that the caller can read.

Use `application/problem+json` with a stable error code. Keep secrets, query text, and stack traces out of the response. Set body size, time, page size, and rate limits for each route before release. Do not expose the private listener to public traffic.

## Input admission and browser boundary

A public route must publish its authentication, authorization and input limits before use. The client cannot assert its tenant, roles, ownership, authentication level, or payment approval through editable headers or JSON. Resolve these from the trusted caller context and owning capability.

- Bound request bytes independently of `http.Server.MaxHeaderBytes` (for example, `http.MaxBytesReader` in a JSON handler). Bound nested input, item counts, pagination, processing time and request concurrency according to the route. For expensive password verification, cap concurrent work as well as request rate.
- Reject unexpected fields **and duplicate JSON object keys** in security- or money-relevant requests. `encoding/json.Decoder.DisallowUnknownFields` alone does not reject duplicate keys. Choose and test the decoder policy before exposing the first JSON write endpoint.
- Accept forwarded identity, original IP and scheme headers only from an explicitly configured trusted proxy boundary. CORS does not grant authorization. Do not treat a SameSite cookie as sufficient CSRF defense.
- If browser cookie sessions are introduced, use host-only `Secure`, `HttpOnly`, appropriate `SameSite`, origin/CSRF validation on state-changing requests, and server-side expiry and revocation. A session authenticates a subject; it never supplies object-level permission by itself.
- Return uniform non-sensitive error responses. Verify public/private route inventories after ingress or reverse-proxy changes. The current loopback listeners do not establish production TLS or network isolation.

The [identity contract](../contracts/identity.md) owns session and authorization rules. The [platform contract](../contracts/platform.md) owns deployment and resource budgets. These constraints remain **pending** for routes that do not yet exist.

## HTTP idempotency

Require an `Idempotency-Key` for requests that change money or state. Scope the key to the authorized caller. Store a fingerprint of the canonical request in the owning capability's database.

| Request | Result |
|---|---|
| Missing key on a mutating route | Return `400`. |
| Same key and same payload after completion | Return the stored outcome after authorization. |
| Same key and different payload | Return `422` without a write. |
| Same key while the first request is running | Return `409`. |

Set the key lifetime in the route contract. A request after expiry is new. The internal [ledger journal key](../contracts/ledger.md#posting-and-replay) has its own lifetime.

## Open decision on wire amounts

Send exact minor units with a currency code. Two wire formats are under consideration.

| Option | Range and effect |
|---|---|
| JSON integer | Use the interoperable safe integer range, up to `2^53 - 1` in magnitude. |
| JSON string of digits | Can carry the full positive signed 64-bit range when the route validates it. Define whether the route accepts a sign or zero. |

Choose the JSON type and range before publishing the first amount-bearing route. Record the choice here and in OpenAPI. Test the wire range and the [internal ledger range](../contracts/ledger.md#posting-and-replay) independently.

## Candidate Identity HTTP registration contract (Draft PR #31; not mounted)

Two proposed first-party browser operations exist as unmounted `http.Handler` logic only: `POST /v1/identity/register` with `email,password`, and `POST /v1/identity/resend-verification` with `email`. HTTPS must reach the Go handler directly (`req.TLS`), the request Host and exactly one Origin must match the reviewed `https://` origin, Fetch Metadata must indicate same-origin if provided, and cookies/Authorization/forwarded transport scheme must not be used as identity authority. Request JSON is capped at 2048 bytes and rejects unknown/duplicate keys, nonstrings, extra documents, invalid email and query parameters. Malformed input returns a generic stable `application/problem+json` error; syntactically valid known/unknown/duplicate/limited outcomes return an identical 202 receipt. Runtime work errors after admission also normalize to the receipt and thus require independent private alerting and reconciliation before go-live. Equal response bodies/statuses do not guarantee constant-time behavior. A reverse-proxy TLS-terminating deployment is not yet supported and must fail closed until reviewed. This candidate does not amend the default health-only router and has no OpenAPI release claim.

## Candidate Identity email-confirmation contract (stacked Draft PR #34; not mounted)

A single unmounted handler proposes `POST /v1/identity/verify-email` with `{"token":"<base64url-encoded 32-byte bearer>"}`, never a token in the request URL. As in draft registration, direct TLS and one exact HTTPS Host+Origin are mandatory; cookie/Authorization and forwarded scheme input are rejected. A 256-byte bounded strict JSON decoder rejects duplicate/unknown fields, wrong types, noncanonical tokens and extra documents. It requires an injected trusted peer resolver and explicit confirmation-admission interface; **there is no deployable distributed limiter in this PR**. A valid proof yields 204/no body. Invalid, expired, consumed and unknown proofs share a stable generic 400. A denied admission yields 429, and peer/admission unavailability yields 503. Only the existing verifier DB transaction changes account state, with one durable event; it does not authenticate or create a browser session. The email link's `#token` fragment must be handled by a separately reviewed first-party frontend. Current public/private routers stay health-only, and this candidate must not be included in public OpenAPI until all release controls are approved.

## Confirmation abuse gate candidate (stacked Draft PR #35; not mounted)

The `POST /v1/identity/verify-email` handler's required `ConfirmationAdmission` dependency now has a database-backed Identity implementation. It charges two HMAC-only client network counters under the separate `CONFIRM` action, capped at 8/minute and 80/hour in fixed UTC-aligned windows, reusing the existing admission table after fingerprinted Identity v7 migration. The HTTP 429 is for exhausted client budget; a storage or trusted-peer failure returns 503. Existing REGISTER/RESEND actions retain their prior budgets and roles. This candidate cannot yet be exposed: global bot quotas, shared-proxy fairness, secret rotation, direct TLS/Origin policy, first-party fragment-consuming UI, operational metrics and verifier fault classification remain pending. No public OpenAPI release claim or router changes.
