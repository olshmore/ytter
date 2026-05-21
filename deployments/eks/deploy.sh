#!/usr/bin/env bash
# Deploy ytter to EKS
#
# Usage:
#   ./deployments/eks/deploy.sh          # secrets + app + add-ons + ingress (CI default)
#   ./deployments/eks/deploy.sh app         # redis + secrets + application manifests
#   ./deployments/eks/deploy.sh production  # app + addons + ingress (CI)
#   ./deployments/eks/deploy.sh secrets     # Kubernetes secret only (needs app-secrets.env)
#   ./deployments/eks/deploy.sh addons      # ingress-nginx + cert-manager
#   ./deployments/eks/deploy.sh ingress     # issuer + ingress rules
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EKS="$ROOT/deployments/eks"

sync_secrets() {
  bash "$EKS/sync-secret.sh"
}

app_rollout_failed() {
  echo "ytter-api rollout failed:" >&2
  kubectl get pods -l app=ytter-api -o wide >&2 || true
  kubectl describe pods -l app=ytter-api 2>/dev/null | tail -80 >&2 || true
  kubectl get events --field-selector involvedObject.kind=Pod --sort-by=.lastTimestamp 2>/dev/null | tail -15 >&2 || true
}

redis_rollout_failed() {
  echo "redis rollout failed:" >&2
  kubectl get pods -l app=redis -o wide >&2 || true
  kubectl describe pods -l app=redis 2>/dev/null | tail -40 >&2 || true
}

deploy_redis() {
  kubectl apply -f "$EKS/redis.yaml"
  if ! kubectl rollout status deployment/redis --timeout=120s; then
    redis_rollout_failed
    return 1
  fi
}

deploy_app() {
  sync_secrets
  deploy_redis
  kubectl apply -f "$EKS/aws-auth.yaml"
  kubectl apply -f "$EKS/deployment.yaml"
  kubectl apply -f "$EKS/service.yaml"
  kubectl rollout restart deployment/ytter-api-deployment
  if ! kubectl rollout status deployment/ytter-api-deployment --timeout=300s; then
    app_rollout_failed
    return 1
  fi
}

deploy_production() {
  deploy_app
  install_addons
  deploy_ingress
}

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

install_addons() {
  kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.2/deploy/static/provider/aws/deploy.yaml
  kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.1/cert-manager.yaml

  kubectl wait --for=condition=Established crd/clusterissuers.cert-manager.io --timeout=180s

  wait_rollout ingress-nginx ingress-nginx-controller 600s
  wait_rollout cert-manager cert-manager-cainjector 600s
  wait_rollout cert-manager cert-manager-webhook 600s
  wait_rollout cert-manager cert-manager 600s
}

deploy_ingress() {
  kubectl apply -f "$EKS/issuer.yaml"
  kubectl apply -f "$EKS/ingress-nginx.yaml"
  kubectl apply -f "$EKS/ingress-http.yaml"
  kubectl apply -f "$EKS/ingress-grpc.yaml"
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
    secrets)
      sync_secrets
      kubectl rollout restart deployment/ytter-api-deployment
      kubectl rollout status deployment/ytter-api-deployment --timeout=300s
      ;;
    addons) install_addons ;;
    ingress) deploy_ingress ;;
    *)
      echo "unknown target: $target (use app, production, secrets, addons, ingress, or no args for all)" >&2
      exit 1
      ;;
  esac
done
