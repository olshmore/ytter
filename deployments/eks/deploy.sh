#!/usr/bin/env bash
# Deploy ytter to EKS
#
# Usage:
#   ./deployments/eks/deploy.sh              # same as production (CI)
#   ./deployments/eks/deploy.sh production   # app rollout + ingress rules (every release)
#   ./deployments/eks/deploy.sh app           # application only
#   ./deployments/eks/deploy.sh secrets       # Kubernetes secret only (needs app-secrets.env)
#   ./deployments/eks/deploy.sh ingress       # apply ingress / issuer manifests only
#
# One-time cluster setup (see deploy-bootstrap.sh):
#   ./deployments/eks/deploy-bootstrap.sh bootstrap
#   ./deployments/eks/deploy-bootstrap.sh addons
#
# Config: deployments/eks/infra.env + app-secrets.env (see app-secrets.env.example)
# Override path: APP_SECRETS_FILE=/path/to/app-secrets.env

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EKS="$ROOT/deployments/eks"
INFRA_ENV="$EKS/infra.env"
APP_SECRETS_FILE="${APP_SECRETS_FILE:-$EKS/app-secrets.env}"
# BOOTSTRAP_SCRIPT="$EKS/deploy-bootstrap.sh"

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

load_env

sync_secrets() {
  bash "$EKS/sync-secret.sh"
}

app_rollout_failed() {
  echo "ytter-api rollout failed:" >&2
  kubectl get pods -l app=ytter-api -o wide >&2 || true
  kubectl describe pods -l app=ytter-api 2>/dev/null | tail -80 >&2 || true
  kubectl get events --field-selector involvedObject.kind=Pod --sort-by=.lastTimestamp 2>/dev/null | tail -15 >&2 || true
}

deploy_app() {
  sync_secrets
  kubectl apply -f "$EKS/aws-auth.yaml"
  kubectl apply -f "$EKS/deployment.yaml"
  kubectl apply -f "$EKS/service.yaml"
  kubectl rollout restart deployment/ytter-api-deployment
  if ! kubectl rollout status deployment/ytter-api-deployment --timeout=300s; then
    app_rollout_failed
    return 1
  fi
}

apply_ingress_rules() {
  kubectl apply -f "$EKS/issuer.yaml"
  kubectl apply -f "$EKS/ingress-nginx.yaml"
  kubectl apply -f "$EKS/ingress-http.yaml"
  kubectl apply -f "$EKS/ingress-grpc.yaml"
}

deploy_production() {
  deploy_app
  apply_ingress_rules
}

run_all() {
  deploy_production
}

if [[ $# -eq 0 ]]; then
  run_all
  exit 0
fi

for target in "$@"; do
  case "$target" in
    app) deploy_app ;;
    production) deploy_production ;;
    # bootstrap|addons) bash "$BOOTSTRAP_SCRIPT" "$target" ;;
    secrets)
      sync_secrets
      kubectl rollout restart deployment/ytter-api-deployment
      kubectl rollout status deployment/ytter-api-deployment --timeout=300s
      ;;
    ingress) apply_ingress_rules ;;
    *)
      echo "unknown target: $target (use app, production, secrets, ingress; bootstrap/addons: deploy-bootstrap.sh)" >&2
      exit 1
      ;;
  esac
done
