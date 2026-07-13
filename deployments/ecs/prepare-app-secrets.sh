#!/usr/bin/env bash
# Write app-secrets.env from CI env (same keys as EKS deploy).
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
exec bash "$ROOT/deployments/eks/prepare-app-secrets.sh"
