#!/usr/bin/env bash
# Sync app-secrets.env to AWS Secrets Manager (JSON secret for ECS task env).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ECS_DIR="$ROOT/deployments/ecs"
INFRA_ENV="$ECS_DIR/infra.env"
STATE_ENV="$ECS_DIR/ecs-state.env"
SECRETS_FILE="${APP_SECRETS_FILE:-$ECS_DIR/app-secrets.env}"

if [[ ! -f "$INFRA_ENV" ]]; then
  echo "missing $INFRA_ENV" >&2
  exit 1
fi
if [[ ! -f "$SECRETS_FILE" ]]; then
  echo "missing $SECRETS_FILE" >&2
  exit 1
fi

set -a
# shellcheck disable=SC1091
source "$INFRA_ENV"
if [[ -f "$STATE_ENV" ]]; then
  # shellcheck disable=SC1091
  source "$STATE_ENV"
fi
set +a

SECRET_NAME="${APP_SECRET_NAME:-ytter/app}"
REGION="${AWS_REGION:?}"

python3 - "$SECRETS_FILE" <<'PY' > /tmp/ytter-secret.json
import json, sys
data = {}
for line in open(sys.argv[1]):
    line = line.strip()
    if not line or line.startswith("#") or "=" not in line:
        continue
    k, _, v = line.partition("=")
    data[k] = v
print(json.dumps(data))
PY

if aws secretsmanager describe-secret --secret-id "$SECRET_NAME" --region "$REGION" >/dev/null 2>&1; then
  aws secretsmanager put-secret-value \
    --secret-id "$SECRET_NAME" \
    --region "$REGION" \
    --secret-string file:///tmp/ytter-secret.json
  echo "updated secret $SECRET_NAME"
else
  arn=$(aws secretsmanager create-secret \
    --name "$SECRET_NAME" \
    --region "$REGION" \
    --secret-string file:///tmp/ytter-secret.json \
    --query ARN --output text)
  echo "created secret $arn"
fi
rm -f /tmp/ytter-secret.json
