#!/usr/bin/env bash
# Resolve ECS deploy ARNs from AWS (for CI when ecs-state.env is absent).
set -euo pipefail

: "${AWS_REGION:?}"
: "${ECS_CLUSTER_NAME:=ytter}"

EXECUTION_ROLE_NAME="${EXECUTION_ROLE_NAME:-ytterEcsTaskExecutionRole}"
TASK_ROLE_NAME="${TASK_ROLE_NAME:-ytterEcsTaskRole}"
ALB_NAME="${ALB_NAME:-ytter-api-alb}"
TG_NAME="${TG_NAME:-ytter-api-tg}"
LOG_GROUP="${LOG_GROUP:-/ecs/ytter-api}"

export EXECUTION_ROLE_ARN="${EXECUTION_ROLE_ARN:-$(aws iam get-role --role-name "$EXECUTION_ROLE_NAME" --query Role.Arn --output text)}"
export TASK_ROLE_ARN="${TASK_ROLE_ARN:-$(aws iam get-role --role-name "$TASK_ROLE_NAME" --query Role.Arn --output text)}"
export LOG_GROUP
export TARGET_GROUP_ARN="${TARGET_GROUP_ARN:-$(aws elbv2 describe-target-groups --names "$TG_NAME" --region "$AWS_REGION" --query 'TargetGroups[0].TargetGroupArn' --output text)}"
export ALB_ARN="${ALB_ARN:-$(aws elbv2 describe-load-balancers --names "$ALB_NAME" --region "$AWS_REGION" --query 'LoadBalancers[0].LoadBalancerArn' --output text)}"
export ALB_DNS_NAME="${ALB_DNS_NAME:-$(aws elbv2 describe-load-balancers --names "$ALB_NAME" --region "$AWS_REGION" --query 'LoadBalancers[0].DNSName' --output text)}"
