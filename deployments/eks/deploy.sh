#!/usr/bin/env bash
# Deploy ytter to EKS
#
# Usage:
#   ./deployments/eks/deploy.sh          # app + add-ons + ingress (CI default)
#   ./deployments/eks/deploy.sh app      # application manifests only
#   ./deployments/eks/deploy.sh addons   # ingress-nginx + cert-manager
#   ./deployments/eks/deploy.sh ingress  # issuer + ingress rules
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
EKS="$ROOT/deployments/eks"

deploy_app() {
  kubectl apply -f "$EKS/aws-auth.yaml"
  kubectl apply -f "$EKS/deployment.yaml"
  kubectl apply -f "$EKS/service.yaml"
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
  deploy_app
  install_addons
  deploy_ingress
}

if [[ $# -eq 0 ]]; then
  run_all
  exit 0
fi

for target in "$@"; do
  case "$target" in
    app) deploy_app ;;
    addons) install_addons ;;
    ingress) deploy_ingress ;;
    *)
      echo "unknown target: $target (use app, addons, ingress, or no args for all)" >&2
      exit 1
      ;;
  esac
done
