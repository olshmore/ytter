#!/usr/bin/env bash
# Create or update the Kubernetes secret

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SECRETS_FILE="${APP_SECRETS_FILE:-$ROOT/deployments/eks/app-secrets.env}"
SECRET_NAME="${APP_SECRET_NAME:-ytter-app-secrets}"

if [[ ! -f "$SECRETS_FILE" ]]; then
  echo "secrets file not found: $SECRETS_FILE" >&2
  exit 1
fi

kubectl create secret generic "$SECRET_NAME" \
  --from-env-file="$SECRETS_FILE" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "applied secret $SECRET_NAME from $SECRETS_FILE"
