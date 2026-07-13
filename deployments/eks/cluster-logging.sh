#!/usr/bin/env bash
# Enable or disable EKS control plane logs in CloudWatch.
#
# Usage:
#   ./deployments/eks/cluster-logging.sh disable   # stop new logs (saves cost)
#   ./deployments/eks/cluster-logging.sh enable    # turn logging back on
#   ./deployments/eks/cluster-logging.sh delete-log-group   # remove stored logs (irreversible)
#
# Config: deployments/eks/infra.env (EKS_CLUSTER_NAME, AWS_REGION)

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

if [[ -z "${EKS_CLUSTER_NAME:-}" || -z "${AWS_REGION:-}" ]]; then
  echo "EKS_CLUSTER_NAME and AWS_REGION must be set in infra.env" >&2
  exit 1
fi

LOG_TYPES='["api","audit","authenticator","controllerManager","scheduler"]'
LOG_GROUP="/aws/eks/${EKS_CLUSTER_NAME}/cluster"

set_logging() {
  local enabled=$1
  echo "Setting EKS control plane logging enabled=${enabled} on ${EKS_CLUSTER_NAME}"
  aws eks update-cluster-config \
    --name "$EKS_CLUSTER_NAME" \
    --region "$AWS_REGION" \
    --logging "{\"clusterLogging\":[{\"types\":${LOG_TYPES},\"enabled\":${enabled}}]}"
}

delete_log_group() {
  if aws logs describe-log-groups --region "$AWS_REGION" \
    --log-group-name-prefix "$LOG_GROUP" \
    --query "logGroups[?logGroupName=='${LOG_GROUP}'].logGroupName" \
    --output text | grep -q .; then
    echo "Deleting log group ${LOG_GROUP} (stored data will be lost)"
    aws logs delete-log-group --region "$AWS_REGION" --log-group-name "$LOG_GROUP"
  else
    echo "Log group ${LOG_GROUP} not found, nothing to delete"
  fi
}

case "${1:-}" in
  disable)
    set_logging false
    echo "Done. New control plane logs will not be sent to CloudWatch."
    echo "To stop storage charges for existing data: $0 delete-log-group"
    ;;
  enable)
    set_logging true
    ;;
  delete-log-group)
    delete_log_group
    ;;
  *)
    echo "usage: $0 disable|enable|delete-log-group" >&2
    exit 1
    ;;
esac
