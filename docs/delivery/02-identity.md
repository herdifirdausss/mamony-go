# 02 — Identity and access

C08 owns tenants, users, sessions, API credentials, roles, and change approvals. Capabilities own their resources. They supply the resource and tenant context for authorization.

## Authentication and lifecycle

| ID | Requirement | Acceptance check |
|---|---|---|
| IAM-01 | Give each tenant an opaque ID and one optional parent. Enforce the configured depth. Apply tenant suspension to the users and credentials within its scope. | Reject a cycle or excessive depth. Suspend a tenant. Check that its sessions and credentials cannot start new work within the specified revocation window. |
| IAM-02 | Require a second factor for operator sign-in. Offer a phishing-resistant factor. Use generic sign-in errors and a bounded attempt policy. Notify the user through an existing contact after a sensitive credential change. | Test an invalid second factor, repeated sign-in failures, factor recovery, and revocation. Verify the notice contains no secret. |
| IAM-03 | Give sessions an idle limit and a total lifetime. End sessions when a user is suspended or removed. Protect stored password verifiers and factor secrets. | Advance a test clock beyond both limits. Exercise session replay after revocation. |
| IAM-04 | Show a new API secret once. Store only its hash. Give it a recognizable test prefix. A creator can grant only scopes the creator holds. | Retry credential creation with the same key. Check one credential and one secret disclosure. Reject an attempt to grant a broader scope. |
| IAM-05 | Rotate a credential with an explicit overlap. End the old secret when the overlap expires or an operator revokes it. | Exercise both secrets before and after expiry. Check audit and event records. |

Record password rules, factor storage, recovery, session lifetimes, credential overlap, and revocation latency before implementation. Review the threat model and current guidance first. Use synthetic credentials in tests.

## Authorization and approvals

1. Authenticate the caller. Resolve its roles or scopes.
2. Resolve the action, target, and tenant. Deny access when the decision is missing.
3. Allow access to another tenant only through an explicit delegated rule. Give unknown and unauthorized resources the same response where that response protects existence.
4. Start the tenant transaction only after authorization. Apply the [tenant database rules](../contracts/postgresql.md#tenant-isolation).
5. Write audit records for security changes. Write identifier-only events with the state change.

**IAM-06.** Protect every registered money and operator route with an authorization check. Test missing permissions, a different tenant, suspended credentials, and a route without required metadata.

**IAM-07.** A requester files a controlled change with its action, target, and exact payload fingerprint. A different authorized person approves within the allowed window. The owner applies the approved content once.

Test self-approval, an expired approval, payload changes after approval, concurrent apply, and an unauthorized read of the approval queue. Operator interfaces follow [step 08](08-operations-events.md).
