#!/usr/bin/env bash
# One-time ECS Fargate setup: cluster, ALB, ACM, Route53, IAM, security groups.
#
# Usage: ./deployments/ecs/bootstrap.sh
# Requires: infra.env, app-secrets.env (or export from EKS: see README)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ECS_DIR="$ROOT/deployments/ecs"
INFRA_ENV="$ECS_DIR/infra.env"
STATE_ENV="$ECS_DIR/ecs-state.env"

if [[ ! -f "$INFRA_ENV" ]]; then
  echo "missing $INFRA_ENV (copy from infra.env.example)" >&2
  exit 1
fi

set -a
# shellcheck disable=SC1091
source "$INFRA_ENV"
set +a

: "${AWS_REGION:?}"
: "${API_HOST:?}"
: "${ROUTE53_ZONE_NAME:?}"
: "${ECS_CLUSTER_NAME:=ytter}"
: "${ECS_SERVICE_NAME:=ytter-api}"

AWS_ACCOUNT_ID="${AWS_ACCOUNT_ID:-$(aws sts get-caller-identity --query Account --output text)}"
VPC_ID="${VPC_ID:-}"
SUBNET_IDS="${SUBNET_IDS:-}"
RDS_SECURITY_GROUP_IDS="${RDS_SECURITY_GROUP_IDS:-}"

if [[ -z "$VPC_ID" || -z "$SUBNET_IDS" ]]; then
  echo "Resolving VPC/subnets from EKS cluster ${EKS_CLUSTER_NAME:-ytter-eks}..."
  EKS_CLUSTER_NAME="${EKS_CLUSTER_NAME:-ytter-eks}"
  VPC_ID=$(aws eks describe-cluster --name "$EKS_CLUSTER_NAME" --region "$AWS_REGION" \
    --query 'cluster.resourcesVpcConfig.vpcId' --output text)
  SUBNET_IDS=$(aws eks describe-cluster --name "$EKS_CLUSTER_NAME" --region "$AWS_REGION" \
    --query 'cluster.resourcesVpcConfig.subnetIds' --output text | tr '\t' ',')
fi

if [[ -z "$RDS_SECURITY_GROUP_IDS" ]]; then
  RDS_SECURITY_GROUP_IDS=$(aws rds describe-db-instances --db-instance-identifier ytter --region "$AWS_REGION" \
    --query 'DBInstances[0].VpcSecurityGroups[].VpcSecurityGroupId' --output text | tr '\t' ',')
fi

IFS=',' read -r -a SUBNET_ARRAY <<< "$SUBNET_IDS"
# Use two subnets for ALB (required across AZs)
SUBNET_ALB="${SUBNET_ARRAY[0]},${SUBNET_ARRAY[1]}"
SUBNET_ECS="${SUBNET_IDS}"

echo "VPC=$VPC_ID"
echo "Subnets=$SUBNET_IDS"

# --- IAM ---
EXECUTION_ROLE_NAME="${EXECUTION_ROLE_NAME:-ytterEcsTaskExecutionRole}"
TASK_ROLE_NAME="${TASK_ROLE_NAME:-ytterEcsTaskRole}"

assume_role_policy='{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Principal": {"Service": "ecs-tasks.amazonaws.com"},
    "Action": "sts:AssumeRole"
  }]
}'

if ! aws iam get-role --role-name "$EXECUTION_ROLE_NAME" >/dev/null 2>&1; then
  aws iam create-role --role-name "$EXECUTION_ROLE_NAME" \
    --assume-role-policy-document "$assume_role_policy"
  aws iam attach-role-policy --role-name "$EXECUTION_ROLE_NAME" \
    --policy-arn arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy
fi

secrets_policy=$(cat <<EOF
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": ["secretsmanager:GetSecretValue"],
    "Resource": "arn:aws:secretsmanager:${AWS_REGION}:${AWS_ACCOUNT_ID}:secret:ytter/*"
  }]
}
EOF
)
aws iam put-role-policy --role-name "$EXECUTION_ROLE_NAME" \
  --policy-name ytterSecretsRead \
  --policy-document "$secrets_policy" 2>/dev/null || true

if ! aws iam get-role --role-name "$TASK_ROLE_NAME" >/dev/null 2>&1; then
  aws iam create-role --role-name "$TASK_ROLE_NAME" \
    --assume-role-policy-document "$assume_role_policy"
fi

EXECUTION_ROLE_ARN=$(aws iam get-role --role-name "$EXECUTION_ROLE_NAME" --query Role.Arn --output text)
TASK_ROLE_ARN=$(aws iam get-role --role-name "$TASK_ROLE_NAME" --query Role.Arn --output text)
echo "Execution role: $EXECUTION_ROLE_ARN"

# --- CloudWatch log group (short retention) ---
LOG_GROUP="${LOG_GROUP:-/ecs/ytter-api}"
if ! aws logs describe-log-groups --log-group-name-prefix "$LOG_GROUP" --region "$AWS_REGION" \
  --query "logGroups[?logGroupName=='${LOG_GROUP}'].logGroupName" --output text | grep -q .; then
  aws logs create-log-group --log-group-name "$LOG_GROUP" --region "$AWS_REGION"
  aws logs put-retention-policy --log-group-name "$LOG_GROUP" --region "$AWS_REGION" --retention-in-days 3
fi

# --- Security groups ---
ALB_SG_NAME="${ALB_SG_NAME:-ytter-ecs-alb}"
ECS_SG_NAME="${ECS_SG_NAME:-ytter-ecs-tasks}"

create_sg() {
  local name=$1 desc=$2
  local sg
  sg=$(aws ec2 describe-security-groups --region "$AWS_REGION" \
    --filters "Name=group-name,Values=$name" "Name=vpc-id,Values=$VPC_ID" \
    --query 'SecurityGroups[0].GroupId' --output text 2>/dev/null || true)
  if [[ -z "$sg" || "$sg" == "None" ]]; then
    sg=$(aws ec2 create-security-group --group-name "$name" --description "$desc" \
      --vpc-id "$VPC_ID" --region "$AWS_REGION" --query GroupId --output text)
  fi
  echo "$sg"
}

ALB_SG_ID=$(create_sg "$ALB_SG_NAME" "ytter ECS ALB")
ECS_SG_ID=$(create_sg "$ECS_SG_NAME" "ytter ECS Fargate tasks")

aws ec2 authorize-security-group-ingress --region "$AWS_REGION" --group-id "$ALB_SG_ID" \
  --protocol tcp --port 443 --cidr 0.0.0.0/0 2>/dev/null || true
aws ec2 authorize-security-group-ingress --region "$AWS_REGION" --group-id "$ALB_SG_ID" \
  --protocol tcp --port 80 --cidr 0.0.0.0/0 2>/dev/null || true
aws ec2 authorize-security-group-ingress --region "$AWS_REGION" --group-id "$ECS_SG_ID" \
  --protocol tcp --port 8080 --source-group "$ALB_SG_ID" 2>/dev/null || true

IFS=',' read -r -a RDS_SGS <<< "$RDS_SECURITY_GROUP_IDS"
for rds_sg in "${RDS_SGS[@]}"; do
  aws ec2 authorize-security-group-ingress --region "$AWS_REGION" --group-id "$rds_sg" \
    --protocol tcp --port 5432 --source-group "$ECS_SG_ID" 2>/dev/null || true
done

# --- ACM certificate ---
CERT_ARN=$(aws acm list-certificates --region "$AWS_REGION" \
  --certificate-statuses ISSUED \
  --query "CertificateSummaryList[?DomainName=='${API_HOST}'].CertificateArn | [0]" --output text)
if [[ -z "$CERT_ARN" || "$CERT_ARN" == "None" ]]; then
  echo "Requesting ACM certificate for ${API_HOST}..."
  CERT_ARN=$(aws acm request-certificate \
    --domain-name "$API_HOST" \
    --validation-method DNS \
    --region "$AWS_REGION" \
    --query CertificateArn --output text)

  HOSTED_ZONE_ID="${ROUTE53_HOSTED_ZONE_ID:-}"
  if [[ -z "$HOSTED_ZONE_ID" ]]; then
    HOSTED_ZONE_ID=$(aws route53 list-hosted-zones-by-name --dns-name "$ROUTE53_ZONE_NAME" \
      --query "HostedZones[?Name=='${ROUTE53_ZONE_NAME}.'].Id | [0]" --output text | sed 's|/hostedzone/||')
  fi

  for _ in $(seq 1 30); do
    opts=$(aws acm describe-certificate --certificate-arn "$CERT_ARN" --region "$AWS_REGION" \
      --query 'Certificate.DomainValidationOptions[0].ResourceRecord' --output json)
    if [[ "$opts" != "null" && -n "$opts" && "$opts" != "{}" ]]; then
      batch=$(python3 -c "
import json, sys
r = json.load(sys.stdin)
print(json.dumps({'Changes':[{'Action':'UPSERT','ResourceRecordSet':{
  'Name': r['Name'], 'Type': r['Type'], 'TTL': 300,
  'ResourceRecords': [{'Value': r['Value']}]
}}]}))
" <<< "$opts")
      aws route53 change-resource-record-sets --hosted-zone-id "$HOSTED_ZONE_ID" --change-batch "$batch"
      break
    fi
    sleep 5
  done

  echo "Waiting for ACM certificate validation..."
  aws acm wait certificate-validated --certificate-arn "$CERT_ARN" --region "$AWS_REGION"
fi
echo "Certificate: $CERT_ARN"

# --- ALB ---
ALB_NAME="${ALB_NAME:-ytter-api-alb}"
TG_NAME="${TG_NAME:-ytter-api-tg}"

ALB_ARN=$(aws elbv2 describe-load-balancers --region "$AWS_REGION" \
  --names "$ALB_NAME" --query 'LoadBalancers[0].LoadBalancerArn' --output text 2>/dev/null || true)
if [[ -z "$ALB_ARN" || "$ALB_ARN" == "None" ]]; then
  ALB_ARN=$(aws elbv2 create-load-balancer --name "$ALB_NAME" --type application \
    --scheme internet-facing \
    --subnets "${SUBNET_ARRAY[0]}" "${SUBNET_ARRAY[1]}" \
    --security-groups "$ALB_SG_ID" \
    --region "$AWS_REGION" \
    --query 'LoadBalancers[0].LoadBalancerArn' --output text)
fi
ALB_DNS=$(aws elbv2 describe-load-balancers --load-balancer-arns "$ALB_ARN" --region "$AWS_REGION" \
  --query 'LoadBalancers[0].DNSName' --output text)
ALB_ZONE=$(aws elbv2 describe-load-balancers --load-balancer-arns "$ALB_ARN" --region "$AWS_REGION" \
  --query 'LoadBalancers[0].CanonicalHostedZoneId' --output text)

TARGET_GROUP_ARN=$(aws elbv2 describe-target-groups --region "$AWS_REGION" \
  --names "$TG_NAME" --query 'TargetGroups[0].TargetGroupArn' --output text 2>/dev/null || true)
if [[ -z "$TARGET_GROUP_ARN" || "$TARGET_GROUP_ARN" == "None" ]]; then
  TARGET_GROUP_ARN=$(aws elbv2 create-target-group --name "$TG_NAME" \
    --protocol HTTP --port 8080 --vpc-id "$VPC_ID" \
    --target-type ip --health-check-path /swagger/ \
    --health-check-interval-seconds 30 --healthy-threshold-count 2 \
    --region "$AWS_REGION" \
    --query 'TargetGroups[0].TargetGroupArn' --output text)
fi

aws elbv2 modify-target-group --target-group-arn "$TARGET_GROUP_ARN" --region "$AWS_REGION" \
  --health-check-path /swagger/ >/dev/null

HTTP_LISTENER=$(aws elbv2 describe-listeners --load-balancer-arn "$ALB_ARN" --region "$AWS_REGION" \
  --query "Listeners[?Port==\`80\`].ListenerArn | [0]" --output text)
if [[ -z "$HTTP_LISTENER" || "$HTTP_LISTENER" == "None" ]]; then
  HTTP_LISTENER=$(aws elbv2 create-listener --load-balancer-arn "$ALB_ARN" --region "$AWS_REGION" \
    --protocol HTTP --port 80 \
    --default-actions Type=redirect,RedirectConfig="{Protocol=HTTPS,Port=443,StatusCode=HTTP_301}" \
    --query 'Listeners[0].ListenerArn' --output text)
fi

HTTPS_LISTENER=$(aws elbv2 describe-listeners --load-balancer-arn "$ALB_ARN" --region "$AWS_REGION" \
  --query "Listeners[?Port==\`443\`].ListenerArn | [0]" --output text)
if [[ -z "$HTTPS_LISTENER" || "$HTTPS_LISTENER" == "None" ]]; then
  aws elbv2 create-listener --load-balancer-arn "$ALB_ARN" --region "$AWS_REGION" \
    --protocol HTTPS --port 443 --certificates CertificateArn="$CERT_ARN" \
    --default-actions Type=forward,TargetGroupArn="$TARGET_GROUP_ARN"
fi

# --- ECS cluster ---
aws ecs create-cluster --cluster-name "$ECS_CLUSTER_NAME" --region "$AWS_REGION" 2>/dev/null || true

# --- Write state for deploy scripts (updated again at end) ---
cat > "$STATE_ENV" <<EOF
EXECUTION_ROLE_ARN=${EXECUTION_ROLE_ARN}
TASK_ROLE_ARN=${TASK_ROLE_ARN}
LOG_GROUP=${LOG_GROUP}
ALB_ARN=${ALB_ARN}
ALB_DNS_NAME=${ALB_DNS}
ALB_HOSTED_ZONE_ID=${ALB_ZONE}
TARGET_GROUP_ARN=${TARGET_GROUP_ARN}
ECS_SECURITY_GROUP_ID=${ECS_SG_ID}
ALB_SECURITY_GROUP_ID=${ALB_SG_ID}
CERT_ARN=${CERT_ARN}
VPC_ID=${VPC_ID}
SUBNET_IDS=${SUBNET_IDS}
EOF

# --- Secrets (from app-secrets.env if present) ---
if [[ -f "${APP_SECRETS_FILE:-$ECS_DIR/app-secrets.env}" ]]; then
  bash "$ECS_DIR/sync-secrets.sh"
fi

# --- Task definition + service ---
ECR_IMAGE="${ECR_IMAGE:-${AWS_ACCOUNT_ID}.dkr.ecr.${AWS_REGION}.amazonaws.com/${ECR_REPOSITORY:-ytter}:latest}"
export EXECUTION_ROLE_ARN TASK_ROLE_ARN LOG_GROUP AWS_REGION FARGATE_CPU FARGATE_MEMORY TASK_FAMILY
TASK_DEF_ARN=$(bash "$ECS_DIR/register-task-definition.sh" "$ECR_IMAGE")

DESIRED_COUNT="${DESIRED_COUNT:-1}"
SERVICE_EXISTS=$(aws ecs describe-services --cluster "$ECS_CLUSTER_NAME" --services "$ECS_SERVICE_NAME" \
  --region "$AWS_REGION" --query 'services[?status==`ACTIVE`].serviceName | [0]' --output text 2>/dev/null || true)

if [[ -z "$SERVICE_EXISTS" || "$SERVICE_EXISTS" == "None" ]]; then
  create_service() {
  aws ecs create-service \
    --cluster "$ECS_CLUSTER_NAME" \
    --service-name "$ECS_SERVICE_NAME" \
    --task-definition "$TASK_DEF_ARN" \
    --desired-count "$DESIRED_COUNT" \
    --launch-type FARGATE \
    --platform-version LATEST \
    --network-configuration "awsvpcConfiguration={subnets=[${SUBNET_ECS}],securityGroups=[${ECS_SG_ID}],assignPublicIp=ENABLED}" \
    --load-balancers "targetGroupArn=${TARGET_GROUP_ARN},containerName=ytter-api,containerPort=8080" \
    --health-check-grace-period-seconds 120 \
    --deployment-configuration "maximumPercent=200,minimumHealthyPercent=100,deploymentCircuitBreaker={enable=true,rollback=true}" \
    --region "$AWS_REGION"
  }
  if ! create_service 2>/tmp/ecs-create-svc.err; then
    if grep -q "service linked role" /tmp/ecs-create-svc.err; then
      echo "Retrying create-service after ECS service-linked role propagation..."
      sleep 15
      create_service
    else
      cat /tmp/ecs-create-svc.err >&2
      exit 1
    fi
  fi
else
  aws ecs update-service --cluster "$ECS_CLUSTER_NAME" --service "$ECS_SERVICE_NAME" \
    --task-definition "$TASK_DEF_ARN" --force-new-deployment --region "$AWS_REGION"
fi

echo "Waiting for ECS service stable..."
aws ecs wait services-stable --cluster "$ECS_CLUSTER_NAME" --services "$ECS_SERVICE_NAME" --region "$AWS_REGION"

# --- Route53 ---
HOSTED_ZONE_ID="${ROUTE53_HOSTED_ZONE_ID:-}"
if [[ -z "$HOSTED_ZONE_ID" ]]; then
  HOSTED_ZONE_ID=$(aws route53 list-hosted-zones-by-name --dns-name "$ROUTE53_ZONE_NAME" \
    --query "HostedZones[?Name=='${ROUTE53_ZONE_NAME}.'].Id | [0]" --output text | sed 's|/hostedzone/||')
fi

echo "Pointing ${API_HOST} to ALB ${ALB_DNS}..."
API_HOST="$API_HOST" ALB_DNS="$ALB_DNS" ALB_ZONE="$ALB_ZONE" python3 -c "
import json, os
dns = os.environ['ALB_DNS']
if not dns.endswith('.'):
    dns += '.'
host = os.environ['API_HOST']
if not host.endswith('.'):
    host += '.'
print(json.dumps({'Changes':[{'Action':'UPSERT','ResourceRecordSet':{
  'Name': host, 'Type': 'A',
  'AliasTarget': {
    'HostedZoneId': os.environ['ALB_ZONE'],
    'DNSName': dns,
    'EvaluateTargetHealth': True,
  },
}}]}))
" > /tmp/ytter-route53.json
aws route53 change-resource-record-sets --hosted-zone-id "$HOSTED_ZONE_ID" --change-batch file:///tmp/ytter-route53.json
rm -f /tmp/ytter-route53.json

echo ""
echo "ECS bootstrap complete."
echo "  Cluster:  ${ECS_CLUSTER_NAME}"
echo "  Service:  ${ECS_SERVICE_NAME}"
echo "  API URL:  https://${API_HOST}"
echo "  ALB DNS:  ${ALB_DNS}"
echo "  State:    ${STATE_ENV}"
echo ""
echo "Next: verify https://${API_HOST}/swagger/ then decommission EKS (see deployments/ecs/README.md)."
