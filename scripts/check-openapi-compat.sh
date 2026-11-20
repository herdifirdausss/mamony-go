#!/usr/bin/env bash
# Usage: BASE_SPEC=path/to/baseline.yaml CURRENT_SPEC=path/to/current.yaml make api-compat
# No spec exists in the Go bootstrap: intentionally fails closed on missing contract.
set -euo pipefail
: "${BASE_SPEC:?set BASE_SPEC to the existing released OpenAPI contract}"
: "${CURRENT_SPEC:?set CURRENT_SPEC to the candidate contract}"
command -v oasdiff >/dev/null || { echo "oasdiff not installed: NOT RUN" >&2; exit 2; }
test -f "$BASE_SPEC" || { echo "missing baseline spec: $BASE_SPEC" >&2; exit 2; }
test -f "$CURRENT_SPEC" || { echo "missing candidate spec: $CURRENT_SPEC" >&2; exit 2; }
echo "Comparing OpenAPI contract (blocking breaking changes)"
exec oasdiff breaking --fail-on ERR "$BASE_SPEC" "$CURRENT_SPEC"
