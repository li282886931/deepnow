#!/bin/sh
set -eu
spec="$(CDPATH= cd -- "$(dirname -- "$0")/../api" && pwd)/openapi.yaml"
test -s "$spec"
grep -q '^openapi: 3.0.3$' "$spec"
for path in '/api/v1/apps:' '/api/v1/tokens:' '/api/v1/tokens/validate:' '/api/v1/approvals:' '/api/v1/audit-logs:' '/api/v1/risk-events:' '/api/v1/policies:'; do
  grep -q "^  $path" "$spec"
done
grep -q 'Idempotency-Key' "$spec"
grep -q 'request_id' "$spec"
echo "openapi contract looks valid"
