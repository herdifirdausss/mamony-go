# OpenAPI breaking-change gate

Recommended tool: [oasdiff](https://github.com/oasdiff/oasdiff), reviewed release `v1.39.0` (2026-11-17). In Go bootstrap **there is currently no implemented HTTP payment API or released OpenAPI specification**. Therefore an actual API compatibility check is **NOT RUN**.

The repository provides [`scripts/check-openapi-compat.sh`](../../scripts/check-openapi-compat.sh), which fails when the binary, baseline spec, or candidate spec is missing. No success-shaped skip.

After approved endpoints and canonical OpenAPI exist:

```sh
go install github.com/oasdiff/oasdiff@v1.39.0
BASE_SPEC=docs/api/released.yaml CURRENT_SPEC=docs/api/openapi.yaml make api-compat
```

**Important:** a committed baseline is needed; comparing a file to itself never proves backward compatibility. In PR CI, obtain the baseline from trusted `main` or an immutable last release, and do not use user-controlled untrusted remote specs.

Check request, response, and security changes against the [local HTTP contract](../adr/0003-public-http-contract.md). Review intended breaking changes with the owner.
