# ECS Fargate deployment

Production API on **ECS Fargate** (replaces EKS): rolling deploys, ALB + ACM HTTPS, Redis sidecar in the task.

## Prerequisites

- AWS CLI, `jq`, Docker (for local image builds)
- `deployments/ecs/infra.env` (copy from `infra.env.example`)
- `deployments/ecs/app-secrets.env` (same keys as EKS; CI generates via `prepare-app-secrets.sh`)

Export secrets from EKS once (optional):

```bash
kubectl get secret ytter-app-secrets -o json | jq -r '.data | to_entries[] | "\(.key)=\(.value | @base64d)"' > deployments/ecs/app-secrets.env
```

## One-time bootstrap

```bash
cp deployments/ecs/infra.env.example deployments/ecs/infra.env
# edit infra.env

./deployments/ecs/bootstrap.sh
```

Creates: ECS cluster, Fargate service, ALB, ACM cert, Route53 alias for `API_HOST`, Secrets Manager secret, security groups (RDS ingress from tasks).

## Deploy (every release)

```bash
export ECR_IMAGE=211125664612.dkr.ecr.eu-west-2.amazonaws.com/ytter:latest
./deployments/ecs/deploy.sh production
```

GitHub Actions runs the same on push to `main`.

## Decommission EKS (after ECS is healthy)

1. Confirm `https://api.ytter.co.uk/swagger/` returns 200.
2. Delete EKS node group, then cluster (Console or CLI).
3. Remove unused NLB created by ingress-nginx (if still billing).
4. Optional: make RDS **private** and remove public accessibility.

Estimated savings: **~$73/mo** EKS control plane + node EC2.

## Rollback

```bash
# Previous task definition revision
aws ecs list-task-definitions --family-prefix ytter-api --sort DESC --max-items 5
aws ecs update-service --cluster ytter --service ytter-api --task-definition ytter-api:REVISION --force-new-deployment
```

Or point Route53 back to the old NLB (only if EKS is still running).
