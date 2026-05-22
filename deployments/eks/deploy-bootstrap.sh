#!/usr/bin/env bash
# One-time / rare EKS cluster setup (subnets, IAM, addons, Route53).
#
# Usage:
#   ./deployments/eks/deploy-bootstrap.sh bootstrap   # full cluster bootstrap
#   ./deployments/eks/deploy-bootstrap.sh addons      # (re)install ingress-nginx + cert-manager
#
# Config: same as deploy.sh (infra.env + app-secrets.env)

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EKS="$ROOT/deployments/eks"
INFRA_ENV="$EKS/infra.env"
APP_SECRETS_FILE="${APP_SECRETS_FILE:-$EKS/app-secrets.env}"

load_env() {
  if [[ ! -f "$INFRA_ENV" ]]; then
    echo "missing ${INFRA_ENV}" >&2
    exit 1
  fi
  set -a
  # shellcheck disable=SC1090
  source "$INFRA_ENV"
  if [[ -f "$APP_SECRETS_FILE" ]]; then
    # shellcheck disable=SC1090
    source "$APP_SECRETS_FILE"
  fi
  set +a
}

require_deploy_env() {
  local key
  for key in EKS_CLUSTER_NAME AWS_REGION ROUTE53_ZONE_NAME API_HOST GAPI_HOST CLUSTER_IAM_ROLE_NAME; do
    if [[ -z "${!key:-}" ]]; then
      echo "deploy config: ${key} must be set (see infra.env and app-secrets.env.example)" >&2
      exit 1
    fi
  done
}

AUTO_MODE_CLUSTER_POLICIES=(
  AmazonEKSLoadBalancingPolicy
  AmazonEKSNetworkingPolicy
  AmazonEKSComputePolicy
)

addon_rollout_failed() {
  local ns=$1
  echo "rollout failed in namespace $ns:" >&2
  kubectl get pods -n "$ns" -o wide >&2 || true
  kubectl get events -n "$ns" --sort-by=.lastTimestamp 2>/dev/null | tail -20 >&2 || true
}

wait_rollout() {
  local ns=$1 deployment=$2 timeout=${3:-600s}
  if ! kubectl rollout status "deployment/$deployment" -n "$ns" --timeout="$timeout"; then
    addon_rollout_failed "$ns"
    return 1
  fi
}

tag_cluster_subnets() {
  local subnet_ids
  subnet_ids="$(aws eks describe-cluster \
    --name "$EKS_CLUSTER_NAME" \
    --region "$AWS_REGION" \
    --query 'cluster.resourcesVpcConfig.subnetIds' \
    --output text)"
  if [[ -z "$subnet_ids" ]]; then
    echo "no subnets found for cluster $EKS_CLUSTER_NAME" >&2
    return 1
  fi
  echo "tagging cluster subnets for internet-facing ELB: $subnet_ids"
  aws ec2 create-tags --region "$AWS_REGION" --resources $subnet_ids \
    --tags \
      "Key=kubernetes.io/role/elb,Value=1" \
      "Key=kubernetes.io/cluster/${EKS_CLUSTER_NAME},Value=shared"
}

ensure_auto_mode_cluster_iam() {
  local policy attached
  for policy in "${AUTO_MODE_CLUSTER_POLICIES[@]}"; do
    attached="$(aws iam list-attached-role-policies \
      --role-name "$CLUSTER_IAM_ROLE_NAME" \
      --query "AttachedPolicies[?PolicyName=='${policy}'].PolicyName" \
      --output text)"
    if [[ -z "$attached" ]]; then
      echo "attaching ${policy} to ${CLUSTER_IAM_ROLE_NAME}"
      aws iam attach-role-policy \
        --role-name "$CLUSTER_IAM_ROLE_NAME" \
        --policy-arn "arn:aws:iam::aws:policy/${policy}"
    fi
  done
  echo "updating trust policy on ${CLUSTER_IAM_ROLE_NAME}"
  aws iam update-assume-role-policy \
    --role-name "$CLUSTER_IAM_ROLE_NAME" \
    --policy-document "file://${EKS}/cluster-role-trust-policy.json"
}

patch_ingress_nginx_lb() {
  kubectl apply -f "$EKS/ingress-nginx-controller-service-patch.yaml"
  kubectl apply -f "$EKS/ingress-nginx-controller-deployment-patch.yaml"
}

wait_ingress_lb_hostname() {
  local hostname="" timeout="${1:-600}" start
  start="$(date +%s)"
  echo "waiting for ingress-nginx LoadBalancer hostname (timeout ${timeout}s)..."
  while true; do
    hostname="$(kubectl get svc ingress-nginx-controller -n ingress-nginx \
      -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || true)"
    if [[ -n "$hostname" ]]; then
      echo "ingress LoadBalancer: ${hostname}"
      echo "$hostname"
      return 0
    fi
    if (( $(date +%s) - start > timeout )); then
      echo "timed out waiting for ingress LoadBalancer hostname" >&2
      kubectl describe svc ingress-nginx-controller -n ingress-nginx 2>&1 | tail -20 >&2 || true
      return 1
    fi
    sleep 10
  done
}

wait_nlb_active() {
  local lb_dns=$1 timeout="${2:-600}" state start
  start="$(date +%s)"
  echo "waiting for NLB to become active..."
  while true; do
    state="$(aws elbv2 describe-load-balancers --region "$AWS_REGION" \
      --query "LoadBalancers[?DNSName=='${lb_dns}'].State.Code | [0]" --output text 2>/dev/null || true)"
    if [[ "$state" == "active" ]]; then
      return 0
    fi
    if (( $(date +%s) - start > timeout )); then
      echo "timed out waiting for NLB active (last state: ${state:-unknown})" >&2
      return 1
    fi
    sleep 15
  done
}

route53_hosted_zone_id() {
  if [[ -n "${ROUTE53_HOSTED_ZONE_ID:-}" ]]; then
    echo "$ROUTE53_HOSTED_ZONE_ID"
    return 0
  fi
  aws route53 list-hosted-zones-by-name \
    --dns-name "$ROUTE53_ZONE_NAME" \
    --query "HostedZones[?Name=='${ROUTE53_ZONE_NAME}.'].Id | [0]" \
    --output text | sed 's|/hostedzone/||'
}

sync_route53_to_ingress_lb() {
  local lb_dns lb_zone_id hosted_zone_id
  lb_dns="$(wait_ingress_lb_hostname 120)"
  wait_nlb_active "$lb_dns" 600

  lb_zone_id="$(aws elbv2 describe-load-balancers --region "$AWS_REGION" \
    --query "LoadBalancers[?DNSName=='${lb_dns}'].CanonicalHostedZoneId | [0]" \
    --output text)"
  hosted_zone_id="$(route53_hosted_zone_id)"
  if [[ -z "$hosted_zone_id" || "$hosted_zone_id" == "None" ]]; then
    echo "could not resolve Route53 hosted zone for ${ROUTE53_ZONE_NAME}" >&2
    return 1
  fi

  echo "updating Route53 aliases in ${hosted_zone_id} -> ${lb_dns}"
  aws route53 change-resource-record-sets --hosted-zone-id "$hosted_zone_id" --change-batch "$(cat <<EOF
{
  "Comment": "Point API hosts to ingress-nginx NLB",
  "Changes": [
    {
      "Action": "UPSERT",
      "ResourceRecordSet": {
        "Name": "${API_HOST}",
        "Type": "A",
        "AliasTarget": {
          "HostedZoneId": "${lb_zone_id}",
          "DNSName": "${lb_dns}.",
          "EvaluateTargetHealth": true
        }
      }
    },
    {
      "Action": "UPSERT",
      "ResourceRecordSet": {
        "Name": "${GAPI_HOST}",
        "Type": "A",
        "AliasTarget": {
          "HostedZoneId": "${lb_zone_id}",
          "DNSName": "${lb_dns}.",
          "EvaluateTargetHealth": true
        }
      }
    }
  ]
}
EOF
)"
}

apply_ingress_rules() {
  kubectl apply -f "$EKS/issuer.yaml"
  kubectl apply -f "$EKS/ingress-nginx.yaml"
  kubectl apply -f "$EKS/ingress-http.yaml"
  kubectl apply -f "$EKS/ingress-grpc.yaml"
}

install_addons() {
  kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.2/deploy/static/provider/aws/deploy.yaml
  patch_ingress_nginx_lb

  kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.1/cert-manager.yaml

  kubectl wait --for=condition=Established crd/clusterissuers.cert-manager.io --timeout=180s

  wait_rollout ingress-nginx ingress-nginx-controller 600s
  wait_rollout cert-manager cert-manager-cainjector 600s
  wait_rollout cert-manager cert-manager-webhook 600s
  wait_rollout cert-manager cert-manager 600s

  wait_ingress_lb_hostname 600
}

bootstrap_cluster() {
  require_deploy_env
  tag_cluster_subnets
  ensure_auto_mode_cluster_iam
  install_addons
  apply_ingress_rules
  sync_route53_to_ingress_lb
}

load_env

if [[ $# -eq 0 ]]; then
  echo "usage: $0 bootstrap|addons" >&2
  exit 1
fi

for target in "$@"; do
  case "$target" in
    bootstrap) bootstrap_cluster ;;
    addons)
      require_deploy_env
      install_addons
      ;;
    *)
      echo "unknown target: $target (use bootstrap, addons)" >&2
      exit 1
      ;;
  esac
done
