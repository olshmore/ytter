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

install_addons() {
  kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.8.2/deploy/static/provider/aws/deploy.yaml
  kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.13.1/cert-manager.yaml

  kubectl wait --for=condition=Established crd/clusterissuers.cert-manager.io --timeout=180s
  kubectl wait --namespace cert-manager \
    --for=condition=Available deployment/cert-manager \
    --timeout=300s
  kubectl wait --namespace cert-manager \
    --for=condition=Available deployment/cert-manager-webhook \
    --timeout=300s
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
