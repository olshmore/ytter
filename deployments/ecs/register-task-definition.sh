#!/usr/bin/env bash
# Register a new Fargate task definition revision.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ECS_DIR="$ROOT/deployments/ecs"
INFRA_ENV="$ECS_DIR/infra.env"
STATE_ENV="$ECS_DIR/ecs-state.env"

IMAGE="${1:?image URI required}"

set -a
# shellcheck disable=SC1091
source "$INFRA_ENV"
if [[ -f "$STATE_ENV" ]]; then
  # shellcheck disable=SC1091
  source "$STATE_ENV"
fi
set +a
: "${EXECUTION_ROLE_ARN:?EXECUTION_ROLE_ARN must be set (bootstrap or ecs-state.env)}"
: "${LOG_GROUP:?LOG_GROUP must be set}"

SECRET_ARN=$(aws secretsmanager describe-secret \
  --secret-id "${APP_SECRET_NAME:-ytter/app}" \
  --region "$AWS_REGION" \
  --query ARN --output text)

export IMAGE SECRET_ARN EXECUTION_ROLE_ARN TASK_ROLE_ARN LOG_GROUP AWS_REGION
export CPU="${FARGATE_CPU:-1024}" MEMORY="${FARGATE_MEMORY:-2048}" TASK_FAMILY="${TASK_FAMILY:-ytter-api}"

python3 <<'PY' > /tmp/ytter-task-def.json
import json, os

secret_arn = os.environ["SECRET_ARN"]
keys = [
    "ALLOWED_ORIGINS", "FRONTEND_BASE_URL", "MIGRATION_URL", "GRPC_SERVER_ADDRESS",
    "GOOGLE_REDIRECT_URL", "DB_URL", "TOKEN_SYMMETRIC_KEY",
    "EMAIL_SENDER_NAME", "EMAIL_SENDER_ADDRESS", "EMAIL_SENDER_PASSWORD",
    "GOOGLE_CLIENT_ID", "GOOGLE_CLIENT_SECRET", "OPENAI_API_KEY",
]
secrets = [{"name": k, "valueFrom": f"{secret_arn}:{k}::"} for k in keys]
log_opts = lambda prefix: {
    "logDriver": "awslogs",
    "options": {
        "awslogs-group": os.environ["LOG_GROUP"],
        "awslogs-region": os.environ["AWS_REGION"],
        "awslogs-stream-prefix": prefix,
    },
}
task = {
    "family": os.environ["TASK_FAMILY"],
    "networkMode": "awsvpc",
    "requiresCompatibilities": ["FARGATE"],
    "cpu": os.environ["CPU"],
    "memory": os.environ["MEMORY"],
    "executionRoleArn": os.environ["EXECUTION_ROLE_ARN"],
    "taskRoleArn": os.environ.get("TASK_ROLE_ARN") or os.environ["EXECUTION_ROLE_ARN"],
    "containerDefinitions": [
        {
            "name": "redis",
            "image": "redis:7-alpine",
            "essential": True,
            "portMappings": [{"containerPort": 6379, "protocol": "tcp"}],
            "logConfiguration": log_opts("redis"),
        },
        {
            "name": "ytter-api",
            "image": os.environ["IMAGE"],
            "essential": True,
            "dependsOn": [{"containerName": "redis", "condition": "START"}],
            "portMappings": [{"containerPort": 8080, "protocol": "tcp"}],
            "environment": [
                {"name": "ENABLE_GRPC_SERVER", "value": "false"},
                {"name": "REDIS_ADDRESS", "value": "127.0.0.1:6379"},
            ],
            "secrets": secrets,
            "logConfiguration": log_opts("api"),
        },
    ],
}
print(json.dumps(task))
PY

aws ecs register-task-definition \
  --region "$AWS_REGION" \
  --cli-input-json file:///tmp/ytter-task-def.json \
  --query 'taskDefinition.taskDefinitionArn' \
  --output text

rm -f /tmp/ytter-task-def.json
