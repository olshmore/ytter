#!/usr/bin/env bash
# Deploy ytter to ECS Fargate (rolling update).
#
# Usage:
#   ./deployments/ecs/deploy.sh              # production (secrets + new task revision + rollout)
#   ./deployments/ecs/deploy.sh app          # rollout only (image tag from ECR_IMAGE env)
#   ./deployments/ecs/deploy.sh secrets    # sync secrets + force redeploy
#
# CI sets ECR_IMAGE or pass: ECR_IMAGE=.../ytter:sha ./deployments/ecs/deploy.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ECS_DIR="$ROOT/deployments/ecs"
INFRA_ENV="$ECS_DIR/infra.env"
STATE_ENV="$ECS_DIR/ecs-state.env"

load_env() {
  if [[ ! -f "$INFRA_ENV" ]]; then
    echo "missing ${INFRA_ENV}" >&2
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
  # shellcheck disable=SC1091
  source "$ECS_DIR/load-state.sh"
}

deploy_failed() {
  echo "ECS deployment failed:" >&2
  aws ecs describe-services --cluster "$ECS_CLUSTER_NAME" --services "$ECS_SERVICE_NAME" \
    --region "$AWS_REGION" \
    --query 'services[0].{running:runningCount,desired:desiredCount,events:events[0:5]}' \
    --output json >&2 || true
}

rollout() {
  local image="${ECR_IMAGE:?ECR_IMAGE must be set}"
  export EXECUTION_ROLE_ARN TASK_ROLE_ARN LOG_GROUP AWS_REGION FARGATE_CPU FARGATE_MEMORY TASK_FAMILY
  local task_def
  task_def=$(bash "$ECS_DIR/register-task-definition.sh" "$image")
  echo "Registered $task_def"

  aws ecs update-service \
    --cluster "$ECS_CLUSTER_NAME" \
    --service "$ECS_SERVICE_NAME" \
    --task-definition "$task_def" \
    --force-new-deployment \
    --region "$AWS_REGION" \
    --query 'service.serviceName' --output text >/dev/null

  if ! aws ecs wait services-stable \
    --cluster "$ECS_CLUSTER_NAME" \
    --services "$ECS_SERVICE_NAME" \
    --region "$AWS_REGION"; then
    deploy_failed
    return 1
  fi
  echo "ECS service stable."
}

deploy_production() {
  bash "$ECS_DIR/sync-secrets.sh"
  rollout
}

load_env

AWS_ACCOUNT_ID="${AWS_ACCOUNT_ID:-$(aws sts get-caller-identity --query Account --output text)}"
ECR_IMAGE="${ECR_IMAGE:-${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/${ECR_REPOSITORY:-ytter}:latest}"

if [[ $# -eq 0 ]]; then
  deploy_production
  exit 0
fi

for target in "$@"; do
  case "$target" in
    app) rollout ;;
    production) deploy_production ;;
    secrets)
      bash "$ECS_DIR/sync-secrets.sh"
      aws ecs update-service --cluster "$ECS_CLUSTER_NAME" --service "$ECS_SERVICE_NAME" \
        --force-new-deployment --region "$AWS_REGION" >/dev/null
      aws ecs wait services-stable --cluster "$ECS_CLUSTER_NAME" --services "$ECS_SERVICE_NAME" --region "$AWS_REGION"
      ;;
    *)
      echo "unknown target: $target (use app, production, secrets)" >&2
      exit 1
      ;;
  esac
done
