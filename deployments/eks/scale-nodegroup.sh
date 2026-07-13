#!/usr/bin/env bash
# Resize the EKS managed node group (cost control).
#
# Usage:
#   EKS_NODEGROUP_DESIRED_SIZE=1 ./deployments/eks/scale-nodegroup.sh
#
# Defaults: min=1, max=1, desired=1 (single node; see EKS_NODEGROUP_NAME in infra.env).

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
INFRA_ENV="$ROOT/deployments/eks/infra.env"

if [[ ! -f "$INFRA_ENV" ]]; then
  echo "missing $INFRA_ENV" >&2
  exit 1
fi
set -a
# shellcheck disable=SC1091
source "$INFRA_ENV"
set +a

NODEGROUP_NAME="${EKS_NODEGROUP_NAME:-ytter-nodes}"
MIN="${EKS_NODEGROUP_MIN_SIZE:-1}"
MAX="${EKS_NODEGROUP_MAX_SIZE:-1}"
DESIRED="${EKS_NODEGROUP_DESIRED_SIZE:-1}"

if [[ -z "${EKS_CLUSTER_NAME:-}" || -z "${AWS_REGION:-}" ]]; then
  echo "EKS_CLUSTER_NAME and AWS_REGION must be set in infra.env" >&2
  exit 1
fi

echo "Scaling ${NODEGROUP_NAME} on ${EKS_CLUSTER_NAME} to min=${MIN} max=${MAX} desired=${DESIRED}"
aws eks update-nodegroup-config \
  --cluster-name "$EKS_CLUSTER_NAME" \
  --nodegroup-name "$NODEGROUP_NAME" \
  --region "$AWS_REGION" \
  --scaling-config "minSize=${MIN},maxSize=${MAX},desiredSize=${DESIRED}"
