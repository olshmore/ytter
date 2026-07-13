#!/usr/bin/env bash
# Resolve ECS deploy ARNs (prefer values already set in infra.env / ecs-state.env).
# Avoids IAM lookups that CI users often cannot perform.
set -euo pipefail

: "${AWS_REGION:?}"
: "${AWS_ACCOUNT_ID:=}"

LOG_GROUP="${LOG_GROUP:-/ecs/ytter-api}"
export LOG_GROUP

if [[ -z "${EXECUTION_ROLE_ARN:-}" ]]; then
  EXECUTION_ROLE_NAME="${EXECUTION_ROLE_NAME:-ytterEcsTaskExecutionRole}"
  if [[ -n "$AWS_ACCOUNT_ID" ]]; then
    EXECUTION_ROLE_ARN="arn:aws:iam::${AWS_ACCOUNT_ID}:role/${EXECUTION_ROLE_NAME}"
  else
    EXECUTION_ROLE_ARN="$(aws iam get-role --role-name "$EXECUTION_ROLE_NAME" --query Role.Arn --output text)"
  fi
fi
export EXECUTION_ROLE_ARN

if [[ -z "${TASK_ROLE_ARN:-}" ]]; then
  TASK_ROLE_NAME="${TASK_ROLE_NAME:-ytterEcsTaskRole}"
  if [[ -n "$AWS_ACCOUNT_ID" ]]; then
    TASK_ROLE_ARN="arn:aws:iam::${AWS_ACCOUNT_ID}:role/${TASK_ROLE_NAME}"
  else
    TASK_ROLE_ARN="$(aws iam get-role --role-name "$TASK_ROLE_NAME" --query Role.Arn --output text)"
  fi
fi
export TASK_ROLE_ARN

if [[ -z "$EXECUTION_ROLE_ARN" || -z "$TASK_ROLE_ARN" ]]; then
  echo "EXECUTION_ROLE_ARN and TASK_ROLE_ARN must be set (see deployments/ecs/infra.env)" >&2
  exit 1
fi
